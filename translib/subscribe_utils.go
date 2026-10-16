////////////////////////////////////////////////////////////////////////////////
//                                                                            //
//  Copyright 2021 Broadcom. The term Broadcom refers to Broadcom Inc. and/or //
//  its subsidiaries.                                                         //
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
	"fmt"
	"reflect"
	"strings"
	"sync/atomic"

	"github.com/Azure/sonic-mgmt-common/translib/ocbinds"
	"github.com/Azure/sonic-mgmt-common/translib/path"
	"github.com/Azure/sonic-mgmt-common/translib/utils"
	"github.com/kylelemons/godebug/pretty"
	"github.com/openconfig/gnmi/proto/gnmi"
	"github.com/openconfig/goyang/pkg/yang"
	"github.com/openconfig/ygot/util"
	"github.com/openconfig/ygot/ygot"
	"github.com/openconfig/ygot/ytypes"
)

var objPrinter = &pretty.Config{
	Compact:        true,
	SkipZeroFields: true,
	Formatter:      pretty.DefaultFormatter,
}

// Bits is a set of 8 bit flags.
type Bits = utils.Bits

// Counter is a monotonically increasing unsigned integer.
type Counter uint64

// Next increments the counter and returns the new value
func (c *Counter) Next() uint64 {
	return atomic.AddUint64((*uint64)(c), 1)
}

// mergeYgotAtPathStr merges a ygot.ValidatedGoStruct at a given path in ygot tree ygRoot.
// Returns the target node from the ygot tree.
func mergeYgotAtPathStr(ygRoot *ocbinds.Device, pathStr string, value ygot.ValidatedGoStruct) (ygot.ValidatedGoStruct, error) {
	p, err := ygot.StringToStructuredPath(pathStr)
	if err != nil {
		return nil, err
	}

	return mergeYgotAtPath(ygRoot, p, value)
}

// mergeYgotAtPathStr merges a ygot.ValidatedGoStruct at a given path in ygot tree ygRoot.
// Returns the target node from the ygot tree.
// This function may update the input path!
func mergeYgotAtPath(ygRoot *ocbinds.Device, p *gnmi.Path, value ygot.ValidatedGoStruct) (ygot.ValidatedGoStruct, error) {
	var pNode ygot.ValidatedGoStruct
	path.RemoveModulePrefix(p)

	ygNode, _, err := ytypes.GetOrCreateNode(ygSchema.RootSchema(), ygRoot, p)
	if err != nil {
		return nil, err
	}
	if n, ok := ygNode.(ygot.ValidatedGoStruct); ok {
		pNode = n
	} else {
		return nil, fmt.Errorf("GetOrCreateNode returned %T; is not a ValidatedGoStruct", ygNode)
	}
	if value != nil {
		err = ygot.MergeStructInto(pNode, value)
	}
	return pNode, err
}

// clearListKeys sets the list key fields of a ygot struct to their zero values.
// Noop if the ygot obj is not a yang list (i.e, not a ygot.KeyHelperGoStruct).
func clearListKeys(y ygot.ValidatedGoStruct) error {
	yList, ok := y.(ygot.KeyHelperGoStruct)
	if !ok { // not a list node
		return nil
	}

	keyMap, err := yList.ΛListKeyMap()
	if err != nil {
		return err
	}

	yv := reflect.ValueOf(y).Elem()
	yt := yv.Type()
	for i := 0; i < yv.NumField(); i++ {
		ft := yt.Field(i)
		pathTag, _ := ft.Tag.Lookup("path")
		if _, ok := keyMap[pathTag]; ok {
			yv.Field(i).Set(reflect.Zero(ft.Type))
		}
	}

	return nil
}

// getYgotAtPath looks up the child ygot struct at a childPath in a parent struct.
// Absolute path to the parent struct from root should also be passed.
// Returns error if childPath does not point to a struct.
// Warning: this function may update the childPath by removing module prefixes.
func getYgotAtPath(parent ygot.ValidatedGoStruct, childPath *gnmi.Path) (ygot.ValidatedGoStruct, error) {
	structName := reflect.TypeOf(parent).Elem().Name()
	schema, ok := ocbinds.SchemaTree[structName]
	if !ok {
		return nil, fmt.Errorf("Could not find schema for %T", parent)
	}

	path.RemoveModulePrefix(childPath)
	nodes, err := ytypes.GetNode(schema, parent, childPath)
	if err != nil {
		return nil, err
	}

	if yv, ok := nodes[0].Data.(ygot.ValidatedGoStruct); ok {
		return yv, nil
	}

	return nil, fmt.Errorf("Not a ValidatedGoStruct")
}

// fanoutListIndex returns the index of the first wildcarded list in p, when the
// wildcards p still carries can be served by reading below one list instance:
// a list above the first wildcard has concrete keys, so the read covers that
// instance alone. Returns -1 when p carries no wildcard or no such list.
func fanoutListIndex(p *gnmi.Path) int {
	keyedAbove := false
	for i, e := range p.GetElem() {
		if path.HasWildcardAtKey(p, i) {
			if keyedAbove {
				return i
			}
			return -1
		}
		if len(e.Key) != 0 {
			keyedAbove = true
		}
	}
	return -1
}

// pruneYgotToPattern returns a new struct of node's type holding only the nodes
// pattern names below node. pattern may carry wildcard keys; each list instance
// they match is kept under its own keys, and an instance that does not carry
// the named node is left out. Returns nil when nothing matches.
func pruneYgotToPattern(node ygot.ValidatedGoStruct, pattern *gnmi.Path) (ygot.ValidatedGoStruct, error) {
	schema, ok := ocbinds.SchemaTree[reflect.TypeOf(node).Elem().Name()]
	if !ok {
		return nil, fmt.Errorf("Could not find schema for %T", node)
	}
	rel := path.Clone(pattern)
	path.RemoveModulePrefix(rel)

	matches := collectYgotMatches(schema, node, nil, rel.Elem)
	if len(matches) == 0 {
		return nil, nil
	}

	pruned, ok := reflect.New(reflect.TypeOf(node).Elem()).Interface().(ygot.ValidatedGoStruct)
	if !ok {
		return nil, fmt.Errorf("%T is not a ValidatedGoStruct", node)
	}
	for _, m := range matches {
		if err := mergeYgotMatch(schema, pruned, node, m); err != nil {
			return nil, err
		}
	}
	return pruned, nil
}

// ygotMatch is a node found below a ygot struct, at a path relative to it with
// every key concrete.
type ygotMatch struct {
	elems []*gnmi.PathElem
	data  interface{}
}

// collectYgotMatches returns the nodes that rest names below node, which sits
// at the relative path at. Wildcards are expanded one list at a time, so that
// an instance missing a node below it drops out instead of failing the lookup
// for its siblings.
func collectYgotMatches(schema *yang.Entry, node interface{}, at, rest []*gnmi.PathElem) []ygotMatch {
	if len(rest) == 0 {
		if util.IsValueNil(node) {
			return nil
		}
		return []ygotMatch{{elems: at, data: node}}
	}

	step := len(rest)
	for i := range rest {
		if path.HasWildcardAtKey(&gnmi.Path{Elem: rest}, i) {
			step = i + 1
			break
		}
	}

	nodes, err := ytypes.GetNode(schema, node, &gnmi.Path{Elem: rest[:step]}, &ytypes.GetHandleWildcards{})
	if err != nil {
		return nil
	}

	var matches []ygotMatch
	for _, n := range nodes {
		next := make([]*gnmi.PathElem, 0, len(at)+len(n.Path.GetElem()))
		next = append(next, at...)
		next = append(next, n.Path.GetElem()...)
		matches = append(matches, collectYgotMatches(n.Schema, n.Data, next, rest[step:])...)
	}
	return matches
}

// mergeYgotMatch copies the node m names in src into dst, a struct of the same
// type. A leaf is copied alone, without the other leaves of its parent.
func mergeYgotMatch(schema *yang.Entry, dst, src ygot.ValidatedGoStruct, m ygotMatch) error {
	if s, ok := m.data.(ygot.ValidatedGoStruct); ok {
		d, err := getOrCreateYgot(schema, dst, m.elems)
		if err != nil {
			return err
		}
		return ygot.MergeStructInto(d, s)
	}

	n := len(m.elems)
	if n == 0 {
		return fmt.Errorf("leaf match in %T has no path", src)
	}
	srcNodes, err := ytypes.GetNode(schema, src, &gnmi.Path{Elem: m.elems[:n-1]})
	if err != nil {
		return err
	}
	d, err := getOrCreateYgot(schema, dst, m.elems[:n-1])
	if err != nil {
		return err
	}
	return copyYgotField(d, srcNodes[0].Data, m.elems[n-1].Name)
}

func getOrCreateYgot(schema *yang.Entry, root ygot.ValidatedGoStruct, elems []*gnmi.PathElem) (ygot.ValidatedGoStruct, error) {
	node, _, err := ytypes.GetOrCreateNode(schema, root, &gnmi.Path{Elem: elems})
	if err != nil {
		return nil, err
	}
	y, ok := node.(ygot.ValidatedGoStruct)
	if !ok {
		return nil, fmt.Errorf("GetOrCreateNode returned %T; is not a ValidatedGoStruct", node)
	}
	return y, nil
}

// copyYgotField sets the field of dst that holds the leaf or leaf-list
// leafName to the value src holds there. dst and src have the same type.
func copyYgotField(dst ygot.ValidatedGoStruct, src interface{}, leafName string) error {
	dv := reflect.ValueOf(dst).Elem()
	sv := reflect.ValueOf(src)
	if sv.Kind() != reflect.Ptr || sv.Elem().Type() != dv.Type() {
		return fmt.Errorf("cannot copy %s from %T into %T", leafName, src, dst)
	}
	sv = sv.Elem()
	dt := dv.Type()
	for i := 0; i < dt.NumField(); i++ {
		tag := dt.Field(i).Tag.Get("path")
		if k := strings.IndexByte(tag, ':'); k != -1 {
			tag = tag[k+1:]
		}
		if tag == leafName {
			dv.Field(i).Set(sv.Field(i))
			return nil
		}
	}
	return fmt.Errorf("%T has no field for %s", dst, leafName)
}

// isEmptyYgotStruct returns true if none of the fields of ygot struct y are set.
func isEmptyYgotStruct(y ygot.ValidatedGoStruct) bool {
	yv := reflect.ValueOf(y).Elem()
	return isEmptyYgotStructVal(yv)
}

// isEmptyYgotStructVal returns true if the reflect.Value yv represents
// a nil or empty ygot struct. Can panic if it is not a ygot struct value.
func isEmptyYgotStructVal(yv reflect.Value) bool {
	for i := yv.NumField() - 1; i >= 0; i-- {
		fv := yv.Field(i)
		// ygot struct's fields will always be one of ptr, map, slice or an int (for enum & identity).
		// Value.IsZero() should handle all cases.
		if fv.IsValid() && !fv.IsZero() {
			ft := fv.Type()
			if ft.Kind() != reflect.Ptr || ft.Elem().Kind() != reflect.Struct {
				return false
			}
			if !isEmptyYgotStructVal(fv.Elem()) {
				return false
			}
		}
	}
	return true
}
