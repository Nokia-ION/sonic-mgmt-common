package transformer

import (
	"reflect"
	"testing"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	"github.com/Azure/sonic-mgmt-common/translib/ocbinds"
	"github.com/Azure/sonic-mgmt-common/translib/tlerr"
	"github.com/openconfig/ygot/ygot"
)

// Every row must name a component type, a transceiver DOM table and a getter.
func TestPropTblShape(t *testing.T) {
	if got := len(propTbl); got != 21 {
		t.Errorf("propTbl has %d rows, want 21", got)
	}
	for name, desc := range propTbl {
		if len(desc.types) == 0 {
			t.Errorf("property %q applies to no component type", name)
		}
		if desc.get == nil {
			t.Errorf("property %q has no getter", name)
		}
		switch desc.table {
		case TRANSCEIVER_DOM_THRESHOLD, TRANSCEIVER_DOM:
		default:
			t.Errorf("property %q reads table %q, want a transceiver DOM table", name, desc.table)
		}
		if !desc.appliesTo(CompTypeXcvr) {
			t.Errorf("property %q does not apply to CompTypeXcvr", name)
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
	}}

	for _, c := range cases {
		gotTable, gotFields, gotOk := propSubscribeInfo(c.prop, c.cType)
		if gotTable != c.wantTable || gotOk != c.wantOk || !reflect.DeepEqual(gotFields, c.wantFields) {
			t.Errorf("propSubscribeInfo(%q, %v) == (%q, %v, %v), want (%q, %v, %v)",
				c.prop, c.cType, gotTable, gotFields, gotOk, c.wantTable, c.wantFields, c.wantOk)
		}
	}
}

// Every property must name the field it reads, or its subscription stays silent.
func TestPropTblEveryPropertyNamesItsField(t *testing.T) {
	for name, desc := range propTbl {
		if desc.field == "" {
			t.Errorf("property %q names no Redis field", name)
			continue
		}
		_, fields, ok := propSubscribeInfo(name, CompTypeXcvr)
		if !ok {
			t.Errorf("property %q cannot be subscribed on CompTypeXcvr", name)
			continue
		}
		if got := fields[desc.field]; got != PROP_VALUE_LEAF {
			t.Errorf("property %q maps field %q to %q, want %q", name, desc.field, got, PROP_VALUE_LEAF)
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
		get: func(compName string, dbs [db.MaxDB]*db.DB) (interface{}, error) {
			return 3.301, nil
		},
	}

	var dbs [db.MaxDB]*db.DB
	comp := &ocbinds.OpenconfigPlatform_Components_Component{}
	if err := setProperty(comp, "Ethernet48", "laser-module-voltage", desc, dbs); err != nil {
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
		get: func(compName string, dbs [db.MaxDB]*db.DB) (interface{}, error) {
			return 3.301, nil
		},
	}

	var dbs [db.MaxDB]*db.DB
	comp := &ocbinds.OpenconfigPlatform_Components_Component{}
	ygot.BuildEmptyTree(comp)
	if _, err := comp.Properties.NewProperty("laser-module-voltage"); err != nil {
		t.Fatalf("pre-creating the property failed: %v", err)
	}

	if err := setProperty(comp, "Ethernet48", "laser-module-voltage", desc, dbs); err != nil {
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
		get: func(compName string, dbs [db.MaxDB]*db.DB) (interface{}, error) {
			return nil, tlerr.NotFound("%s has no row", compName)
		},
	}

	var dbs [db.MaxDB]*db.DB
	comp := &ocbinds.OpenconfigPlatform_Components_Component{}
	if err := setProperty(comp, "Ethernet0", "laser-module-voltage", desc, dbs); err == nil {
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
	if _, err := desc.get("Ethernet48", dbs); err == nil {
		t.Fatal("the DOM getter returned nil error with no STATE_DB client")
	}
}
