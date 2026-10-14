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
	"path/filepath"
	"sync"
	"syscall"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	lvl "github.com/Azure/sonic-mgmt-common/translib/log"
	"github.com/Azure/sonic-mgmt-common/translib/ocbinds"
	"github.com/Azure/sonic-mgmt-common/translib/tlerr"
	log "github.com/golang/glog"
	"github.com/openconfig/ygot/ygot"
)

/* STATE_DB table naming the filesystem components.  A row carries no values;
 * those are read from statfs on every request. */
const FILESYSTEM_TBL = "FILESYSTEM_INFO"

/* Mount points served as filesystem components.  Each is written to
 * FILESYSTEM_TBL only if statfs succeeds on it. */
var fsComponentNames = []string{
	"/", "/host", "/boot", "/var/lib/docker", "/var/log",
	"/tmp", "/run", "/dev/shm",
	"/dev/hugepages", "/dev/mqueue", "/sys/fs/bpf", "/sys/fs/cgroup",
	"/sys/kernel/config", "/sys/kernel/security", "/sys/kernel/tracing",
	"/sys/kernel/debug/tracing",
}

var (
	fsTableMu      sync.Mutex
	fsTableWritten bool
)

/* Writes FILESYSTEM_TBL once per process.  A failed write is retried on the
 * next call. */
func ensureFilesystemTable() {
	fsTableMu.Lock()
	defer fsTableMu.Unlock()
	if fsTableWritten {
		return
	}
	d, err := db.NewDB(getDBOptions(db.StateDB))
	if err != nil {
		log.Warningf("ensureFilesystemTable: opening %v failed: %v", db.StateDB.Name(), err)
		return
	}
	defer d.DeleteDB()
	if err = writeFilesystemTable(d); err != nil {
		log.Warningf("ensureFilesystemTable: writing %s failed: %v", FILESYSTEM_TBL, err)
		return
	}
	fsTableWritten = true
}

/* Replaces the rows of FILESYSTEM_TBL with the entries of fsComponentNames that
 * resolve.  Redis drops a hash with no fields, so each row holds NULL:NULL. */
func writeFilesystemTable(d *db.DB) error {
	ts := &db.TableSpec{Name: FILESYSTEM_TBL}
	if err := d.DeleteKeys(ts, db.Key{Comp: []string{"*"}}); err != nil {
		return err
	}
	for _, name := range fsComponentNames {
		var st syscall.Statfs_t
		if err := syscall.Statfs(hostPath(name), &st); err != nil {
			log.V(lvl.DEBUG).Infof("writeFilesystemTable: statfs on %s failed: %v", name, err)
			continue
		}
		row := db.Value{Field: map[string]string{"NULL": "NULL"}}
		if err := d.SetEntry(ts, db.Key{Comp: []string{name}}, row); err != nil {
			return err
		}
	}
	return nil
}

/* Inside the gnmi container a bare statfs("/var/log") reports the container
 * overlay, not the host partition, so it goes through the read-only host root
 * bind-mounted at /mnt/host.  The os.Stat guard keeps
 * host-side runs and images without that mount working. */
func hostPath(p string) string {
	/* Clean while the name is still rooted, so a ".." in it resolves against "/"
	 * and cannot walk back out of the prefix. */
	p = filepath.Clean(p)
	if _, err := os.Stat("/mnt/host"); err == nil {
		return filepath.Join("/mnt/host", p)
	}
	return p
}

/* One statfs call yields every property, so only field selection differs.
 * Block counts are in Frsize units, and "used" is total minus free, which is
 * what df reports. */
var fsPropFields = map[string]func(*syscall.Statfs_t) uint64{
	"total_blocks":     func(s *syscall.Statfs_t) uint64 { return s.Blocks },
	"free_blocks":      func(s *syscall.Statfs_t) uint64 { return s.Bfree },
	"available_blocks": func(s *syscall.Statfs_t) uint64 { return s.Bavail },
	"total_bytes":      func(s *syscall.Statfs_t) uint64 { return s.Blocks * uint64(s.Frsize) },
	"free_bytes":       func(s *syscall.Statfs_t) uint64 { return s.Bfree * uint64(s.Frsize) },
	"available_bytes":  func(s *syscall.Statfs_t) uint64 { return s.Bavail * uint64(s.Frsize) },
	"used_blocks":      func(s *syscall.Statfs_t) uint64 { return s.Blocks - s.Bfree },
	"used_bytes":       func(s *syscall.Statfs_t) uint64 { return (s.Blocks - s.Bfree) * uint64(s.Frsize) },
}

/* Fills the state and properties of a mounted filesystem from statfs(2).  name
 * is the mount point, and subKey is the property name, or empty to emit every
 * property.  Only state/name and the properties are served. */
func fillFilesystemInfo(comp *ocbinds.OpenconfigPlatform_Components_Component, name, subKey, targetUriPath string) error {
	/* A name that does not resolve on this platform must yield no component at
	 * all, rather than a named one with no properties. */
	var st syscall.Statfs_t
	if err := syscall.Statfs(hostPath(name), &st); err != nil {
		log.V(lvl.DEBUG).Infof("fillFilesystemInfo: statfs on %s failed: %v", name, err)
		return err
	}

	switch {
	case targetUriPath == COMP, targetUriPath == COMP_ST, targetUriPath == COMP_STATE_NAME, isPropertiesPath(targetUriPath):
	default:
		return tlerr.NotFound("unsupported leaf path for Filesystem component: %v", targetUriPath)
	}

	/* A named property that does not exist must be reported. */
	if _, ok := fsPropFields[subKey]; subKey != "" && !ok {
		return tlerr.NotFound("%s has no property %s", name, subKey)
	}

	ygot.BuildEmptyTree(comp)
	compState := comp.State
	ygot.BuildEmptyTree(compState)
	compState.Name = &name

	for propName, field := range fsPropFields {
		if subKey != "" && propName != subKey {
			continue
		}
		/* The framework pre-creates list entries named in the request URI, so
		 * look before creating; NewProperty rejects a duplicate key. */
		prop := comp.Properties.Property[propName]
		if prop == nil {
			var err error
			if prop, err = comp.Properties.NewProperty(propName); err != nil {
				log.V(lvl.DEBUG).Infof("fillFilesystemInfo: NewProperty(%s) failed: %v", propName, err)
				continue
			}
		}
		ygot.BuildEmptyTree(prop)
		value, err := prop.State.To_OpenconfigPlatform_Components_Component_Properties_Property_State_Value_Union(field(&st))
		if err != nil {
			log.V(lvl.DEBUG).Infof("fillFilesystemInfo: property %s value conversion failed: %v", propName, err)
			continue
		}
		prop.State.Name = &propName
		prop.State.Value = value
	}
	return nil
}
