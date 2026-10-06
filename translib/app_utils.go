////////////////////////////////////////////////////////////////////////////////
//                                                                            //
//  Copyright 2019 Broadcom. The term Broadcom refers to Broadcom Inc. and/or //
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
	"reflect"
	"strings"

	"github.com/Azure/sonic-mgmt-common/cvl"
	"github.com/Azure/sonic-mgmt-common/translib/db"
	"github.com/Azure/sonic-mgmt-common/translib/ocbinds"
	"github.com/Azure/sonic-mgmt-common/translib/tlerr"
	"github.com/Azure/sonic-mgmt-common/translib/transformer"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	log "github.com/golang/glog"
	"github.com/openconfig/gnmi/proto/gnmi"
	"github.com/openconfig/goyang/pkg/yang"
	"github.com/openconfig/ygot/ygot"
	"github.com/openconfig/ygot/ytypes"
)

// stripWildcardListKeys rewrites a request URI so that a list element whose keys
// are all "*" carries no keys at all. The two mean the same thing - every
// instance of that list - but only the keyless form can be served: ygot binds a
// request URI by creating the target node, and it cannot turn "*" into a typed
// key such as a uint32, so a wildcard fails the request outright with
// "unable to convert \"*\" to uint32".
//
// An element with a mix of concrete and wildcard keys is left untouched. Dropping
// only part of a key set would leave a partially keyed list that the binder
// cannot create either, so there is nothing to gain by rewriting it.
//
// Returns the rewritten URI and whether anything changed.
func stripWildcardListKeys(uri string) (string, bool) {
	// Only the structured form is asked for: ygot.PathToString below renders the
	// deprecated Element slice whenever it is set, so a key cleared from Elem
	// would be rebuilt from the string form and the rewrite would be a no-op.
	gPath, err := ygot.StringToPath(uri, ygot.StructuredPath)
	if err != nil {
		return uri, false
	}

	changed := false
	for _, elem := range gPath.Elem {
		if len(elem.Key) == 0 {
			continue
		}
		allWildcard := true
		for _, v := range elem.Key {
			if v != "*" {
				allWildcard = false
				break
			}
		}
		if allWildcard {
			elem.Key = nil
			changed = true
		}
	}

	if !changed {
		return uri, false
	}

	newUri, err := ygot.PathToString(gPath)
	if err != nil {
		log.Warningf("stripWildcardListKeys: cannot rebuild uri from %s: %v", uri, err)
		return uri, false
	}

	return newUri, true
}

// truncateToKeylessList shortens a request URI to the list itself when the
// target sits somewhere below a list that carries no keys. Such a target
// cannot be served as asked: with no key there is no single instance to bind
// the node to, so the request comes back empty today. The list subtree does
// hold the requested node for every instance, so serving that is the closest
// answer to the question and leaves the caller to pick the instances it
// wanted.
//
// A list that is itself the target is left alone, since that already returns
// every instance.
//
// Only one keyless list above the target can be served this way. Serving the
// deeper of two would still leave a keyless list above it, which translib
// cannot bind either, and serving the outer one would leave the caller a
// payload it can only split at one level. Such a request is rejected rather
// than answered wrongly.
//
// Only OpenConfig paths are rewritten. The sonic-yang paths reach the DB
// through a separate field-level extraction that handles a keyless list on
// its own.
//
// Returns the shortened URI, the key leaf names of that list, and whether
// anything changed. The key names let a caller that wants per instance answers
// split the list payload back up by instance.
func truncateToKeylessList(uri string) (string, []string, bool, error) {
	if strings.HasPrefix(uri, "/sonic") {
		return uri, nil, false, nil
	}

	gPath, err := ygot.StringToPath(uri, ygot.StructuredPath)
	if err != nil {
		return uri, nil, false, nil
	}

	listUri := ""
	for i := len(gPath.Elem) - 2; i >= 0; i-- {
		if len(gPath.Elem[i].Key) != 0 {
			continue
		}
		p, perr := ygot.PathToString(&gnmi.Path{Elem: gPath.Elem[:i+1]})
		if perr != nil {
			return uri, nil, false, nil
		}
		if !transformer.IsListNode(p) {
			continue
		}
		if listUri != "" {
			return uri, nil, false, tlerr.NotSupported(
				"wildcard or keyless keys on more than one list are not supported: %s", uri)
		}
		listUri = p
	}

	if listUri == "" {
		return uri, nil, false, nil
	}
	return listUri, transformer.ListKeyNames(listUri), true, nil
}

func getYangPathFromUri(uri string) (string, error) {
	var path *gnmi.Path
	var err error

	path, err = ygot.StringToPath(uri, ygot.StructuredPath, ygot.StringSlicePath)
	if err != nil {
		log.Errorf("Error in uri to path conversion: %v", err)
		return "", err
	}

	yangPath, yperr := ygot.PathToSchemaPath(path)
	if yperr != nil {
		log.Errorf("Error in Gnmi path to Yang path conversion: %v", yperr)
		return "", yperr
	}

	return yangPath, err
}

func getYangPathFromYgotStruct(s ygot.GoStruct, yangPathPrefix string, appModuleName string) string {
	tn := reflect.TypeOf(s).Elem().Name()
	schema, ok := ocbinds.SchemaTree[tn]
	if !ok {
		log.Errorf("could not find schema for type %s", tn)
		return ""
	} else if schema != nil {
		yPath := schema.Path()
		//yPath = strings.Replace(yPath, "/device/acl", "/openconfig-acl:acl", 1)
		yPath = strings.Replace(yPath, yangPathPrefix, appModuleName, 1)
		return yPath
	}
	return ""
}

func generateGetResponse(targetUri string, root *ygot.GoStruct, fmtType TranslibFmtType) (GetResponse, error) {
	var err error
	var resp GetResponse

	if len(targetUri) == 0 {
		return resp, tlerr.InvalidArgs("generateGetResponse failed as target Uri is not valid")
	}
	path, err := ygot.StringToPath(targetUri, ygot.StructuredPath, ygot.StringSlicePath)
	if err != nil {
		return resp, tlerr.InvalidArgs("URI to path conversion failed: %v", err)
	}

	// Get current node (corresponds to ygotTarget) and its parent node
	var pathList []*gnmi.PathElem = path.Elem
	parentPath := &gnmi.Path{}
	for i := 0; i < len(pathList); i++ {
		if log.V(3) {
			log.Infof("pathList[%d]: %s\n", i, pathList[i])
		}
		pathSlice := strings.Split(pathList[i].Name, ":")
		pathList[i].Name = pathSlice[len(pathSlice)-1]
		if i < (len(pathList) - 1) {
			parentPath.Elem = append(parentPath.Elem, pathList[i])
		}
	}
	parentNodeList, err := ytypes.GetNode(ygSchema.RootSchema(), *root, parentPath)
	if err != nil {
		return resp, err
	}
	if len(parentNodeList) == 0 {
		return resp, tlerr.InvalidArgs("Invalid URI: %s", targetUri)
	}
	parentNode := parentNodeList[0].Data

	currentNodeList, ygerr := ytypes.GetNode(ygSchema.RootSchema(), *root, path, &(ytypes.GetPartialKeyMatch{}))
	if ygerr != nil {
		log.Errorf("Error from ytypes.GetNode: %v", ygerr)
		if status.Convert(ygerr).Code() == codes.NotFound {
			return resp, tlerr.NotFound("Resource not found")
		} else {
			return resp, ygerr
		}
	}
	if len(currentNodeList) == 0 {
		return resp, tlerr.NotFound("Resource not found")
	}
	//currentNode := currentNodeList[0].Data
	currentNodeYangName := currentNodeList[0].Schema.Name

	// Create empty clone of parent node
	parentNodeClone := reflect.New(reflect.TypeOf(parentNode).Elem())
	var parentCloneObj ygot.ValidatedGoStruct
	var ok bool
	if parentCloneObj, ok = (parentNodeClone.Interface()).(ygot.ValidatedGoStruct); ok {
		ygot.BuildEmptyTree(parentCloneObj)
		pcType := reflect.TypeOf(parentCloneObj).Elem()
		pcValue := reflect.ValueOf(parentCloneObj).Elem()

		var currentNodeOCFieldName string
		for i := 0; i < pcValue.NumField(); i++ {
			fld := pcValue.Field(i)
			fldType := pcType.Field(i)
			if fldType.Tag.Get("path") == currentNodeYangName {
				currentNodeOCFieldName = fldType.Name
				// Take value from original parent and set in parent clone
				valueFromParent := reflect.ValueOf(parentNode).Elem().FieldByName(currentNodeOCFieldName)
				fld.Set(valueFromParent)
				break
			}
		}
		if log.V(3) {
			log.Infof("Target yang name: %s  OC Field name: %s\n", currentNodeYangName, currentNodeOCFieldName)
		}
	}
	if fmtType == TRANSLIB_FMT_YGOT {
		resp.ValueTree = parentCloneObj
		return resp, err
	}

	resp.Payload, err = dumpIetfJson(parentCloneObj)

	return resp, err
}

func getTargetNodeYangSchema(targetUri string, deviceObj *ocbinds.Device) (*yang.Entry, error) {
	if len(targetUri) == 0 {
		return nil, tlerr.InvalidArgs("GetResponse failed as target Uri is not valid")
	}
	path, err := ygot.StringToPath(targetUri, ygot.StructuredPath, ygot.StringSlicePath)
	if err != nil {
		return nil, tlerr.InvalidArgs("URI to path conversion failed: %v", err)
	}
	// Get current node (corresponds to ygotTarget)
	var pathList []*gnmi.PathElem = path.Elem
	for i := 0; i < len(pathList); i++ {
		if log.V(3) {
			log.Infof("pathList[%d]: %s\n", i, pathList[i])
		}
		pathSlice := strings.Split(pathList[i].Name, ":")
		pathList[i].Name = pathSlice[len(pathSlice)-1]
	}
	targetNodeList, err := ytypes.GetNode(ygSchema.RootSchema(), deviceObj, path, &(ytypes.GetPartialKeyMatch{}))
	if err != nil {
		return nil, tlerr.InvalidArgs("Getting node information failed: %v", err)
	}
	if len(targetNodeList) == 0 {
		return nil, tlerr.NotFound("Resource not found")
	}
	targetNodeSchema := targetNodeList[0].Schema
	//targetNode := targetNodeList[0].Data
	if log.V(3) {
		log.Infof("Target node yang name: %s\n", targetNodeSchema.Name)
	}
	return targetNodeSchema, nil
}

func dumpIetfJson(s ygot.ValidatedGoStruct) ([]byte, error) {
	cfg := ocbinds.EmitJSONOptions{
		SortList: true,
	}
	return ocbinds.EmitJSON(s, &cfg)
}

func contains(sl []string, str string) bool {
	for _, v := range sl {
		if v == str {
			return true
		}
	}
	return false
}

func removeElement(sl []string, str string) []string {
	for i := 0; i < len(sl); i++ {
		if sl[i] == str {
			sl = append(sl[:i], sl[i+1:]...)
			i--
			sl = sl[:len(sl)]
			break
		}
	}
	return sl
}

// isNotFoundError return true if the error is a 'not found' error
func isNotFoundError(err error) bool {
	switch err.(type) {
	case tlerr.TranslibRedisClientEntryNotExist, tlerr.NotFoundError:
		return true
	default:
		return false
	}
}

// isBulkNotFoundError return true if the error is a 'not found' error
func isBulkNotFoundError(err error) bool {
	switch e := err.(type) {
	case tlerr.TranslibRedisClientEntryNotExist, tlerr.NotFoundError:
		return true
	case tlerr.TranslibCVLFailure:
		if cvl.CVLRetCode(e.Code) == cvl.CVL_SEMANTIC_KEY_NOT_EXIST {
			return true
		} else {
			return false
		}
	default:
		return false
	}
}

// asKey cretaes a db.Key from given key components
func asKey(parts ...string) db.Key {
	return db.Key{Comp: parts}
}

func createEmptyDbValue(fieldName string) db.Value {
	return db.Value{Field: map[string]string{fieldName: ""}}
}

// Content query parameter (gNMI Get data type) handling for the apps that fill
// the ygot tree by themselves, i.e. do not go through the transformer. The node
// classification below mirrors the transformer implementation in
// translib/transformer/xlate_prune.go, so that a Get of a given data type
// behaves the same way for transformer and non-transformer paths.

const ocContentMismatchErr = "Bad Request - requested content type doesn't match content type of terminal node uri."

type ocContentType uint8

const (
	ocContentAll ocContentType = iota
	ocContentConfig
	ocContentNonConfig
	ocContentOperational
)

func toOCContentType(content string) ocContentType {
	switch strings.ToLower(strings.TrimSpace(content)) {
	case QueryParametersContentConfig:
		return ocContentConfig
	case QueryParametersContentNonConfig, QueryParametersContentState:
		return ocContentNonConfig
	case QueryParametersContentOperational:
		return ocContentOperational
	default:
		return ocContentAll
	}
}

// filterGetContent removes the nodes not matching the requested content type
// from the ygot tree filled by an app. Returns an error if the target node of
// the request itself does not match the requested content type.
func filterGetContent(pathTemplate, content string, ygotRoot *ygot.GoStruct) error {
	ct := toOCContentType(content)
	if ct == ocContentAll {
		return nil
	}
	if ocContentMismatch(pathTemplate, ct) {
		return tlerr.InvalidArgsError{Format: ocContentMismatchErr}
	}
	if ygotRoot != nil && *ygotRoot != nil {
		pruneOCContent(*ygotRoot, ct)
	}
	return nil
}

// ocContentMismatch checks if the target node of a request path is filtered
// out by the content type - e.g. a config node for a STATE request.
func ocContentMismatch(pathTemplate string, ct ocContentType) bool {
	entry := ocEntryForPathTemplate(pathTemplate)
	return entry != nil && !ocEntryMatchesContent(entry, ct)
}

// ocEntryForPathTemplate resolves the yang entry for a PathInfo template, like
// "/openconfig-acl:acl/acl-sets/acl-set{}{}/config/description".
func ocEntryForPathTemplate(pathTemplate string) *yang.Entry {
	entry := ocbinds.SchemaTree["Device"]
	for _, seg := range strings.Split(pathTemplate, "/") {
		name := ocYangName(seg)
		if len(name) == 0 {
			continue
		}
		if entry == nil {
			return nil
		}
		entry = entry.Dir[name]
	}
	return entry
}

// ocYangName strips the module prefix and the key placeholders of a path
// template segment - e.g. "openconfig-acl:acl-set{}{}" becomes "acl-set".
func ocYangName(seg string) string {
	if i := strings.LastIndex(seg, ":"); i >= 0 {
		seg = seg[i+1:]
	}
	for strings.HasSuffix(seg, "{}") {
		seg = strings.TrimSuffix(seg, "{}")
	}
	return seg
}

func ocEntryMatchesContent(entry *yang.Entry, ct ocContentType) bool {
	switch ct {
	case ocContentConfig:
		return !entry.ReadOnly()
	case ocContentNonConfig:
		return isOCStateEntry(entry)
	case ocContentOperational:
		return isOCOperationalEntry(entry)
	}
	return true
}

func isOCStateEntry(entry *yang.Entry) bool {
	if entry.ReadOnly() {
		return true
	}
	// Writable terminal nodes and the "config" containers are config data.
	if entry.IsLeaf() || entry.IsLeafList() {
		return false
	}
	if entry.IsContainer() && entry.Name == "config" {
		return false
	}
	// Writable container/list can still hold state data below it.
	return true
}

func isOCOperationalEntry(entry *yang.Entry) bool {
	if !isOCStateEntry(entry) {
		return false
	}
	if !entry.IsLeaf() && !entry.IsLeafList() {
		return true
	}
	// A state leaf mirroring a config leaf is not operational data.
	cfg := entry.Find("../../config/" + entry.Name)
	if cfg == nil || cfg.ReadOnly() {
		return true
	}
	return !cfg.IsLeaf() && !cfg.IsLeafList()
}

func pruneOCContent(root ygot.GoStruct, ct ocContentType) {
	v := reflect.ValueOf(root)
	if v.Kind() != reflect.Ptr || v.IsNil() {
		return
	}
	pruneOCStruct(v.Elem(), ocbinds.SchemaTree[v.Type().Elem().Name()], ct)
}

// pruneOCStruct removes the fields of a ygot struct that do not match the
// content type. Returns true if the struct still holds any data other than the
// keys of the list it belongs to.
func pruneOCStruct(v reflect.Value, entry *yang.Entry, ct ocContentType) bool {
	if entry == nil {
		return !v.IsZero() // no schema info; keep the subtree as is
	}
	keys := ocListKeys(entry)
	hasData := false
	t := v.Type()
	for i := 0; i < v.NumField(); i++ {
		fv := v.Field(i)
		if !fv.CanSet() || fv.IsZero() {
			continue
		}
		name := ocYangName(t.Field(i).Tag.Get("path"))
		child := entry.Dir[name]
		if child == nil {
			hasData = true // not a yang node; e.g. annotation field
			continue
		}
		// List keys are always retained, but do not make the entry non-empty
		// by themselves.
		if keys[name] {
			continue
		}
		if !ocEntryMatchesContent(child, ct) {
			fv.Set(reflect.Zero(fv.Type()))
			continue
		}
		if pruneOCSubtree(fv, child, ct) {
			hasData = true
		}
	}
	return hasData
}

// pruneOCSubtree prunes a non-zero struct field and clears it if nothing is
// left in it after the pruning. Returns true if the field still has data.
func pruneOCSubtree(fv reflect.Value, entry *yang.Entry, ct ocContentType) bool {
	if entry.IsLeaf() || entry.IsLeafList() {
		return true
	}
	switch fv.Kind() {
	case reflect.Ptr: // container or list instance
		if fv.IsNil() {
			return false
		}
		if sv := fv.Elem(); sv.Kind() == reflect.Struct {
			if pruneOCStruct(sv, entry, ct) {
				return true
			}
			if fv.CanSet() { // not settable when it is a map value
				fv.Set(reflect.Zero(fv.Type()))
			}
			return false
		}
	case reflect.Map: // keyed list
		for _, k := range fv.MapKeys() {
			if !pruneOCSubtree(fv.MapIndex(k), entry, ct) {
				fv.SetMapIndex(k, reflect.Value{})
			}
		}
		if fv.Len() == 0 {
			fv.Set(reflect.Zero(fv.Type()))
			return false
		}
	case reflect.Slice: // unkeyed list
		kept := reflect.MakeSlice(fv.Type(), 0, fv.Len())
		for i := 0; i < fv.Len(); i++ {
			if pruneOCSubtree(fv.Index(i), entry, ct) {
				kept = reflect.Append(kept, fv.Index(i))
			}
		}
		if kept.Len() == 0 {
			fv.Set(reflect.Zero(fv.Type()))
			return false
		}
		fv.Set(kept)
	}
	return true
}

func ocListKeys(entry *yang.Entry) map[string]bool {
	if len(entry.Key) == 0 {
		return nil
	}
	keys := make(map[string]bool)
	for _, k := range strings.Fields(entry.Key) {
		keys[k] = true
	}
	return keys
}
