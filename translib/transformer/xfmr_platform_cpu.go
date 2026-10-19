////////////////////////////////////////////////////////////////////////////////
//                                                                             //
//  Copyright 2026 Nokia.                                                      //
//                                                                             //
//  Licensed under the Apache License, Version 2.0 (the "License");            //
//  you may not use this file except in compliance with the License.           //
//  You may obtain a copy of the License at                                    //
//                                                                             //
//     http://www.apache.org/licenses/LICENSE-2.0                              //
//                                                                             //
//  Unless required by applicable law or agreed to in writing, software        //
//  distributed under the License is distributed on an "AS IS" BASIS,          //
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.   //
//  See the License for the specific language governing permissions and        //
//  limitations under the License.                                             //
//                                                                             //
////////////////////////////////////////////////////////////////////////////////

package transformer

import (
	"os"
	"strings"
	"sync"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	lvl "github.com/Azure/sonic-mgmt-common/translib/log"
	"github.com/Azure/sonic-mgmt-common/translib/ocbinds"
	"github.com/Azure/sonic-mgmt-common/translib/tlerr"
	log "github.com/golang/glog"
	"github.com/openconfig/ygot/ygot"
)

/* STATE_DB table naming the host CPU component, so that a subscription has a
 * row to watch.  The row carries no values; those are read from /proc. */
const CPU_INFO_TBL = "CPU_INFO"

var (
	cpuModelOnce sync.Once
	cpuModel     string

	cpuTableMu      sync.Mutex
	cpuTableWritten bool
)

/* Writes CPU_INFO_TBL once per process.  A failed write is retried on the next
 * call. */
func ensureCpuTable() {
	cpuTableMu.Lock()
	defer cpuTableMu.Unlock()
	if cpuTableWritten {
		return
	}
	d, err := db.NewDB(getDBOptions(db.StateDB))
	if err != nil {
		log.Warningf("ensureCpuTable: opening %v failed: %v", db.StateDB.Name(), err)
		return
	}
	defer d.DeleteDB()
	if err = writeCpuTable(d); err != nil {
		log.Warningf("ensureCpuTable: writing %s failed: %v", CPU_INFO_TBL, err)
		return
	}
	cpuTableWritten = true
}

/* Writes the CPU_INFO_TBL row of the host CPU.  Redis drops a hash with no
 * fields, so the row holds NULL:NULL. */
func writeCpuTable(d *db.DB) error {
	row := db.Value{Field: map[string]string{"NULL": "NULL"}}
	return d.SetEntry(&db.TableSpec{Name: CPU_INFO_TBL}, db.Key{Comp: []string{CPU_COMP_NAME}}, row)
}

/* The cpu model string does not change while the host runs, so
 * /proc/cpuinfo is read once per process. */
func getCpuModel() string {
	cpuModelOnce.Do(func() {
		data, err := os.ReadFile(procCpuInfoPath)
		if err != nil {
			log.V(lvl.DEBUG).Infof("getCpuModel: reading %s failed: %v", procCpuInfoPath, err)
			return
		}
		cpuModel = parseCpuModel(string(data))
	})
	return cpuModel
}

/* The first "model name" line of /proc/cpuinfo.  ARM kernels write none. */
func parseCpuModel(cpuinfo string) string {
	for _, line := range strings.Split(cpuinfo, "\n") {
		key, val, ok := strings.Cut(line, ":")
		if ok && strings.TrimSpace(key) == "model name" {
			return strings.TrimSpace(val)
		}
	}
	return ""
}

/* Creates the host CPU component, which no table lists, and fills it. */
func createCpuComp(pfCpts *ocbinds.OpenconfigPlatform_Components, targetUriPath string, inParams XfmrParams) {
	comp := pfCpts.Component[CPU_COMP_NAME]
	if comp == nil {
		var err error
		if comp, err = pfCpts.NewComponent(CPU_COMP_NAME); err != nil {
			log.V(lvl.DEBUG).Infof("Component creation failed with NewComponent for comp %v; err = %v", CPU_COMP_NAME, err)
			return
		}
	}
	ygot.BuildEmptyTree(comp)
	if err := compTypeToFuncCall(CompTypeCpu, CPU_COMP_NAME, "", comp, targetUriPath, inParams.dbs, AllPaths, inParams.ygRoot); err != nil {
		log.V(lvl.DEBUG).Info(err)
	}
	fillHealthzInfo(CPU_COMP_NAME, "", targetUriPath, inParams.dbs[db.StateDB], comp)
}

/* Fills the host CPU component.  The host has no source for its oper-status;
 * a CPU running this code is ACTIVE. */
func fillCpuInfo(comp *ocbinds.OpenconfigPlatform_Components_Component, pType PathType, targetUriPath string) error {
	ygot.BuildEmptyTree(comp)
	ygot.BuildEmptyTree(comp.State)
	name := CPU_COMP_NAME
	parent := CHASSIS_COMP_NAME
	model := getCpuModel()

	if targetUriPath == COMP || targetUriPath == COMP_CFG || targetUriPath == COMP_CONFIG_NAME {
		comp.Config.Name = &name
	}

	if pType == AllPaths || targetUriPath == COMP_ST {
		comp.State.Name = &name
		comp.State.Type, _ = comp.State.To_OpenconfigPlatform_Components_Component_State_Type_Union(
			ocbinds.OpenconfigPlatformTypes_OPENCONFIG_HARDWARE_COMPONENT_CPU)
		comp.State.Parent = &parent
		comp.State.OperStatus = ocbinds.OpenconfigPlatformTypes_COMPONENT_OPER_STATUS_ACTIVE
		if model != "" {
			comp.State.Description = &model
		}
	}

	/* Utilization is sampled from /proc/stat; having none is not fatal to the
	 * rest of the component. */
	if pType == AllPaths {
		if err := fillCpuUtilization(comp); err != nil {
			log.V(lvl.DEBUG).Infof("fillCpuInfo: cpu utilization: %v", err)
		}
	}

	if pType != SingularPath {
		return nil
	}

	switch targetUriPath {
	case COMP_STATE_NAME:
		comp.State.Name = &name
	case COMP_STATE_TYPE:
		comp.State.Type, _ = comp.State.To_OpenconfigPlatform_Components_Component_State_Type_Union(
			ocbinds.OpenconfigPlatformTypes_OPENCONFIG_HARDWARE_COMPONENT_CPU)
	case COMP_STATE_PARENT:
		comp.State.Parent = &parent
	case COMP_STATE_OPER_STATUS:
		comp.State.OperStatus = ocbinds.OpenconfigPlatformTypes_COMPONENT_OPER_STATUS_ACTIVE
	case COMP_STATE_DESCR:
		if model == "" {
			return tlerr.NotFound("%s has no description", name)
		}
		comp.State.Description = &model
	default:
		return tlerr.NotFound("unsupported leaf path for CPU component: %v", targetUriPath)
	}
	return nil
}
