package transformer

import (
	"reflect"
	"testing"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	"github.com/Azure/sonic-mgmt-common/translib/ocbinds"
	"github.com/Azure/sonic-mgmt-common/translib/tlerr"
	"github.com/openconfig/ygot/ygot"
)

// Properties read from the host rather than from a database. Naming them here
// rather than deriving them from an empty table keeps the invariant below able
// to catch a database-backed row that forgot its table.
var nonDbProps = map[string]bool{
	"loadAvg1m":   true,
	"loadAvg5m":   true,
	"loadAvg15m":  true,
	"numCores":    true,
	"uptime":      true,
	"utilization": true,
}

// Every row must name a component type and a getter. A database-backed row must
// also name the table it reads, either one table for all of its types or one per
// type and never both; a host-backed row must name neither.
func TestPropTblShape(t *testing.T) {
	if got := len(propTbl); got != 37 {
		t.Errorf("propTbl has %d rows, want 37", got)
	}
	for name, desc := range propTbl {
		if len(desc.types) == 0 {
			t.Errorf("property %q applies to no component type", name)
		}
		if desc.get == nil {
			t.Errorf("property %q has no getter", name)
		}
		if nonDbProps[name] {
			if desc.table != "" || desc.tables != nil {
				t.Errorf("property %q is read from the host but names table %q and tables %v, want neither",
					name, desc.table, desc.tables)
			}
			continue
		}
		if (desc.table == "") == (desc.tables == nil) {
			t.Errorf("property %q names table %q and tables %v, want exactly one",
				name, desc.table, desc.tables)
		}
		for _, cType := range desc.types {
			if desc.tables != nil && desc.tables[cType] == "" {
				t.Errorf("property %q applies to %v but names no table for it", name, cType)
			}
		}
	}
}

// laser-module-voltage is the only property read from the sensor table.
func TestPropTblTableSplit(t *testing.T) {
	sensors := []string{}
	for name, desc := range propTbl {
		if desc.table == TRANSCEIVER_DOM {
			sensors = append(sensors, name)
		}
	}
	if !reflect.DeepEqual(sensors, []string{"laser-module-voltage"}) {
		t.Errorf("properties read from %s are %v, want [laser-module-voltage]", TRANSCEIVER_DOM, sensors)
	}
}

func TestIsPropertiesPath(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{COMP_PROPS, true},
		{COMP_PROPS + "/property", true},
		{COMP_PROPS + "/property/state/value", true},
		{COMP, false},
		{COMP_ST, false},
		{COMP_STATE_TEMP, false},
		{"", false},
	}
	for _, c := range cases {
		if got := isPropertiesPath(c.in); got != c.want {
			t.Errorf("isPropertiesPath(%q) == %v, want %v", c.in, got, c.want)
		}
	}
}

// A subscription must resolve to the table holding the property and a map from
// its Redis field name to the value leaf. Both are required for on-change to
// fire; an empty field map makes the framework drop every notification.
func TestPropSubscribeInfo(t *testing.T) {
	// A value derived from more than one field watches all of them.
	fruFields := map[string]string{"presence": PROP_VALUE_LEAF, "status": PROP_VALUE_LEAF}
	cases := []struct {
		prop       string
		cType      componentType
		wantTable  string
		wantFields map[string]string
		wantOk     bool
	}{{
		prop:       "laser-temperature-high-alarm-threshold",
		cType:      CompTypeXcvr,
		wantTable:  TRANSCEIVER_DOM_THRESHOLD,
		wantFields: map[string]string{"temphighalarm": PROP_VALUE_LEAF},
		wantOk:     true,
	}, {
		prop:       "laser-module-voltage",
		cType:      CompTypeXcvr,
		wantTable:  TRANSCEIVER_DOM,
		wantFields: map[string]string{"voltage": PROP_VALUE_LEAF},
		wantOk:     true,
	}, {
		// A real property, but not on this component type.
		prop:  "laser-module-voltage",
		cType: CompTypePsu,
	}, {
		prop:  "no-such-property",
		cType: CompTypeXcvr,
	}, {
		// The whole properties container names no property.
		prop:  "",
		cType: CompTypeXcvr,
	}, {
		// operStatus reads a different table on each type it applies to.
		prop:       "operStatus",
		cType:      CompTypePsu,
		wantTable:  PSU_TBL,
		wantFields: fruFields,
		wantOk:     true,
	}, {
		prop:       "operStatus",
		cType:      CompTypeFan,
		wantTable:  FAN_TBL,
		wantFields: fruFields,
		wantOk:     true,
	}, {
		prop:       "operStatus",
		cType:      CompTypeFanTray,
		wantTable:  FAN_TRAY_TBL,
		wantFields: fruFields,
		wantOk:     true,
	}, {
		prop:  "operStatus",
		cType: CompTypeXcvr,
	}, {
		prop:       "outputPower",
		cType:      CompTypePsu,
		wantTable:  PSU_TBL,
		wantFields: map[string]string{"power": PROP_VALUE_LEAF},
		wantOk:     true,
	}, {
		prop:  "outputPower",
		cType: CompTypeFan,
	}, {
		// hwStatus and active each watch only the one field they serve.
		prop:       "hwStatus",
		cType:      CompTypeFanTray,
		wantTable:  FAN_TRAY_TBL,
		wantFields: map[string]string{"presence": PROP_VALUE_LEAF},
		wantOk:     true,
	}, {
		prop:       "active",
		cType:      CompTypePsu,
		wantTable:  PSU_TBL,
		wantFields: map[string]string{"status": PROP_VALUE_LEAF},
		wantOk:     true,
	}, {
		// A name with no descriptor row cannot be subscribed at all.
		prop:  "redundancyProtocol",
		cType: CompTypePsu,
	}, {
		// A property read from the host has no table to watch.
		prop:  "uptime",
		cType: CompTypeChassis,
	}, {
		prop:  "utilization",
		cType: CompTypeChassis,
	}}

	for _, c := range cases {
		gotTable, gotFields, gotOk := propSubscribeInfo(c.prop, c.cType)
		if gotTable != c.wantTable || gotOk != c.wantOk || !reflect.DeepEqual(gotFields, c.wantFields) {
			t.Errorf("propSubscribeInfo(%q, %v) == (%q, %v, %v), want (%q, %v, %v)",
				c.prop, c.cType, gotTable, gotFields, gotOk, c.wantTable, c.wantFields, c.wantOk)
		}
	}
}

// Every database-backed property must name the fields it reads, on every
// component type it applies to, or its subscription stays silent. A host-backed
// property must not be subscribable at all.
func TestPropTblEveryPropertyNamesItsField(t *testing.T) {
	for name, desc := range propTbl {
		if nonDbProps[name] {
			for _, cType := range desc.types {
				if _, _, ok := propSubscribeInfo(name, cType); ok {
					t.Errorf("property %q is subscribable on %v, want not", name, cType)
				}
			}
			continue
		}
		want := desc.subFields
		if len(want) == 0 {
			want = []string{desc.field}
		}
		if want[0] == "" {
			t.Errorf("property %q names no Redis field", name)
			continue
		}
		for _, cType := range desc.types {
			_, fields, ok := propSubscribeInfo(name, cType)
			if !ok {
				t.Errorf("property %q cannot be subscribed on %v", name, cType)
				continue
			}
			for _, f := range want {
				if got := fields[f]; got != PROP_VALUE_LEAF {
					t.Errorf("property %q on %v maps field %q to %q, want %q",
						name, cType, f, got, PROP_VALUE_LEAF)
				}
			}
		}
	}
}

// On-change is supported on the value leaf and its state parent, nowhere else.
func TestPropOnChangeSupported(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{COMP_PROP_STATE_VALUE, true},
		{COMP_PROP_STATE, true},
		// Cannot name the property list entry that changed.
		{COMP_PROPS + "/property", false},
		{COMP_PROPS, false},
		{COMP_PROPS + "/property/config/value", false},
		{COMP_ST, false},
		{"", false},
	}
	for _, c := range cases {
		if got := propOnChangeSupported(c.in); got != c.want {
			t.Errorf("propOnChangeSupported(%q) == %v, want %v", c.in, got, c.want)
		}
	}
}

// The property name is read from either URI, and is empty for the container.
func TestPropNameFromSubscribeUri(t *testing.T) {
	const compUri = "/openconfig-platform:components/component[name=Ethernet0]"
	const propUri = compUri + "/properties/property[name=laser-tx-bias-low-warn-threshold]/state/value"
	cases := []struct {
		desc       string
		uri        string
		requestUri string
		want       string
	}{{
		desc:       "both URIs name the property",
		uri:        propUri,
		requestUri: propUri,
		want:       "laser-tx-bias-low-warn-threshold",
	}, {
		// The framework expands the component key, so the property name may be
		// left in only one of the two URIs.
		desc:       "only the request URI names the property",
		uri:        compUri + "/properties",
		requestUri: "/openconfig-platform:components/component[name=*]/properties/property[name=laser-tx-bias-low-warn-threshold]/state/value",
		want:       "laser-tx-bias-low-warn-threshold",
	}, {
		desc:       "the whole container names no property",
		uri:        compUri + "/properties",
		requestUri: compUri + "/properties",
		want:       "",
	}}

	for _, c := range cases {
		if got := propNameFromSubscribeUri(c.uri, c.requestUri); got != c.want {
			t.Errorf("%s: propNameFromSubscribeUri == %q, want %q", c.desc, got, c.want)
		}
	}
}

// An unknown property name is NotFound, and leaves the component untouched.
func TestFillPropertiesUnknownProperty(t *testing.T) {
	var dbs [db.MaxDB]*db.DB
	comp := &ocbinds.OpenconfigPlatform_Components_Component{}
	err := fillProperties(comp, "Ethernet48", CompTypeXcvr, "no-such-property", dbs)
	if _, ok := err.(tlerr.NotFoundError); !ok {
		t.Fatalf("fillProperties with an unknown property returned %T (%v), want tlerr.NotFoundError", err, err)
	}
	if comp.Properties != nil {
		t.Errorf("fillProperties populated the component despite failing: %v", comp)
	}
}

// A transceiver property on a PSU is NotFound, not a zero.
func TestFillPropertiesWrongComponentType(t *testing.T) {
	var dbs [db.MaxDB]*db.DB
	comp := &ocbinds.OpenconfigPlatform_Components_Component{}
	err := fillProperties(comp, "PSU1", CompTypePsu, "laser-module-voltage", dbs)
	if _, ok := err.(tlerr.NotFoundError); !ok {
		t.Fatalf("fillProperties for the wrong component type returned %T (%v), want tlerr.NotFoundError", err, err)
	}
	if comp.Properties != nil {
		t.Errorf("fillProperties populated the component despite failing: %v", comp)
	}
}

// A component type carrying no properties reports NotFound, not success.
func TestFillPropertiesNoneApply(t *testing.T) {
	var dbs [db.MaxDB]*db.DB
	comp := &ocbinds.OpenconfigPlatform_Components_Component{}
	err := fillProperties(comp, "PSU1", CompTypePsu, "", dbs)
	if _, ok := err.(tlerr.NotFoundError); !ok {
		t.Fatalf("fillProperties on a type with no properties returned %T (%v), want tlerr.NotFoundError", err, err)
	}
}

func TestSetProperty(t *testing.T) {
	desc := propDescriptor{
		types: []componentType{CompTypeXcvr},
		table: TRANSCEIVER_DOM,
		get: func(compName string, cType componentType, dbs [db.MaxDB]*db.DB) (interface{}, error) {
			return 3.301, nil
		},
	}

	var dbs [db.MaxDB]*db.DB
	comp := &ocbinds.OpenconfigPlatform_Components_Component{}
	if err := setProperty(comp, "Ethernet48", CompTypeXcvr, "laser-module-voltage", desc, dbs); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	prop, ok := comp.Properties.Property["laser-module-voltage"]
	if !ok {
		t.Fatalf("property not created: %v", comp.Properties)
	}
	if prop.State.Name == nil || *prop.State.Name != "laser-module-voltage" {
		t.Errorf("state/name == %v, want laser-module-voltage", prop.State.Name)
	}
	// The union members are pointers to generated structs, so compare with
	// DeepEqual rather than ==, which would compare the pointers.
	want, err := prop.State.To_OpenconfigPlatform_Components_Component_Properties_Property_State_Value_Union(3.301)
	if err != nil {
		t.Fatalf("unexpected error building the expected value union: %v", err)
	}
	if !reflect.DeepEqual(prop.State.Value, want) {
		t.Errorf("state/value == %v, want %v", prop.State.Value, want)
	}
}

// An existing list entry is filled in place rather than failing as a duplicate.
func TestSetPropertyIntoPreCreatedEntry(t *testing.T) {
	desc := propDescriptor{
		types: []componentType{CompTypeXcvr},
		table: TRANSCEIVER_DOM,
		get: func(compName string, cType componentType, dbs [db.MaxDB]*db.DB) (interface{}, error) {
			return 3.301, nil
		},
	}

	var dbs [db.MaxDB]*db.DB
	comp := &ocbinds.OpenconfigPlatform_Components_Component{}
	ygot.BuildEmptyTree(comp)
	if _, err := comp.Properties.NewProperty("laser-module-voltage"); err != nil {
		t.Fatalf("pre-creating the property failed: %v", err)
	}

	if err := setProperty(comp, "Ethernet48", CompTypeXcvr, "laser-module-voltage", desc, dbs); err != nil {
		t.Fatalf("setProperty into a pre-created entry failed: %v", err)
	}
	if got := len(comp.Properties.Property); got != 1 {
		t.Errorf("emitted %d properties, want 1", got)
	}
	prop := comp.Properties.Property["laser-module-voltage"]
	if prop.State == nil || prop.State.Value == nil {
		t.Errorf("pre-created entry left unpopulated: %v", prop)
	}
}

// A getter failure leaves the property out rather than emitting a zero.
func TestSetPropertyGetterFailure(t *testing.T) {
	desc := propDescriptor{
		types: []componentType{CompTypeXcvr},
		table: TRANSCEIVER_DOM,
		get: func(compName string, cType componentType, dbs [db.MaxDB]*db.DB) (interface{}, error) {
			return nil, tlerr.NotFound("%s has no row", compName)
		},
	}

	var dbs [db.MaxDB]*db.DB
	comp := &ocbinds.OpenconfigPlatform_Components_Component{}
	if err := setProperty(comp, "Ethernet0", CompTypeXcvr, "laser-module-voltage", desc, dbs); err == nil {
		t.Fatal("setProperty returned nil error when the getter failed")
	}
	if comp.Properties != nil {
		t.Errorf("setProperty populated the component despite failing: %v", comp)
	}
}

// A getter fails rather than panics with no STATE_DB client.
func TestXcvrFloatWithoutDb(t *testing.T) {
	var dbs [db.MaxDB]*db.DB
	desc := domThreshold("temphighalarm")
	if _, err := desc.get("Ethernet48", CompTypeXcvr, dbs); err == nil {
		t.Fatal("the DOM getter returned nil error with no STATE_DB client")
	}
}

// The SONiC boolean spelling differs by table: PSU_INFO lowercase, FAN_INFO and
// FAN_DRAWER_INFO Python-capitalized.  Both must read as the same bool.
func TestParseFruBool(t *testing.T) {
	cases := map[string]bool{
		"true": true, "True": true, "TRUE": true, " true ": true,
		"false": false, "False": false, "N/A": false, "": false,
	}
	for in, want := range cases {
		if got := parseFruBool(in); got != want {
			t.Errorf("parseFruBool(%q) == %v, want %v", in, got, want)
		}
	}
}

// A whole-component GET on a PSU emits its electrical readings and operStatus,
// and omits the input-side properties whose fields read N/A.
func TestFillPropertiesPsu(t *testing.T) {
	d, err := db.NewDB(getDBOptions(db.StateDB))
	if err != nil {
		t.Fatal("NewDB failed")
	}
	defer d.DeleteDB()

	seedPsuInfoEntry(t, d, "PSU2", map[string]string{
		"presence":      "true",
		"status":        "true",
		"voltage":       "12.062",
		"current":       "36.875",
		"power":         "475.5",
		"input_voltage": "N/A",
		"input_current": "N/A",
	})
	var dbs [db.MaxDB]*db.DB
	dbs[db.StateDB] = d

	comp := &ocbinds.OpenconfigPlatform_Components_Component{}
	ygot.BuildEmptyTree(comp)
	if err := fillProperties(comp, "PSU2", CompTypePsu, "", dbs); err != nil {
		t.Fatalf("fillProperties() unexpected error: %v", err)
	}

	want := map[string]interface{}{
		"outputPower":         float64(475.5),
		"outputVoltageSensor": float64(12.062),
		"operStatus":          "ACTIVE",
	}
	for name, raw := range want {
		prop, ok := comp.Properties.Property[name]
		if !ok {
			t.Errorf("property %q missing", name)
			continue
		}
		wantUnion, err := prop.State.To_OpenconfigPlatform_Components_Component_Properties_Property_State_Value_Union(raw)
		if err != nil {
			t.Fatalf("building union for %q: %v", name, err)
		}
		if !reflect.DeepEqual(prop.State.Value, wantUnion) {
			t.Errorf("%s value == %v, want %v", name, prop.State.Value, wantUnion)
		}
	}
	for _, name := range []string{"inputPower", "inputVoltageSensor"} {
		if _, ok := comp.Properties.Property[name]; ok {
			t.Errorf("property %q present, want absent (N/A backend)", name)
		}
	}
}

// operStatus reuses the native oper-status derivation across all three FRU types.
func TestFillPropertiesOperStatus(t *testing.T) {
	d, err := db.NewDB(getDBOptions(db.StateDB))
	if err != nil {
		t.Fatal("NewDB failed")
	}
	defer d.DeleteDB()
	var dbs [db.MaxDB]*db.DB
	dbs[db.StateDB] = d

	cases := []struct {
		name     string
		cType    componentType
		presence string
		status   string
		want     string
	}{
		{"Fan1", CompTypeFan, "True", "True", "ACTIVE"},
		{"Fan2", CompTypeFan, "True", "False", "INACTIVE"},
		{"Fan3", CompTypeFan, "False", "False", "DISABLED"},
		{"PSU2", CompTypePsu, "true", "true", "ACTIVE"},
		{"PSU1", CompTypePsu, "true", "false", "INACTIVE"},
		{"drawer1", CompTypeFanTray, "True", "True", "ACTIVE"},
		{"drawer2", CompTypeFanTray, "False", "False", "DISABLED"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fields := map[string]string{"presence": c.presence, "status": c.status}
			switch c.cType {
			case CompTypePsu:
				seedPsuInfoEntry(t, d, c.name, fields)
			case CompTypeFanTray:
				seedFanDrawerInfoEntry(t, d, c.name, fields)
			default:
				seedFanInfoEntry(t, d, c.name, fields)
			}
			comp := &ocbinds.OpenconfigPlatform_Components_Component{}
			ygot.BuildEmptyTree(comp)
			if err := fillProperties(comp, c.name, c.cType, "operStatus", dbs); err != nil {
				t.Fatalf("fillProperties() unexpected error: %v", err)
			}
			prop := comp.Properties.Property["operStatus"]
			if prop == nil || prop.State == nil {
				t.Fatalf("operStatus not populated")
			}
			wantUnion, err := prop.State.To_OpenconfigPlatform_Components_Component_Properties_Property_State_Value_Union(c.want)
			if err != nil {
				t.Fatalf("building union: %v", err)
			}
			if !reflect.DeepEqual(prop.State.Value, wantUnion) {
				t.Errorf("operStatus == %v, want %v", prop.State.Value, wantUnion)
			}
		})
	}
}

// hwStatus and active serve the two bits operStatus combines, as booleans rather
// than as the oper-status enum.
func TestFillPropertiesFruBool(t *testing.T) {
	d, err := db.NewDB(getDBOptions(db.StateDB))
	if err != nil {
		t.Fatal("NewDB failed")
	}
	defer d.DeleteDB()
	var dbs [db.MaxDB]*db.DB
	dbs[db.StateDB] = d

	seedFanDrawerInfoEntry(t, d, "drawer3", map[string]string{"presence": "True", "status": "False"})

	comp := &ocbinds.OpenconfigPlatform_Components_Component{}
	ygot.BuildEmptyTree(comp)
	if err := fillProperties(comp, "drawer3", CompTypeFanTray, "", dbs); err != nil {
		t.Fatalf("fillProperties() unexpected error: %v", err)
	}

	want := map[string]interface{}{
		"hwStatus":   true,
		"active":     false,
		"operStatus": "INACTIVE",
	}
	for name, raw := range want {
		prop, ok := comp.Properties.Property[name]
		if !ok {
			t.Errorf("property %q missing", name)
			continue
		}
		wantUnion, err := prop.State.To_OpenconfigPlatform_Components_Component_Properties_Property_State_Value_Union(raw)
		if err != nil {
			t.Fatalf("building union for %q: %v", name, err)
		}
		if !reflect.DeepEqual(prop.State.Value, wantUnion) {
			t.Errorf("%s value == %v, want %v", name, prop.State.Value, wantUnion)
		}
	}
}

// A FRU with no row at all reports NotFound for each of its status properties
// rather than defaulting them to false.
func TestFillPropertiesFruBoolNoRow(t *testing.T) {
	d, err := db.NewDB(getDBOptions(db.StateDB))
	if err != nil {
		t.Fatal("NewDB failed")
	}
	defer d.DeleteDB()
	var dbs [db.MaxDB]*db.DB
	dbs[db.StateDB] = d

	comp := &ocbinds.OpenconfigPlatform_Components_Component{}
	for _, name := range []string{"hwStatus", "active", "operStatus"} {
		if err := fillProperties(comp, "drawer9", CompTypeFanTray, name, dbs); err == nil {
			t.Errorf("fillProperties(%q) on an absent drawer returned nil error", name)
		}
	}
	if comp.Properties != nil {
		t.Errorf("fillProperties populated the component despite failing: %v", comp)
	}
}

// A property with no descriptor row must be NotFound, not an empty component.
func TestFillPropertiesOmittedNotFound(t *testing.T) {
	d, err := db.NewDB(getDBOptions(db.StateDB))
	if err != nil {
		t.Fatal("NewDB failed")
	}
	defer d.DeleteDB()
	seedPsuInfoEntry(t, d, "PSU2", map[string]string{"presence": "true", "status": "true"})
	var dbs [db.MaxDB]*db.DB
	dbs[db.StateDB] = d

	comp := &ocbinds.OpenconfigPlatform_Components_Component{}
	ygot.BuildEmptyTree(comp)
	for _, name := range []string{"redundancyProtocol", "adminStatus", "slotType", "routedAddr"} {
		err := fillProperties(comp, "PSU2", CompTypePsu, name, dbs)
		if _, ok := err.(tlerr.NotFoundError); !ok {
			t.Errorf("fillProperties(%q) returned %T (%v), want tlerr.NotFoundError", name, err, err)
		}
	}
}

// The three load averages are the first three fields, and a two-decimal reading
// parses to exactly that decimal rather than to a scaled approximation of it.
func TestParseLoadAvg(t *testing.T) {
	loads, err := parseLoadAvg("0.11 0.36 0.62 2/1215 2780670\n")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := [3]float64{0.11, 0.36, 0.62}
	if loads != want {
		t.Errorf("parseLoadAvg == %v, want %v", loads, want)
	}
}

// A short or unparseable file is an error, so the property drops out instead of
// serving a zero load.
func TestParseLoadAvgRejectsBadInput(t *testing.T) {
	for _, in := range []string{"", "0.11", "0.11 0.36", "0.11 0.36 x 2/1215 1"} {
		if _, err := parseLoadAvg(in); err == nil {
			t.Errorf("parseLoadAvg(%q) returned nil error", in)
		}
	}
}

func TestParseNumCores(t *testing.T) {
	const cpuinfo = `processor	: 0
cpu cores	: 8

processor	: 1
cpu cores	: 8
`
	cores, err := parseNumCores(cpuinfo)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cores != 2 {
		t.Errorf("parseNumCores == %d, want 2", cores)
	}
}

// A file listing no processor is an error rather than a count of zero.
func TestParseNumCoresNoProcessor(t *testing.T) {
	if _, err := parseNumCores("cpu cores\t: 8\n"); err == nil {
		t.Error("parseNumCores on a file with no processor returned nil error")
	}
}

// Only the aggregate line is read, and idle is the field left out of busy.
func TestParseCpuJiffies(t *testing.T) {
	const stat = `cpu  100 2 30 800 60 0 8 0 0 0
cpu0 50 1 15 400 30 0 4 0 0 0
cpu1 50 1 15 400 30 0 4 0 0 0
intr 12345
`
	j, err := parseCpuJiffies(stat)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := uint64(1000); j.total != want {
		t.Errorf("total == %d, want %d", j.total, want)
	}
	if want := uint64(200); j.busy != want {
		t.Errorf("busy == %d, want %d", j.busy, want)
	}
}

// guest and guest_nice are already counted inside user and nice, so summing
// them again would inflate both totals.
func TestParseCpuJiffiesExcludesGuest(t *testing.T) {
	j, err := parseCpuJiffies("cpu  100 2 30 800 60 0 8 0 500 7\n")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := uint64(1000); j.total != want {
		t.Errorf("total == %d, want %d", j.total, want)
	}
	if want := uint64(200); j.busy != want {
		t.Errorf("busy == %d, want %d", j.busy, want)
	}
}

func TestParseCpuJiffiesRejectsBadInput(t *testing.T) {
	for _, in := range []string{"", "intr 12345\n", "cpu0 1 2 3 4 5 6\n", "cpu x 2 3 4 5 6\n"} {
		if _, err := parseCpuJiffies(in); err == nil {
			t.Errorf("parseCpuJiffies(%q) returned nil error", in)
		}
	}
}

func TestCpuUtilizationBetween(t *testing.T) {
	cases := []struct {
		desc  string
		first cpuJiffies
		last  cpuJiffies
		want  uint64
	}{
		{"fully idle", cpuJiffies{total: 1000, busy: 100}, cpuJiffies{total: 1160, busy: 100}, 0},
		{"fully busy", cpuJiffies{total: 1000, busy: 100}, cpuJiffies{total: 1160, busy: 260}, 100},
		{"half busy", cpuJiffies{total: 1000, busy: 100}, cpuJiffies{total: 1160, busy: 180}, 50},
		{"rounds up", cpuJiffies{total: 0, busy: 0}, cpuJiffies{total: 160, busy: 3}, 2},
		{"busy beyond total is capped", cpuJiffies{total: 1000, busy: 100}, cpuJiffies{total: 1160, busy: 400}, 100},
	}
	for _, c := range cases {
		got, err := cpuUtilizationBetween(c.first, c.last)
		if err != nil {
			t.Errorf("%s: unexpected error: %v", c.desc, err)
			continue
		}
		if got != c.want {
			t.Errorf("%s: cpuUtilizationBetween == %d, want %d", c.desc, got, c.want)
		}
	}
}

// Two samples with no time between them cannot yield a percentage.
func TestCpuUtilizationBetweenNoElapsedTime(t *testing.T) {
	for _, c := range []struct {
		desc  string
		first cpuJiffies
		last  cpuJiffies
	}{
		{"same reading", cpuJiffies{total: 1000, busy: 100}, cpuJiffies{total: 1000, busy: 100}},
		{"counters went backwards", cpuJiffies{total: 1000, busy: 100}, cpuJiffies{total: 900, busy: 90}},
	} {
		if _, err := cpuUtilizationBetween(c.first, c.last); err == nil {
			t.Errorf("%s: cpuUtilizationBetween returned nil error", c.desc)
		}
	}
}

// The filler sets the instant leaf and leaves the rest of the component alone.
func TestFillCpuUtilization(t *testing.T) {
	comp := &ocbinds.OpenconfigPlatform_Components_Component{}
	if err := fillCpuUtilization(comp); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if comp.Cpu == nil || comp.Cpu.Utilization == nil || comp.Cpu.Utilization.State == nil {
		t.Fatalf("cpu utilization state not built: %v", comp.Cpu)
	}
	instant := comp.Cpu.Utilization.State.Instant
	if instant == nil {
		t.Fatal("instant not set")
	}
	if *instant > 100 {
		t.Errorf("instant == %d, want at most 100", *instant)
	}
	if comp.State != nil && comp.State.Name != nil {
		t.Errorf("filler touched state/name: %v", *comp.State.Name)
	}
}

// The host properties are served on the chassis and nowhere else.
func TestFillPropertiesHostCpuOnlyOnChassis(t *testing.T) {
	var dbs [db.MaxDB]*db.DB
	for name := range nonDbProps {
		comp := &ocbinds.OpenconfigPlatform_Components_Component{}
		err := fillProperties(comp, "CPU", CompTypeTemp, name, dbs)
		if _, ok := err.(tlerr.NotFoundError); !ok {
			t.Errorf("fillProperties(%q) on a sensor returned %T (%v), want tlerr.NotFoundError", name, err, err)
		}
	}
}

// capacity has no source and so has no descriptor row at all.
func TestFillPropertiesCapacityNotFound(t *testing.T) {
	var dbs [db.MaxDB]*db.DB
	comp := &ocbinds.OpenconfigPlatform_Components_Component{}
	err := fillProperties(comp, "chassis 1", CompTypeChassis, "capacity", dbs)
	if _, ok := err.(tlerr.NotFoundError); !ok {
		t.Fatalf("fillProperties(capacity) returned %T (%v), want tlerr.NotFoundError", err, err)
	}
	if comp.Properties != nil {
		t.Errorf("fillProperties populated the component despite failing: %v", comp)
	}
}

// Every host property is served on a chassis without a database client, since
// none of them reads one.
func TestFillPropertiesHostCpuWithoutDb(t *testing.T) {
	var dbs [db.MaxDB]*db.DB
	comp := &ocbinds.OpenconfigPlatform_Components_Component{}
	if err := fillProperties(comp, "chassis 1", CompTypeChassis, "", dbs); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for name := range nonDbProps {
		prop, ok := comp.Properties.Property[name]
		if !ok {
			t.Errorf("property %q missing", name)
			continue
		}
		if prop.State == nil || prop.State.Value == nil {
			t.Errorf("property %q has no value", name)
		}
	}
	if _, ok := comp.Properties.Property["capacity"]; ok {
		t.Error("property capacity present, want absent")
	}
}

func assertPropFloat(t *testing.T, comp *ocbinds.OpenconfigPlatform_Components_Component, name string, want float64) {
	t.Helper()
	prop, ok := comp.Properties.Property[name]
	if !ok {
		t.Fatalf("property %q missing", name)
	}
	wantUnion, err := prop.State.To_OpenconfigPlatform_Components_Component_Properties_Property_State_Value_Union(want)
	if err != nil {
		t.Fatalf("building union for %q: %v", name, err)
	}
	if !reflect.DeepEqual(prop.State.Value, wantUnion) {
		t.Errorf("property %q = %v, want %v", name, prop.State.Value, wantUnion)
	}
}

// speed is served on fans, temperature and overheat_threshold on sensors, and a
// property is NotFound on a type that does not carry it.
func TestFillPropertiesFanAndSensor(t *testing.T) {
	d, err := db.NewDB(getDBOptions(db.StateDB))
	if err != nil {
		t.Fatal("NewDB failed")
	}
	defer d.DeleteDB()

	seedFanInfoEntry(t, d, "Fan3", map[string]string{
		"speed": "44",
	})
	seedTempInfoEntry(t, d, "MB Front", map[string]string{
		"temperature":             "26.75",
		"high_threshold":          "70",
		"critical_high_threshold": "73",
	})
	var dbs [db.MaxDB]*db.DB
	dbs[db.StateDB] = d

	t.Run("fan speed", func(t *testing.T) {
		comp := &ocbinds.OpenconfigPlatform_Components_Component{}
		if err := fillProperties(comp, "Fan3", CompTypeFan, "speed", dbs); err != nil {
			t.Fatalf("fillProperties(speed) unexpected error: %v", err)
		}
		assertPropFloat(t, comp, "speed", 44)
	})
	t.Run("sensor temperature and overheat", func(t *testing.T) {
		comp := &ocbinds.OpenconfigPlatform_Components_Component{}
		if err := fillProperties(comp, "MB Front", CompTypeTemp, "", dbs); err != nil {
			t.Fatalf("fillProperties() unexpected error: %v", err)
		}
		assertPropFloat(t, comp, "temperature", 26.75)
		assertPropFloat(t, comp, "overheat_threshold", 73)
		if _, ok := comp.Properties.Property["speed"]; ok {
			t.Error("property speed present on a sensor, want absent")
		}
	})
	t.Run("temperature on a fan", func(t *testing.T) {
		comp := &ocbinds.OpenconfigPlatform_Components_Component{}
		err := fillProperties(comp, "Fan3", CompTypeFan, "temperature", dbs)
		if _, ok := err.(tlerr.NotFoundError); !ok {
			t.Fatalf("fillProperties(temperature on fan) returned %T (%v), want tlerr.NotFoundError", err, err)
		}
	})
}
