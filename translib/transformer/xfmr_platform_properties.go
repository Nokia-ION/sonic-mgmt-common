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
	"strconv"
	"strings"

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
)

/* Reads one property for one component.  The returned value must be a type the
 * properties/property/state/value union accepts: bool, float64, int64, string
 * or uint64. */
type propGetter func(compName string, dbs [db.MaxDB]*db.DB) (interface{}, error)

type propDescriptor struct {
	/* Component types carrying this property.  A GET on any other type is
	 * NotFound rather than empty. */
	types []componentType
	/* STATE_DB table and field the value is read from, empty for a non-DB
	 * property.  Both are needed for on-change: a subscription watches this
	 * table and matches notifications by this field name. */
	table string
	field string
	get   propGetter
}

func (d propDescriptor) appliesTo(cType componentType) bool {
	for _, t := range d.types {
		if t == cType {
			return true
		}
	}
	return false
}

/* Reads a float-valued field from a per-port transceiver table. */
func xcvrFloat(table, field string) propDescriptor {
	return propDescriptor{
		types: []componentType{CompTypeXcvr},
		table: table,
		field: field,
		get: func(compName string, dbs [db.MaxDB]*db.DB) (interface{}, error) {
			d := dbs[db.StateDB]
			if d == nil {
				return nil, tlerr.NotFound("no STATE_DB client")
			}
			entry, err := d.GetEntry(&db.TableSpec{Name: table}, db.Key{Comp: []string{compName}})
			if err != nil {
				return nil, err
			}
			/* A port with no optic has no row, and a module may report an
			 * individual field as unavailable. */
			raw := entry.Get(field)
			if raw == "" || raw == "N/A" {
				return nil, tlerr.NotFound("%s has no %s.%s", compName, table, field)
			}
			v, err := strconv.ParseFloat(raw, 64)
			if err != nil {
				return nil, err
			}
			return v, nil
		},
	}
}

func domThreshold(field string) propDescriptor {
	return xcvrFloat(TRANSCEIVER_DOM_THRESHOLD, field)
}

func domSensor(field string) propDescriptor {
	return xcvrFloat(TRANSCEIVER_DOM, field)
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
	if !found || !desc.appliesTo(cType) || desc.table == "" || desc.field == "" {
		return "", nil, false
	}
	return desc.table, map[string]string{desc.field: PROP_VALUE_LEAF}, true
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
		return setProperty(comp, compName, propName, desc, dbs)
	}

	filled := 0
	for name, desc := range propTbl {
		if !desc.appliesTo(cType) {
			continue
		}
		if err := setProperty(comp, compName, name, desc, dbs); err != nil {
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

func setProperty(comp *ocbinds.OpenconfigPlatform_Components_Component, compName, propName string, desc propDescriptor, dbs [db.MaxDB]*db.DB) error {
	value, err := desc.get(compName, dbs)
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
