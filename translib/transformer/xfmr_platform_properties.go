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
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	lvl "github.com/Azure/sonic-mgmt-common/translib/log"
	"github.com/Azure/sonic-mgmt-common/translib/ocbinds"
	"github.com/Azure/sonic-mgmt-common/translib/tlerr"
	log "github.com/golang/glog"
	"github.com/openconfig/ygot/ygot"
)

/* components/component/properties/property is a generic name/value bag for
 * facts with no typed leaf of their own.  The property name is a free-form
 * string, so every property resolves to the same yang leaf and one descriptor
 * table plus one filler serves all of them. */

const (
	/* Prefix shared by every property leaf. */
	COMP_PROPS = "/openconfig-platform:components/component/properties"

	/* Per-port alarm and warning limits for the DOM readings. */
	TRANSCEIVER_DOM_THRESHOLD = "TRANSCEIVER_DOM_THRESHOLD"

	/* Yang leaf a property's value lands on. */
	PROP_VALUE_LEAF = "value"

	/* Nodes an on-change subscription can be served on. */
	COMP_PROP_STATE       = COMP_PROPS + "/property/state"
	COMP_PROP_STATE_VALUE = COMP_PROP_STATE + "/" + PROP_VALUE_LEAF

	/* Prefix shared by every leaf under a component's cpu anchor.  The
	 * utilization container arrives module-qualified, so a prefix match is what
	 * covers the anchor, the container, its state and the leaf alike. */
	COMP_CPU = COMP + "/cpu"

	procLoadAvgPath = "/proc/loadavg"
	procCpuInfoPath = "/proc/cpuinfo"
	procStatPath    = "/proc/stat"

	/* Interval between the two /proc/stat reads a utilization percentage is
	 * computed from. */
	cpuSampleWindow = 100 * time.Millisecond

	/* The fields following the name on a /proc/stat cpu line are user, nice,
	 * system, idle, iowait, irq, softirq and steal.  idle is the fourth, and
	 * steal is the last one summed: the two after it, guest and guest_nice, are
	 * already counted inside user and nice. */
	cpuStatIdleField = 4
	cpuStatLastField = 8
)

/* Reads one property for one component.  cType is the type the caller resolved
 * the component to, and selects the table for a property whose source depends on
 * it.  The returned value must be a type the properties/property/state/value
 * union accepts: bool, float64, int64, string or uint64. */
type propGetter func(compName string, cType componentType, dbs [db.MaxDB]*db.DB) (interface{}, error)

type propDescriptor struct {
	/* Component types carrying this property.  A GET on any other type is
	 * NotFound rather than empty. */
	types []componentType
	/* STATE_DB table and field the value is read from, empty for a non-DB
	 * property.  Both are needed for on-change: a subscription watches this
	 * table and matches notifications by this field name. */
	table string
	field string
	/* tables overrides table per component type, for a property whose backing
	 * table depends on the type.  When nil, table applies to every listed
	 * type. */
	tables map[componentType]string
	/* subFields lists the Redis fields an on-change subscription must watch when
	 * the value derives from more than one field.  When empty, field is used. */
	subFields []string
	get       propGetter
}

func (d propDescriptor) appliesTo(cType componentType) bool {
	for _, t := range d.types {
		if t == cType {
			return true
		}
	}
	return false
}

/* STATE_DB table this property reads on the given component type. */
func (d propDescriptor) tableFor(cType componentType) string {
	if d.tables != nil {
		return d.tables[cType]
	}
	return d.table
}

/* Reads a float-valued field from a per-component STATE_DB table.  Several field
 * names may be given; the first present, non-"N/A" one wins, which serves a
 * value that has a legacy alias (e.g. PSU power / output_power). */
func stateFloat(types []componentType, table string, fields ...string) propDescriptor {
	return propDescriptor{
		types: types,
		table: table,
		field: fields[0],
		get: func(compName string, _ componentType, dbs [db.MaxDB]*db.DB) (interface{}, error) {
			d := dbs[db.StateDB]
			if d == nil {
				return nil, tlerr.NotFound("no STATE_DB client")
			}
			entry, err := d.GetEntry(&db.TableSpec{Name: table}, db.Key{Comp: []string{compName}})
			if err != nil {
				return nil, err
			}
			/* A component may be absent, or report an individual field as
			 * unavailable. */
			var raw string
			for _, f := range fields {
				if v := entry.Get(f); v != "" && v != "N/A" {
					raw = v
					break
				}
			}
			if raw == "" {
				return nil, tlerr.NotFound("%s has no %s value in %s", compName, fields[0], table)
			}
			v, err := strconv.ParseFloat(raw, 64)
			if err != nil {
				return nil, err
			}
			return v, nil
		},
	}
}

/* Reads one float field from a per-port transceiver table. */
func xcvrFloat(table, field string) propDescriptor {
	return stateFloat([]componentType{CompTypeXcvr}, table, field)
}

func domThreshold(field string) propDescriptor {
	return xcvrFloat(TRANSCEIVER_DOM_THRESHOLD, field)
}

func domSensor(field string) propDescriptor {
	return xcvrFloat(TRANSCEIVER_DOM, field)
}

/* Reads a property from the host rather than from a database.  Naming no table
 * leaves the property unsubscribable: no key changes when the value does, so it
 * can only be sampled. */
func hostProp(types []componentType, get propGetter) propDescriptor {
	return propDescriptor{types: types, get: get}
}

/* Component type the host properties are served on, the one the kernel they
 * describe runs on. */
var hostCpuTypes = []componentType{CompTypeChassis}

/* FRU component types that carry presence/status health in STATE_DB. */
var fruTypes = []componentType{CompTypePsu, CompTypeFan, CompTypeFanTray}

/* Component types with no presence or status in STATE_DB.  One that is served
 * exists and is working, so its status properties are fixed: present, active,
 * ACTIVE.  With no table to watch they can only be sampled. */
var fixedStatusTypes = []componentType{CompTypeStorage}

/* Component types carrying the status properties. */
var statusTypes = slices.Concat(fruTypes, fixedStatusTypes)

/* Maps a FRU component type to the STATE_DB table holding its presence/status. */
var fruStatusTables = map[componentType]string{
	CompTypePsu:     PSU_TBL,
	CompTypeFan:     FAN_TBL,
	CompTypeFanTray: FAN_TRAY_TBL,
}

/* Reads a SONiC boolean field, tolerant of both the lowercase and the
 * Python-capitalized spelling, which differ between the tables that write it. */
func parseFruBool(raw string) bool {
	return strings.EqualFold(strings.TrimSpace(raw), "true")
}

/* Reads a FRU's health row from the table its component type is served from. */
func fruStatusEntry(compName string, cType componentType, dbs [db.MaxDB]*db.DB) (db.Value, error) {
	d := dbs[db.StateDB]
	if d == nil {
		return db.Value{}, tlerr.NotFound("no STATE_DB client")
	}
	table, ok := fruStatusTables[cType]
	if !ok {
		return db.Value{}, tlerr.NotFound("%s carries no status", compName)
	}
	return d.GetEntry(&db.TableSpec{Name: table}, db.Key{Comp: []string{compName}})
}

/* Serves a FRU's health as the oper-status enum: absent -> DISABLED, present and
 * !status -> INACTIVE, present and status -> ACTIVE. */
func operStatusDescriptor() propDescriptor {
	return propDescriptor{
		types:     statusTypes,
		tables:    fruStatusTables,
		subFields: []string{"presence", "status"},
		get: func(compName string, cType componentType, dbs [db.MaxDB]*db.DB) (interface{}, error) {
			if slices.Contains(fixedStatusTypes, cType) {
				return "ACTIVE", nil
			}
			entry, err := fruStatusEntry(compName, cType, dbs)
			if err != nil {
				return nil, err
			}
			if entry.Get("presence") == "" {
				return nil, tlerr.NotFound("%s has no presence", compName)
			}
			switch {
			case !parseFruBool(entry.Get("presence")):
				return "DISABLED", nil
			case parseFruBool(entry.Get("status")):
				return "ACTIVE", nil
			default:
				return "INACTIVE", nil
			}
		},
	}
}

/* Serves one boolean field of a FRU's health row as its own property. */
func fruBool(field string) propDescriptor {
	return propDescriptor{
		types:  statusTypes,
		tables: fruStatusTables,
		field:  field,
		get: func(compName string, cType componentType, dbs [db.MaxDB]*db.DB) (interface{}, error) {
			if slices.Contains(fixedStatusTypes, cType) {
				return true, nil
			}
			entry, err := fruStatusEntry(compName, cType, dbs)
			if err != nil {
				return nil, err
			}
			raw := entry.Get(field)
			if raw == "" {
				return nil, tlerr.NotFound("%s has no %s", compName, field)
			}
			return parseFruBool(raw), nil
		},
	}
}

/* Property name to source.  The DOM threshold names follow
 * laser-<quantity>-<high|low>-<alarm|warn>-threshold; note that the yang "warn"
 * is Redis "warning" and the yang "voltage" is Redis "vcc". */
var propTbl = map[string]propDescriptor{
	"laser-temperature-high-alarm-threshold": domThreshold("temphighalarm"),
	"laser-temperature-low-alarm-threshold":  domThreshold("templowalarm"),
	"laser-temperature-high-warn-threshold":  domThreshold("temphighwarning"),
	"laser-temperature-low-warn-threshold":   domThreshold("templowwarning"),

	"laser-voltage-high-alarm-threshold": domThreshold("vcchighalarm"),
	"laser-voltage-low-alarm-threshold":  domThreshold("vcclowalarm"),
	"laser-voltage-high-warn-threshold":  domThreshold("vcchighwarning"),
	"laser-voltage-low-warn-threshold":   domThreshold("vcclowwarning"),

	"laser-rx-power-high-alarm-threshold": domThreshold("rxpowerhighalarm"),
	"laser-rx-power-low-alarm-threshold":  domThreshold("rxpowerlowalarm"),
	"laser-rx-power-high-warn-threshold":  domThreshold("rxpowerhighwarning"),
	"laser-rx-power-low-warn-threshold":   domThreshold("rxpowerlowwarning"),

	"laser-tx-power-high-alarm-threshold": domThreshold("txpowerhighalarm"),
	"laser-tx-power-low-alarm-threshold":  domThreshold("txpowerlowalarm"),
	"laser-tx-power-high-warn-threshold":  domThreshold("txpowerhighwarning"),
	"laser-tx-power-low-warn-threshold":   domThreshold("txpowerlowwarning"),

	"laser-tx-bias-high-alarm-threshold": domThreshold("txbiashighalarm"),
	"laser-tx-bias-low-alarm-threshold":  domThreshold("txbiaslowalarm"),
	"laser-tx-bias-high-warn-threshold":  domThreshold("txbiashighwarning"),
	"laser-tx-bias-low-warn-threshold":   domThreshold("txbiaslowwarning"),

	/* A reading rather than a threshold, so it comes from a different table. */
	"laser-module-voltage": domSensor("voltage"),

	/* PSU electrical readings.  The input side is absent or "N/A" where the
	 * hardware has no per-PSU PMBus, and is then served as NotFound. */
	"outputPower":         stateFloat([]componentType{CompTypePsu}, PSU_TBL, "power", "output_power"),
	"outputVoltageSensor": stateFloat([]componentType{CompTypePsu}, PSU_TBL, "voltage", "output_voltage"),
	"inputVoltageSensor":  stateFloat([]componentType{CompTypePsu}, PSU_TBL, "input_voltage"),
	"inputPower":          stateFloat([]componentType{CompTypePsu}, PSU_TBL, "input_power"),

	/* FAN_INFO.speed is a percent of maximum, not an RPM count. */
	"speed":       stateFloat([]componentType{CompTypeFan}, FAN_TBL, "speed"),
	"temperature": stateFloat([]componentType{CompTypeTemp}, TEMP_TBL, "temperature"),
	/* The overheat trip point is critical_high_threshold; high_threshold is the
	 * warning level. */
	"overheat_threshold": stateFloat([]componentType{CompTypeTemp}, TEMP_TBL, "critical_high_threshold"),

	"operStatus": operStatusDescriptor(),
	"hwStatus":   fruBool("presence"),
	"active":     fruBool("status"),

	/* Host load, core count, uptime and cpu utilization, read from procfs and
	 * from sysinfo. */
	"loadAvg1m":   hostProp(hostCpuTypes, cpuLoadAvgProp(0)),
	"loadAvg5m":   hostProp(hostCpuTypes, cpuLoadAvgProp(1)),
	"loadAvg15m":  hostProp(hostCpuTypes, cpuLoadAvgProp(2)),
	"numCores":    hostProp(hostCpuTypes, cpuNumCoresProp),
	"uptime":      hostProp(hostCpuTypes, cpuUptimeProp),
	"utilization": hostProp(hostCpuTypes, cpuUtilizationProp),
}

/* True when the request targets anything under a component's properties. */
func isPropertiesPath(targetUriPath string) bool {
	return strings.HasPrefix(targetUriPath, COMP_PROPS)
}

/* True when on-change can be served on this path.  The field map names the
 * "value" leaf, which the framework resolves relative to the subscribed node, so
 * it lands correctly only on that leaf or its state parent.  Higher up, one row
 * holds many properties and a notification cannot say which one changed. */
func propOnChangeSupported(targetUriPath string) bool {
	return targetUriPath == COMP_PROP_STATE || targetUriPath == COMP_PROP_STATE_VALUE
}

/* Property named by a subscribe request, empty when the request covers the whole
 * properties container. */
func propNameFromSubscribeUri(uri, requestUri string) string {
	/* The property list key shares the name "name" with the component list, so
	 * it arrives as "name#2".  Either URI may carry it, depending on how the
	 * framework expanded the request. */
	if propName := NewPathInfo(uri).Var("name#2"); propName != "" {
		return propName
	}
	return NewPathInfo(requestUri).Var("name#2")
}

/* Table and field map a subscription on one property must watch.  ok is false
 * when the property has no database behind it, this component type does not
 * carry it, or propName is empty. */
func propSubscribeInfo(propName string, cType componentType) (table string, fields map[string]string, ok bool) {
	desc, found := propTbl[propName]
	if !found || !desc.appliesTo(cType) {
		return "", nil, false
	}
	table = desc.tableFor(cType)
	watch := desc.subFields
	if len(watch) == 0 && desc.field != "" {
		watch = []string{desc.field}
	}
	if table == "" || len(watch) == 0 {
		return "", nil, false
	}
	fields = make(map[string]string, len(watch))
	for _, f := range watch {
		fields[f] = PROP_VALUE_LEAF
	}
	return table, fields, true
}

/* Fills a component's properties.  propName selects one property, or is empty to
 * fill every property applying to the component type.  A property the type does
 * not carry, or with no value on this component, is NotFound rather than
 * empty. */
func fillProperties(comp *ocbinds.OpenconfigPlatform_Components_Component, compName string, cType componentType, propName string, dbs [db.MaxDB]*db.DB) error {
	if propName != "" {
		desc, found := propTbl[propName]
		if !found || !desc.appliesTo(cType) {
			return tlerr.NotFound("%s has no property %s", compName, propName)
		}
		return setProperty(comp, compName, cType, propName, desc, dbs)
	}

	filled := 0
	for name, desc := range propTbl {
		if !desc.appliesTo(cType) {
			continue
		}
		if err := setProperty(comp, compName, cType, name, desc, dbs); err != nil {
			log.V(lvl.DEBUG).Infof("fillProperties: %s property %s: %v", compName, name, err)
			continue
		}
		filled++
	}
	if filled == 0 {
		return tlerr.NotFound("%s has no properties", compName)
	}
	return nil
}

func setProperty(comp *ocbinds.OpenconfigPlatform_Components_Component, compName string, cType componentType, propName string, desc propDescriptor, dbs [db.MaxDB]*db.DB) error {
	value, err := desc.get(compName, cType, dbs)
	if err != nil {
		return err
	}

	ygot.BuildEmptyTree(comp)
	/* The entry may already exist, and NewProperty rejects a duplicate key. */
	prop := comp.Properties.Property[propName]
	if prop == nil {
		if prop, err = comp.Properties.NewProperty(propName); err != nil {
			return err
		}
	}
	ygot.BuildEmptyTree(prop)

	union, err := prop.State.To_OpenconfigPlatform_Components_Component_Properties_Property_State_Value_Union(value)
	if err != nil {
		return err
	}
	prop.State.Name = &propName
	prop.State.Value = union
	return nil
}

/* Reads the 1, 5 and 15 minute load averages from /proc/loadavg. */
func procLoadAvg() ([3]float64, error) {
	var loads [3]float64
	data, err := os.ReadFile(procLoadAvgPath)
	if err != nil {
		return loads, tlerr.NotFound("cannot read %s: %v", procLoadAvgPath, err)
	}
	return parseLoadAvg(string(data))
}

func parseLoadAvg(loadavg string) ([3]float64, error) {
	var loads [3]float64
	fields := strings.Fields(loadavg)
	if len(fields) < len(loads) {
		return loads, tlerr.NotFound("%s holds %d fields, want at least %d",
			procLoadAvgPath, len(fields), len(loads))
	}
	for i := range loads {
		v, err := strconv.ParseFloat(fields[i], 64)
		if err != nil {
			return [3]float64{}, err
		}
		loads[i] = v
	}
	return loads, nil
}

/* Counts the logical cpus /proc/cpuinfo lists.  Reading the file keeps the
 * answer the host's: the runtime counts only the cpus this process may run
 * on. */
func procNumCores() (uint64, error) {
	data, err := os.ReadFile(procCpuInfoPath)
	if err != nil {
		return 0, tlerr.NotFound("cannot read %s: %v", procCpuInfoPath, err)
	}
	return parseNumCores(string(data))
}

func parseNumCores(cpuinfo string) (uint64, error) {
	var cores uint64
	for _, line := range strings.Split(cpuinfo, "\n") {
		if strings.HasPrefix(line, "processor") {
			cores++
		}
	}
	if cores == 0 {
		return 0, tlerr.NotFound("%s lists no processor", procCpuInfoPath)
	}
	return cores, nil
}

/* Cumulative jiffies across every cpu since boot: total over all the fields,
 * busy over all but idle. */
type cpuJiffies struct {
	total uint64
	busy  uint64
}

func procCpuJiffies() (cpuJiffies, error) {
	data, err := os.ReadFile(procStatPath)
	if err != nil {
		return cpuJiffies{}, tlerr.NotFound("cannot read %s: %v", procStatPath, err)
	}
	return parseCpuJiffies(string(data))
}

/* Reads the aggregate line, the one named "cpu"; the per-cpu lines that follow
 * it are named "cpu0", "cpu1" and so on. */
func parseCpuJiffies(stat string) (cpuJiffies, error) {
	for _, line := range strings.Split(stat, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 6 || fields[0] != "cpu" {
			continue
		}
		var j cpuJiffies
		for i := 1; i <= cpuStatLastField && i < len(fields); i++ {
			v, err := strconv.ParseUint(fields[i], 10, 64)
			if err != nil {
				return cpuJiffies{}, err
			}
			j.total += v
			if i != cpuStatIdleField {
				j.busy += v
			}
		}
		return j, nil
	}
	return cpuJiffies{}, tlerr.NotFound("%s holds no aggregate cpu line", procStatPath)
}

/* Busy share of the cpu time that elapsed across one sample window.  Two reads
 * are needed because the counters are cumulative: one read gives the average
 * over the host's whole uptime.  The window is spent inside the request. */
func cpuUtilizationPct() (uint64, error) {
	first, err := procCpuJiffies()
	if err != nil {
		return 0, err
	}
	time.Sleep(cpuSampleWindow)
	second, err := procCpuJiffies()
	if err != nil {
		return 0, err
	}
	return cpuUtilizationBetween(first, second)
}

/* busy/total as a percent, capped at 100.  The per-cpu counters are not updated
 * at one instant, so a window this short can show more busy jiffies than total
 * ones. */
func cpuUtilizationBetween(first, second cpuJiffies) (uint64, error) {
	if second.total <= first.total {
		return 0, tlerr.NotFound("no cpu time elapsed between samples")
	}
	total := second.total - first.total
	busy := second.busy - first.busy
	if busy > total {
		return 100, nil
	}
	return (100*busy + total/2) / total, nil
}

/* Serves the host cpu utilization on a component's cpu anchor. */
func fillCpuUtilization(comp *ocbinds.OpenconfigPlatform_Components_Component) error {
	pct, err := cpuUtilizationPct()
	if err != nil {
		return err
	}
	/* The framework pre-creates the containers named in the request URI, and
	 * building a tree stops at one that already exists, so each level is built
	 * in turn. */
	ygot.BuildEmptyTree(comp)
	ygot.BuildEmptyTree(comp.Cpu)
	ygot.BuildEmptyTree(comp.Cpu.Utilization)
	instant := uint8(pct)
	comp.Cpu.Utilization.State.Instant = &instant
	return nil
}

/* Serves one of the three load averages as a property. */
func cpuLoadAvgProp(idx int) propGetter {
	return func(_ string, _ componentType, _ [db.MaxDB]*db.DB) (interface{}, error) {
		loads, err := procLoadAvg()
		if err != nil {
			return nil, err
		}
		return loads[idx], nil
	}
}

func cpuNumCoresProp(_ string, _ componentType, _ [db.MaxDB]*db.DB) (interface{}, error) {
	cores, err := procNumCores()
	if err != nil {
		return nil, err
	}
	return cores, nil
}

/* Uptime is served in nanoseconds, so that this property and
 * /system/state/up-time report one quantity in one unit. */
func cpuUptimeProp(_ string, _ componentType, _ [db.MaxDB]*db.DB) (interface{}, error) {
	_, uptime, err := getSystemBootAndUpTime()
	if err != nil {
		return nil, err
	}
	return uptime, nil
}

/* Percent busy, the same quantity the cpu anchor's instant leaf carries.  The
 * two sample separately, so a GET reading both spends two windows and can see
 * two different values. */
func cpuUtilizationProp(_ string, _ componentType, _ [db.MaxDB]*db.DB) (interface{}, error) {
	pct, err := cpuUtilizationPct()
	if err != nil {
		return nil, err
	}
	return pct, nil
}
