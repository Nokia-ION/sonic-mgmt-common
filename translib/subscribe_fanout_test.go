////////////////////////////////////////////////////////////////////////////////
//                                                                            //
//  Licensed under the Apache License, Version 2.0 (the "License");           //
//  you may not use this file except in compliance with the License.          //
//  You may obtain a copy of the License at                                   //
//                                                                            //
//     http://www.apache.org/licenses/LICENSE-2.0                             //
//                                                                            //
//  Unless required by applicable law or agreed to in writing, software       //
//  distributed under the License is distributed on an "AS IS" BASIS,         //
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.  //
//  See the License for the specific language governing permissions and       //
//  limitations under the License.                                            //
//                                                                            //
////////////////////////////////////////////////////////////////////////////////

package translib

import (
	"sort"
	"testing"

	"github.com/Azure/sonic-mgmt-common/translib/ocbinds"
	"github.com/openconfig/gnmi/proto/gnmi"
	"github.com/openconfig/ygot/ygot"
)

func fanoutTestPath(t *testing.T, s string) *gnmi.Path {
	t.Helper()
	p, err := ygot.StringToStructuredPath(s)
	if err != nil {
		t.Fatalf("bad path %q: %v", s, err)
	}
	return p
}

func TestFanoutListIndex(t *testing.T) {
	tests := []struct {
		path string
		want int
	}{
		// Nothing left to fan out.
		{"/openconfig-platform:components/component[name=Fan1]/properties/property[name=speed]/state/value", -1},
		// No concrete instance above the wildcard: the read would cover every component.
		{"/openconfig-platform:components/component[name=*]/properties/property[name=*]/state/value", -1},
		{"/openconfig-platform:components/component[name=*]/state", -1},
		// Served below the named component.
		{"/openconfig-platform:components/component[name=Fan1]/properties/property[name=*]/state/value", 3},
		{"/openconfig-platform:components/component[name=Ethernet0]/transceiver/physical-channels/channel[index=*]/state", 4},
	}
	for _, tt := range tests {
		if got := fanoutListIndex(fanoutTestPath(t, tt.path)); got != tt.want {
			t.Errorf("fanoutListIndex(%s) = %d, want %d", tt.path, got, tt.want)
		}
	}
}

func fanoutTestComponent(t *testing.T, comps *ocbinds.OpenconfigPlatform_Components, name string, props map[string]float64) {
	t.Helper()
	comp, err := comps.NewComponent(name)
	if err != nil {
		t.Fatalf("NewComponent(%s): %v", name, err)
	}
	ygot.BuildEmptyTree(comp)
	for pname, pval := range props {
		prop, err := comp.Properties.NewProperty(pname)
		if err != nil {
			t.Fatalf("NewProperty(%s): %v", pname, err)
		}
		ygot.BuildEmptyTree(prop)
		v, err := prop.State.To_OpenconfigPlatform_Components_Component_Properties_Property_State_Value_Union(pval)
		if err != nil {
			t.Fatalf("value union for %s: %v", pname, err)
		}
		prop.State.Name = ygot.String(pname)
		prop.State.Value = v
	}
}

// leafPaths renders a pruned struct the way the gNMI server does, and returns
// the update paths.
func leafPaths(t *testing.T, y ygot.ValidatedGoStruct) []string {
	t.Helper()
	notifs, err := ygot.TogNMINotifications(y, 0, ygot.GNMINotificationsConfig{UsePathElem: true})
	if err != nil {
		t.Fatalf("TogNMINotifications: %v", err)
	}
	var paths []string
	for _, u := range notifs[0].Update {
		s, err := ygot.PathToString(u.Path)
		if err != nil {
			t.Fatalf("PathToString: %v", err)
		}
		paths = append(paths, s)
	}
	sort.Strings(paths)
	return paths
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestPruneYgotToPatternLeafUnderWildcard(t *testing.T) {
	comps := &ocbinds.OpenconfigPlatform_Components{}
	fanoutTestComponent(t, comps, "PSU 1", map[string]float64{"outputPower": 120, "inputPower": 130})
	props := comps.Component["PSU 1"].Properties
	// An instance without state carries no state/value and must drop out.
	if _, err := props.NewProperty("bare"); err != nil {
		t.Fatalf("NewProperty(bare): %v", err)
	}

	got, err := pruneYgotToPattern(props, fanoutTestPath(t, "/property[name=*]/state/value"))
	if err != nil {
		t.Fatalf("pruneYgotToPattern: %v", err)
	}
	if got == nil {
		t.Fatalf("pruneYgotToPattern returned nil")
	}
	pruned := got.(*ocbinds.OpenconfigPlatform_Components_Component_Properties)
	if len(pruned.Property) != 2 {
		t.Fatalf("got %d properties, want 2: %v", len(pruned.Property), leafPaths(t, pruned))
	}
	for name, p := range pruned.Property {
		if p.State == nil || p.State.Value == nil {
			t.Errorf("property %s lost its value", name)
		}
		if p.State != nil && p.State.Name != nil {
			t.Errorf("property %s kept state/name, which the pattern does not name", name)
		}
	}
	// The source tree is left alone.
	if props.Property["outputPower"].State.Name == nil {
		t.Errorf("source property lost state/name")
	}

	want := []string{
		"/property[name=inputPower]/name",
		"/property[name=inputPower]/state/value",
		"/property[name=outputPower]/name",
		"/property[name=outputPower]/state/value",
	}
	if paths := leafPaths(t, pruned); !equalStrings(paths, want) {
		t.Errorf("rendered paths = %v, want %v", paths, want)
	}
}

func TestPruneYgotToPatternTwoWildcards(t *testing.T) {
	comps := &ocbinds.OpenconfigPlatform_Components{}
	fanoutTestComponent(t, comps, "Fan1", map[string]float64{"speed": 40})
	fanoutTestComponent(t, comps, "Fan2", map[string]float64{"speed": 60})
	fanoutTestComponent(t, comps, "chassis 1", nil)

	got, err := pruneYgotToPattern(comps,
		fanoutTestPath(t, "/component[name=*]/properties/property[name=*]/state/value"))
	if err != nil {
		t.Fatalf("pruneYgotToPattern: %v", err)
	}
	if got == nil {
		t.Fatalf("pruneYgotToPattern returned nil")
	}
	want := []string{
		"/component[name=Fan1]/name",
		"/component[name=Fan1]/properties/property[name=speed]/name",
		"/component[name=Fan1]/properties/property[name=speed]/state/value",
		"/component[name=Fan2]/name",
		"/component[name=Fan2]/properties/property[name=speed]/name",
		"/component[name=Fan2]/properties/property[name=speed]/state/value",
	}
	if paths := leafPaths(t, got); !equalStrings(paths, want) {
		t.Errorf("rendered paths = %v, want %v", paths, want)
	}
}

func TestPruneYgotToPatternContainerUnderWildcard(t *testing.T) {
	comps := &ocbinds.OpenconfigPlatform_Components{}
	fanoutTestComponent(t, comps, "Fan1", map[string]float64{"speed": 40, "temperature": 30})
	props := comps.Component["Fan1"].Properties

	got, err := pruneYgotToPattern(props, fanoutTestPath(t, "/property[name=*]/state"))
	if err != nil {
		t.Fatalf("pruneYgotToPattern: %v", err)
	}
	want := []string{
		"/property[name=speed]/name",
		"/property[name=speed]/state/name",
		"/property[name=speed]/state/value",
		"/property[name=temperature]/name",
		"/property[name=temperature]/state/name",
		"/property[name=temperature]/state/value",
	}
	if paths := leafPaths(t, got); !equalStrings(paths, want) {
		t.Errorf("rendered paths = %v, want %v", paths, want)
	}
}

func TestPruneYgotToPatternNoMatch(t *testing.T) {
	comps := &ocbinds.OpenconfigPlatform_Components{}
	fanoutTestComponent(t, comps, "chassis 1", nil)
	props := comps.Component["chassis 1"].Properties

	got, err := pruneYgotToPattern(props, fanoutTestPath(t, "/property[name=*]/state/value"))
	if err != nil {
		t.Fatalf("pruneYgotToPattern: %v", err)
	}
	if got != nil {
		t.Errorf("got %v, want nil for a component with no properties", leafPaths(t, got))
	}
}
