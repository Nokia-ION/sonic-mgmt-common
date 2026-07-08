package transformer

import (
	"fmt"
	"strconv"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	lvl "github.com/Azure/sonic-mgmt-common/translib/log"
	"github.com/Azure/sonic-mgmt-common/translib/ocbinds"
	"github.com/Azure/sonic-mgmt-common/translib/tlerr"
	log "github.com/golang/glog"
	"github.com/openconfig/ygot/ygot"
)

const (
	faultsTbl                  = "FAULT_INFO"
	faultsKey                  = "symptom"
	faultsRemediationTbl       = "REMEDIATION_INFO"
	faultsRemediationTblLookup = "repair_strategy"
	faultsRemediationKey       = "index"
)

type faultState struct {
	Symptom           ocbinds.E_OpenconfigPlatformHealthzFault_SYMPTOM_BASE
	Priority          uint8
	OriginTime        uint64
	LastDetectionTime uint64
	Occurrences       uint64
	Description       string
	Status            ocbinds.E_OpenconfigPlatform_Components_Component_Healthz_Faults_Fault_State_Status
}

type remediationState struct {
	Action ocbinds.E_OpenconfigPlatformHealthzFault_ACTION_BASE
	Target string
}

var dbToYangFaultSymptomMap = map[string]ocbinds.E_OpenconfigPlatformHealthzFault_SYMPTOM_BASE{
	"":                          ocbinds.OpenconfigPlatformHealthzFault_SYMPTOM_BASE_UNSET,
	"SYMPTOM_OVER_THRESHOLD":    ocbinds.OpenconfigPlatformHealthzFault_SYMPTOM_BASE_SYMPTOM_OVER_THRESHOLD,
	"SYMPTOM_UNDER_THRESHOLD":   ocbinds.OpenconfigPlatformHealthzFault_SYMPTOM_BASE_SYMPTOM_UNDER_THRESHOLD,
	"SYMPTOM_MEMORY_ERRORS":     ocbinds.OpenconfigPlatformHealthzFault_SYMPTOM_BASE_SYMPTOM_MEMORY_ERRORS,
	"SYMPTOM_MISSING_COMPONENT": ocbinds.OpenconfigPlatformHealthzFault_SYMPTOM_BASE_SYMPTOM_MISSING_COMPONENT,
	"SYMPTOM_MISWIRED":          ocbinds.OpenconfigPlatformHealthzFault_SYMPTOM_BASE_SYMPTOM_MISWIRED,
	"SYMPTOM_COMM_ERROR":        ocbinds.OpenconfigPlatformHealthzFault_SYMPTOM_BASE_SYMPTOM_COMM_ERROR,
	"SYMPTOM_UNKNOWN":           ocbinds.OpenconfigPlatformHealthzFault_SYMPTOM_BASE_SYMPTOM_UNKNOWN,
}

var dbToYangFaultStatusMap = map[string]ocbinds.E_OpenconfigPlatform_Components_Component_Healthz_Faults_Fault_State_Status{
	"":            ocbinds.OpenconfigPlatform_Components_Component_Healthz_Faults_Fault_State_Status_UNSET,
	"ACTIVE":      ocbinds.OpenconfigPlatform_Components_Component_Healthz_Faults_Fault_State_Status_ACTIVE,
	"INACTIVE":    ocbinds.OpenconfigPlatform_Components_Component_Healthz_Faults_Fault_State_Status_INACTIVE,
	"UNSPECIFIED": ocbinds.OpenconfigPlatform_Components_Component_Healthz_Faults_Fault_State_Status_UNSPECIFIED,
}

var dbToYangFaultActionMap = map[string]ocbinds.E_OpenconfigPlatformHealthzFault_ACTION_BASE{
	"":                     ocbinds.OpenconfigPlatformHealthzFault_ACTION_BASE_UNSET,
	"ACTION_RESEAT":        ocbinds.OpenconfigPlatformHealthzFault_ACTION_BASE_ACTION_RESEAT,
	"ACTION_COLD_REBOOT":   ocbinds.OpenconfigPlatformHealthzFault_ACTION_BASE_ACTION_COLD_REBOOT,
	"ACTION_WARM_REBOOT":   ocbinds.OpenconfigPlatformHealthzFault_ACTION_BASE_ACTION_WARM_REBOOT,
	"ACTION_POWER_CYCLE":   ocbinds.OpenconfigPlatformHealthzFault_ACTION_BASE_ACTION_POWER_CYCLE,
	"ACTION_FACTORY_RESET": ocbinds.OpenconfigPlatformHealthzFault_ACTION_BASE_ACTION_FACTORY_RESET,
	"ACTION_REPLACE":       ocbinds.OpenconfigPlatformHealthzFault_ACTION_BASE_ACTION_REPLACE,
}

func init() {
	XlateFuncBind("DbToYang_healthz_fault_xfmr", DbToYang_healthz_fault_xfmr)
	XlateFuncBind("Subscribe_healthz_fault_xfmr", Subscribe_healthz_fault_xfmr)
}

func Subscribe_healthz_fault_xfmr(inParams XfmrSubscInParams) (XfmrSubscOutParams, error) {
	pathInfo := NewPathInfo(inParams.uri)
	name := pathInfo.Var("name")
	symptom := pathInfo.Var(faultsKey)
	index := pathInfo.Var("index")
	log.V(lvl.DEBUG).Infof("Subscribe_healthz_fault_xfmr: name: %v; symptom: %v; index: %v", name, symptom, index)

	if index == "" {
		return XfmrSubscOutParams{
			dbDataMap:    RedisDbSubscribeMap{db.StateDB: {faultsTbl: {name + "|" + symptom: {}}}},
			needCache:    true,
			nOpts:        &notificationOpts{mInterval: 0, pType: OnChange},
			isVirtualTbl: false,
		}, nil
	}

	if inParams.dbs[db.StateDB] == nil {
		return XfmrSubscOutParams{}, fmt.Errorf("Subscribe_healthz_fault_xfmr: inParams.dbs[db.StateDB] is nil!")
	}
	subMap := make(map[string]map[string]string)
	if keys, err := inParams.dbs[db.StateDB].GetKeysPattern(&db.TableSpec{Name: faultsRemediationTbl}, db.Key{Comp: []string{"*", index}}); err == nil {
		for _, key := range keys {
			if key.Len() != 2 {
				continue
			}
			subMap[key.Comp[0]+"|"+index] = map[string]string{}
		}
	}

	return XfmrSubscOutParams{
		dbDataMap:    RedisDbSubscribeMap{db.StateDB: {faultsRemediationTbl: subMap}},
		needCache:    true,
		nOpts:        &notificationOpts{mInterval: 0, pType: OnChange},
		isVirtualTbl: false,
	}, nil
}

var DbToYang_healthz_fault_xfmr SubTreeXfmrDbToYang = func(inParams XfmrParams) error {
	log.V(lvl.DEBUG).Infof("DbToYang_healthz_fault_xfmr - inParams.uri: %v", inParams.uri)
	pathInfo := NewPathInfo(inParams.uri)
	componentName := pathInfo.Var("name")
	if componentName == "" {
		// This case is handled by a more generic component transformer
		return nil
	}
	symptom := pathInfo.Var(faultsKey)
	targetUriPath, err := getYangPathFromUri(pathInfo.Path)
	if err != nil {
		return tlerr.InvalidArgs("DbToYang_healthz_fault_xfmr failed to get Yang path from URI: %v", err)
	}

	comp := (*inParams.ygRoot).(*ocbinds.Device).Components.Component[componentName]
	if comp == nil {
		return tlerr.InvalidArgs("Component not found")
	}

	stdb := inParams.dbs[db.StateDB]
	if stdb == nil {
		return tlerr.InvalidArgs("DbToYang_healthz_fault_xfmr stateDb is nil!")
	}

	return fillHealthzInfo(componentName, symptom, targetUriPath, stdb, comp)
}

func fillRemediationInfo(faultObj *ocbinds.OpenconfigPlatform_Components_Component_Healthz_Faults_Fault, repairStrategy string, stateDb *db.DB) error {
	if repairStrategy == "" {
		return fmt.Errorf("Repair strategy is empty for fault")
	}
	faultRemediationKeys, err := stateDb.GetKeysPattern(&db.TableSpec{Name: faultsRemediationTbl}, db.Key{Comp: []string{repairStrategy, "*"}})
	log.V(lvl.DEBUG).Infof("faultRemediationKeys: %v", faultRemediationKeys)
	if err != nil {
		return fmt.Errorf("Failed to read from Remediation Info table err: %v", err)
	}
	for i, _ := range faultRemediationKeys {
		log.V(lvl.DEBUG).Infof("faultRemediationKey: %v", faultRemediationKeys[i])
		index := strconv.Itoa(i)
		faultRemediationTable, err := stateDb.GetEntry(&db.TableSpec{Name: faultsRemediationTbl + "|" + repairStrategy}, db.Key{Comp: []string{index}})
		if err != nil {
			log.V(lvl.INFO).Infof("Failed to read from %#v err: %v", &db.TableSpec{Name: faultsRemediationTbl + "|" + repairStrategy}, db.Key{Comp: []string{index}}, err)
			continue
		}
		// create object and read values for each remediation entry.
		var remediation remediationState
		remediation.Action = dbToYangFaultActionMap[faultRemediationTable.Get("action")]
		remediation.Target = faultRemediationTable.Get("target")

		remObj, ok := faultObj.Remediations.Remediation[uint64(i)]
		if !ok {
			remObj, err = faultObj.Remediations.NewRemediation(uint64(i))
			if err != nil {
				log.V(lvl.INFO).Infof("Remediation creation failed with NewRemediation for action %v; err = %v", remediation.Action, err)
				continue
			}
		}

		ygot.BuildEmptyTree(remObj)
		ygot.BuildEmptyTree(remObj.State)
		remObj.State.Action = remediation.Action
		remObj.State.Target = &remediation.Target
	}
	return nil
}

func fillHealthzInfo(name, symptom, targetUriPath string, stdb *db.DB, comp *ocbinds.OpenconfigPlatform_Components_Component) error {
	ygot.BuildEmptyTree(comp.Healthz)
	ygot.BuildEmptyTree(comp.Healthz.State)
	ygot.BuildEmptyTree(comp.Healthz.Faults)

	var faultSymptomKeys []db.Key
	var faultTable db.Value
	var err error
	if symptom == "" {
		symptom = "*"
	}
	if symptom == "*" {
		if faultSymptomKeys, err = stdb.GetKeys(&db.TableSpec{Name: faultsTbl + "|" + name}); err != nil {
			log.V(lvl.DEBUG).Infof("Failed to read from FAULT_INFO table err: %v", err)
			return tlerr.NotFound("Failed to read from FAULT_INFO table err: %v", err)
		}
	} else {
		faultSymptomKeys = []db.Key{db.Key{Comp: []string{name, symptom}}}
	}

	if len(faultSymptomKeys) == 0 {
		comp.Healthz.State.Status = ocbinds.OpenconfigPlatform_Components_Component_Healthz_State_Status_HEALTHY
		return nil
	}

	comp.Healthz.State.Status = ocbinds.OpenconfigPlatform_Components_Component_Healthz_State_Status_UNHEALTHY

	for _, symptomKey := range faultSymptomKeys {
		log.V(lvl.DEBUG).Infof("faultSymptomKey: %v", symptomKey)
		if len(symptomKey.Comp) < 2 {
			log.V(lvl.DEBUG).Infof("Symptom %#v is not supported", symptomKey.Comp)
			continue
		}
		symptomString := symptomKey.Comp[1]
		faultTable, err = stdb.GetEntry(&db.TableSpec{Name: faultsTbl}, symptomKey)
		if err != nil {
			log.V(lvl.DEBUG).Infof("Failed to read from FAULT_INFO table err: %v", err)
			continue
		}

		symptomEnum := dbToYangFaultSymptomMap[symptomString]
		if symptomEnum == ocbinds.OpenconfigPlatformHealthzFault_SYMPTOM_BASE_UNSET {
			log.V(lvl.ERROR).Infof("Symptom %v is not supported", symptomString)
			continue
		}
		faultObj, ok := comp.Healthz.Faults.Fault[symptomEnum]
		if !ok {
			faultObj, err = comp.Healthz.Faults.NewFault(symptomEnum)
			if err != nil {
				return tlerr.New("Fault creation failed for symptom %v; err = %v", symptomString, err)
			}
		}

		ygot.BuildEmptyTree(faultObj)
		ygot.BuildEmptyTree(faultObj.State)
		ygot.BuildEmptyTree(faultObj.State.Counters)

		var state faultState
		state.Description = faultTable.Get("description")
		if status, ok := dbToYangFaultStatusMap[faultTable.Get("status")]; ok {
			state.Status = status
		} else {
			state.Status = ocbinds.OpenconfigPlatform_Components_Component_Healthz_Faults_Fault_State_Status_UNSET
		}

		if p, err := strconv.ParseUint(faultTable.Get("priority"), 10, 8); err == nil {
			state.Priority = uint8(p)
		}
		if state.OriginTime, err = strconv.ParseUint(faultTable.Get("origin_time"), 10, 64); err == nil {
			faultObj.State.OriginTime = &state.OriginTime
		}
		if state.LastDetectionTime, err = strconv.ParseUint(faultTable.Get("last_detection_time"), 10, 64); err == nil {
			faultObj.State.LastDetectionTime = &state.LastDetectionTime
		}
		if state.Occurrences, err = strconv.ParseUint(faultTable.Get("occurrences"), 10, 64); err == nil {
			faultObj.State.Counters.Occurrences = &state.Occurrences
		}
		faultObj.State.Symptom = symptomEnum
		faultObj.State.Description = NilIfStringEmpty(&state.Description)
		faultObj.State.Status = state.Status

		// Read from Remediation Info table
		if err := fillRemediationInfo(faultObj, faultTable.Get(faultsRemediationTblLookup), stdb); err != nil {
			log.V(lvl.WARNING).Infof("Failed to fill remediation info for fault %v; err = %v", symptomString, err)
			continue
		}
	}
	return nil
}
