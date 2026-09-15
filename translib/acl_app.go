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
	"bytes"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	"github.com/Azure/sonic-mgmt-common/translib/ocbinds"
	"github.com/Azure/sonic-mgmt-common/translib/path"
	"github.com/Azure/sonic-mgmt-common/translib/tlerr"

	log "github.com/golang/glog"
	"github.com/openconfig/gnmi/proto/gnmi"
	"github.com/openconfig/ygot/util"
	"github.com/openconfig/ygot/ygot"
)

const (
	TABLE_SEPARATOR          = "|"
	KEY_SEPARATOR            = "|"
	ACL_TABLE                = "ACL_TABLE"
	RULE_TABLE               = "ACL_RULE"
	ACL_TYPE                 = "type"
	ACL_DESCRIPTION          = "policy_desc"
	ACL_SERVICES             = "services"
	SONIC_ACL_TYPE_L2        = "L2"
	SONIC_ACL_TYPE_IPV4      = "L3"
	SONIC_ACL_TYPE_IPV6      = "L3V6"
	SONIC_ACL_TYPE_CTRLPLANE = "CTRLPLANE"
	OPENCONFIG_ACL_TYPE_IPV4 = "ACL_IPV4"
	OPENCONFIG_ACL_TYPE_IPV6 = "ACL_IPV6"
	OPENCONFIG_ACL_TYPE_L2   = "ACL_L2"
	OC_ACL_APP_MODULE_NAME   = "/openconfig-acl:acl"
	OC_ACL_YANG_PATH_PREFIX  = "/device/acl"

	// Rule counters are published by the flex counter service in COUNTERS_DB.
	// ACL_COUNTER_RULE_MAP maps "<acl table>:<rule>" to the counter object id,
	// whose packet and byte counts live in the COUNTERS table.
	ACL_COUNTER_RULE_MAP     = "ACL_COUNTER_RULE_MAP"
	COUNTERS_TABLE           = "COUNTERS"
	ACL_COUNTER_ATTR_PACKETS = "SAI_ACL_COUNTER_ATTR_PACKETS"
	ACL_COUNTER_ATTR_BYTES   = "SAI_ACL_COUNTER_ATTR_BYTES"

	MIN_PRIORITY = 1
	MAX_PRIORITY = 65536
)

var IP_PROTOCOL_MAP = map[ocbinds.E_OpenconfigPacketMatchTypes_IP_PROTOCOL]uint8{
	ocbinds.OpenconfigPacketMatchTypes_IP_PROTOCOL_IP_ICMP: 1,
	ocbinds.OpenconfigPacketMatchTypes_IP_PROTOCOL_IP_IGMP: 2,
	ocbinds.OpenconfigPacketMatchTypes_IP_PROTOCOL_IP_TCP:  6,
	ocbinds.OpenconfigPacketMatchTypes_IP_PROTOCOL_IP_UDP:  17,
	ocbinds.OpenconfigPacketMatchTypes_IP_PROTOCOL_IP_RSVP: 46,
	ocbinds.OpenconfigPacketMatchTypes_IP_PROTOCOL_IP_GRE:  47,
	ocbinds.OpenconfigPacketMatchTypes_IP_PROTOCOL_IP_AUTH: 51,
	ocbinds.OpenconfigPacketMatchTypes_IP_PROTOCOL_IP_PIM:  103,
	ocbinds.OpenconfigPacketMatchTypes_IP_PROTOCOL_IP_L2TP: 115,
}

// ICMPV6_TYPE_MAP maps the standard openconfig-icmpv6-types TYPE identities to
// their numeric ICMPv6 message type (SONiC ACL_RULE ICMPV6_TYPE). ICMPv6 code
// (subcode) mapping is handled separately as a follow-up.
var ICMPV6_TYPE_MAP = map[ocbinds.E_OpenconfigIcmpv6Types_TYPE]uint8{
	ocbinds.OpenconfigIcmpv6Types_TYPE_DESTINATION_UNREACHABLE:              1,
	ocbinds.OpenconfigIcmpv6Types_TYPE_PACKET_TOO_BIG:                       2,
	ocbinds.OpenconfigIcmpv6Types_TYPE_TIME_EXCEEDED:                        3,
	ocbinds.OpenconfigIcmpv6Types_TYPE_PARAMETER_PROBLEM:                    4,
	ocbinds.OpenconfigIcmpv6Types_TYPE_ECHO_REQUEST:                         128,
	ocbinds.OpenconfigIcmpv6Types_TYPE_ECHO_REPLY:                           129,
	ocbinds.OpenconfigIcmpv6Types_TYPE_MULTICAST_LISTENER_QUERY:             130,
	ocbinds.OpenconfigIcmpv6Types_TYPE_MULTICAST_LISTENER_REPORT:            131,
	ocbinds.OpenconfigIcmpv6Types_TYPE_MULTICAST_LISTENER_DONE:              132,
	ocbinds.OpenconfigIcmpv6Types_TYPE_ROUTER_SOLICITATION:                  133,
	ocbinds.OpenconfigIcmpv6Types_TYPE_ROUTER_ADVERTISEMENT:                 134,
	ocbinds.OpenconfigIcmpv6Types_TYPE_NEIGHBOR_SOLICITATION:                135,
	ocbinds.OpenconfigIcmpv6Types_TYPE_NEIGHBOR_ADVERTISEMENT:               136,
	ocbinds.OpenconfigIcmpv6Types_TYPE_REDIRECT:                             137,
	ocbinds.OpenconfigIcmpv6Types_TYPE_RENUNBERING:                          138,
	ocbinds.OpenconfigIcmpv6Types_TYPE_NODE_INFORMATION_QUERY:               139,
	ocbinds.OpenconfigIcmpv6Types_TYPE_NODE_INFORMATION_RESPONSE:            140,
	ocbinds.OpenconfigIcmpv6Types_TYPE_INVERSE_NEIGHBOR_SOLICITATION:        141,
	ocbinds.OpenconfigIcmpv6Types_TYPE_INVERSE_NEIGHBOR_ADVERTISEMENT:       142,
	ocbinds.OpenconfigIcmpv6Types_TYPE_VERSION2_MULTICAST_LISTENER:          143,
	ocbinds.OpenconfigIcmpv6Types_TYPE_HOME_AGENT_ADDRESS_DISCOVERY_REQUEST: 144,
	ocbinds.OpenconfigIcmpv6Types_TYPE_HOME_AGENT_ADDRESS_DISCOVERY_REPLY:   145,
	ocbinds.OpenconfigIcmpv6Types_TYPE_MOBILE_PREFIX_SOLICITATION:           146,
	ocbinds.OpenconfigIcmpv6Types_TYPE_MOBILE_PREFIX_ADVERTISEMENT:          147,
	ocbinds.OpenconfigIcmpv6Types_TYPE_CERTIFICATION_PATH_SOLICITATION:      148,
	ocbinds.OpenconfigIcmpv6Types_TYPE_CERTIFICATION_PATH_ADVERTISEMENT:     149,
	ocbinds.OpenconfigIcmpv6Types_TYPE_MULTICAST_ROUTER_ADVERTISEMENT:       151,
	ocbinds.OpenconfigIcmpv6Types_TYPE_MULTICAST_ROUTER_SOLICITATION:        152,
	ocbinds.OpenconfigIcmpv6Types_TYPE_MULTICAST_ROUTER_TERMINATION:         153,
	ocbinds.OpenconfigIcmpv6Types_TYPE_FMIPV6:                               154,
	ocbinds.OpenconfigIcmpv6Types_TYPE_RPL_CONTROL:                          155,
	ocbinds.OpenconfigIcmpv6Types_TYPE_ILNPV6_LOCATOR_UPDATE:                156,
	ocbinds.OpenconfigIcmpv6Types_TYPE_DUPLICATE_ADDRESS_REQUEST:            157,
	ocbinds.OpenconfigIcmpv6Types_TYPE_DUPLICATE_ADDRESS_CONFIRMATION:       158,
	ocbinds.OpenconfigIcmpv6Types_TYPE_MPL_CONTROL:                          159,
	ocbinds.OpenconfigIcmpv6Types_TYPE_EXT_ECHO_REQUEST:                     160,
	ocbinds.OpenconfigIcmpv6Types_TYPE_EXT_ECHO_REPLY:                       161,
}

var ETHERTYPE_MAP = map[ocbinds.E_OpenconfigPacketMatchTypes_ETHERTYPE]uint32{
	ocbinds.OpenconfigPacketMatchTypes_ETHERTYPE_ETHERTYPE_LLDP: 0x88CC,
	ocbinds.OpenconfigPacketMatchTypes_ETHERTYPE_ETHERTYPE_VLAN: 0x8100,
	ocbinds.OpenconfigPacketMatchTypes_ETHERTYPE_ETHERTYPE_ROCE: 0x8915,
	ocbinds.OpenconfigPacketMatchTypes_ETHERTYPE_ETHERTYPE_ARP:  0x0806,
	ocbinds.OpenconfigPacketMatchTypes_ETHERTYPE_ETHERTYPE_IPV4: 0x0800,
	ocbinds.OpenconfigPacketMatchTypes_ETHERTYPE_ETHERTYPE_IPV6: 0x86DD,
	ocbinds.OpenconfigPacketMatchTypes_ETHERTYPE_ETHERTYPE_MPLS: 0x8847,
}

type AclApp struct {
	pathInfo   *PathInfo
	ygotRoot   *ygot.GoStruct
	ygotTarget *interface{}
	appOptions

	aclTs  *db.TableSpec
	ruleTs *db.TableSpec

	aclTableMap  map[string]db.Value
	ruleTableMap map[string]map[string]db.Value

	// Counters are read outside the ConfigDB handle passed to processCommon.
	countersDb  *db.DB
	counterOids db.Value
	oidsRead    bool
}

func init() {

	err := register("/openconfig-acl:acl",
		&appInfo{appType: reflect.TypeOf(AclApp{}),
			ygotRootType:  reflect.TypeOf(ocbinds.OpenconfigAcl_Acl{}),
			isNative:      false,
			tablesToWatch: []*db.TableSpec{&db.TableSpec{Name: ACL_TABLE}, &db.TableSpec{Name: RULE_TABLE}}})

	if err != nil {
		log.Fatal("Register ACL app module with App Interface failed with error=", err)
	}

	err = addModel(&ModelData{Name: "openconfig-acl",
		Org: "OpenConfig working group",
		Ver: "1.0.2"})
	if err != nil {
		log.Fatal("Adding model data to appinterface failed with error=", err)
	}
}

func (app *AclApp) initialize(data appData) {
	log.Info("initialize:acl:path =", data.path)
	pathInfo := NewPathInfo(data.path)
	*app = AclApp{pathInfo: pathInfo, ygotRoot: data.ygotRoot, ygotTarget: data.ygotTarget, appOptions: data.appOptions}

	app.aclTs = &db.TableSpec{Name: ACL_TABLE}
	app.ruleTs = &db.TableSpec{Name: RULE_TABLE}

	app.aclTableMap = make(map[string]db.Value)
	app.ruleTableMap = make(map[string]map[string]db.Value)
}

func (app *AclApp) getAppRootObject() *ocbinds.OpenconfigAcl_Acl {
	deviceObj := (*app.ygotRoot).(*ocbinds.Device)
	return deviceObj.Acl
}

func (app *AclApp) translateCreate(d *db.DB) ([]db.WatchKeys, error) {
	return app.translateCRUCommon(d, CREATE)
}

func (app *AclApp) translateUpdate(d *db.DB) ([]db.WatchKeys, error) {
	return app.translateCRUCommon(d, UPDATE)
}

func (app *AclApp) translateReplace(d *db.DB) ([]db.WatchKeys, error) {
	return app.translateCRUCommon(d, REPLACE)
}

func (app *AclApp) translateDelete(d *db.DB) ([]db.WatchKeys, error) {
	return nil, nil
}

func (app *AclApp) translateGet(dbs [db.MaxDB]*db.DB) error {
	return nil
}

func (app *AclApp) translateAction(dbs [db.MaxDB]*db.DB) error {
	return tlerr.NotSupported("unsupported")
}

func (app *AclApp) processCreate(d *db.DB) (SetResponse, error) {
	var err error
	var resp SetResponse

	if err = app.processCommon(d, CREATE); err != nil {
		log.Error(err)
		resp = SetResponse{ErrSrc: AppErr}
	}
	return resp, err
}

func (app *AclApp) processUpdate(d *db.DB) (SetResponse, error) {
	var err error
	var resp SetResponse

	if err = app.processCommon(d, UPDATE); err != nil {
		log.Error(err)
		resp = SetResponse{ErrSrc: AppErr}
	}
	return resp, err
}

func (app *AclApp) processReplace(d *db.DB) (SetResponse, error) {
	var err error
	var resp SetResponse

	if err = app.processCommon(d, REPLACE); err != nil {
		log.Error(err)
		resp = SetResponse{ErrSrc: AppErr}
	}
	return resp, err
}

func (app *AclApp) processDelete(d *db.DB) (SetResponse, error) {
	var err error
	var resp SetResponse

	if err = app.processCommon(d, DELETE); err != nil {
		log.Error(err)
		resp = SetResponse{ErrSrc: AppErr}
	}
	return resp, err
}

func (app *AclApp) processGet(dbs [db.MaxDB]*db.DB, fmtType TranslibFmtType) (GetResponse, error) {
	var err error
	var payload []byte

	configDb := dbs[db.ConfigDB]
	app.countersDb = dbs[db.CountersDB]
	err = app.processCommon(configDb, GET)
	if err != nil {
		return GetResponse{Payload: payload, ErrSrc: AppErr}, err
	}

	if err = filterGetContent(app.pathInfo.Template, app.content, app.ygotRoot); err != nil {
		return GetResponse{Payload: payload, ErrSrc: AppErr}, err
	}

	return generateGetResponse(app.pathInfo.Path, app.ygotRoot, fmtType)
}

func (app *AclApp) processAction(dbs [db.MaxDB]*db.DB) (ActionResponse, error) {
	return ActionResponse{}, tlerr.New("not implemented")
}

func (app *AclApp) translateCRUCommon(d *db.DB, opcode int) ([]db.WatchKeys, error) {
	var err error
	var keys []db.WatchKeys
	log.Info("translateCRUCommon:acl:path =", app.pathInfo.Template)

	app.convertOCAclsToInternal()
	app.convertOCAclRulesToInternal(d)
	app.convertOCAclBindingsToInternal()

	return keys, err
}

func (app *AclApp) processCommon(d *db.DB, opcode int) error {
	var err error
	var topmostPath bool = false
	acl := app.getAppRootObject()

	log.Infof("processCommon--Path Received: %s", app.pathInfo.Template)
	targetType := reflect.TypeOf(*app.ygotTarget)
	if !util.IsValueScalar(reflect.ValueOf(*app.ygotTarget)) && util.IsValuePtr(reflect.ValueOf(*app.ygotTarget)) {
		log.Infof("processCommon: Target object is a <%s> of Type: %s", targetType.Kind().String(), targetType.Elem().Name())
		if targetType.Elem().Name() == "OpenconfigAcl_Acl" {
			topmostPath = true
		}
	}

	targetUriPath, _ := getYangPathFromUri(app.pathInfo.Path)
	if isSubtreeRequest(app.pathInfo.Template, "/openconfig-acl:acl/acl-sets") {
		if isSubtreeRequest(app.pathInfo.Template, "/openconfig-acl:acl/acl-sets/acl-set{}{}") {
			for aclSetKey, _ := range acl.AclSets.AclSet {
				aclSet := acl.AclSets.AclSet[aclSetKey]
				aclKey := getAclKeyStrFromOCKey(aclSetKey.Name, aclSetKey.Type)

				if isSubtreeRequest(app.pathInfo.Template, "/openconfig-acl:acl/acl-sets/acl-set{}{}/acl-entries/acl-entry{}") {
					// Subtree of one Rule
					for seqId, _ := range aclSet.AclEntries.AclEntry {
						ruleKey := "RULE_" + strconv.Itoa(int(seqId))
						entrySet := aclSet.AclEntries.AclEntry[seqId]

						ruleNodeYangPath := getYangPathFromYgotStruct(entrySet, OC_ACL_YANG_PATH_PREFIX, OC_ACL_APP_MODULE_NAME)
						isRuleNodeSubtree := len(targetUriPath) > len(ruleNodeYangPath)
						switch opcode {
						case CREATE:
							if isRuleNodeSubtree {
								err = app.setAclRuleDataInConfigDb(d, app.ruleTableMap, false)
							} else if *app.ygotTarget == entrySet {
								err = app.setAclRuleDataInConfigDb(d, app.ruleTableMap, true)
							} else {
								log.Errorf("processCommon: Given CREATE path %s not handled", targetUriPath)
							}
						case REPLACE:
							err = d.SetEntry(app.ruleTs, db.Key{Comp: []string{aclKey, ruleKey}}, app.ruleTableMap[aclKey][ruleKey])
						case UPDATE:
							err = d.ModEntry(app.ruleTs, db.Key{Comp: []string{aclKey, ruleKey}}, app.ruleTableMap[aclKey][ruleKey])
						case DELETE:
							if *app.ygotTarget == entrySet {
								err = d.DeleteEntry(app.ruleTs, db.Key{Comp: []string{aclKey, ruleKey}})
							} else if isRuleNodeSubtree {
								err = app.handleRuleFieldsDeletion(d, aclKey, ruleKey)
								if err != nil {
									return err
								}
								//err = d.SetEntry(app.ruleTs, db.Key{Comp: []string{aclKey, ruleKey}}, app.ruleTableMap[aclKey][ruleKey])
							} else {
								log.Errorf("processCommon: Given DELETE path %s not handled", targetUriPath)
							}
						case GET:
							err = app.convertDBAclRulesToInternal(d, aclKey, int64(seqId), db.Key{})
							ygot.BuildEmptyTree(entrySet)
							app.convertInternalToOCAclRule(aclKey, aclSetKey.Type, int64(seqId), nil, entrySet)
						}
					}
				} else {
					isAclEntriesSubtree := isSubtreeRequest(app.pathInfo.Template, "/openconfig-acl:acl/acl-sets/acl-set{}{}/acl-entries")
					switch opcode {
					case CREATE:
						if *app.ygotTarget == aclSet {
							err = app.setAclDataInConfigDb(d, app.aclTableMap, true)
							if err != nil {
								return err
							}
							err = app.setAclRuleDataInConfigDb(d, app.ruleTableMap, true)
						} else if isAclEntriesSubtree {
							err = app.setAclRuleDataInConfigDb(d, app.ruleTableMap, true)
						} else {
							err = d.SetEntry(app.aclTs, db.Key{Comp: []string{aclKey}}, app.aclTableMap[aclKey])
						}
					case REPLACE:
						// ACL_RULE:aclname is a leafref to ACL_TABLE:aclname and CVL
						// validates every write as it is added to the transaction, so
						// the table row has to be written before its rules. An empty
						// table map carries no table level fields to write; skipping it
						// avoids ModEntry falling back to deleting the ACL_TABLE row
						// out from under the rules being created here.
						if !isAclEntriesSubtree && len(app.aclTableMap[aclKey].Field) > 0 {
							existingAcl, _ := d.GetEntry(app.aclTs, asKey(aclKey))
							if existingAcl.IsPopulated() {
								// Merge instead of overwrite: ports and stage belong
								// to the interface binding, not to this payload.
								err = d.ModEntry(app.aclTs, asKey(aclKey), app.aclTableMap[aclKey])
							} else {
								// CVL rejects an update against a key that does not
								// exist yet, so a brand new ACL has to be created.
								err = d.CreateEntry(app.aclTs, asKey(aclKey), app.aclTableMap[aclKey])
							}
							if err != nil {
								return err
							}
						}
						if *app.ygotTarget == aclSet || isAclEntriesSubtree {
							err = d.DeleteKeys(app.ruleTs, db.Key{Comp: []string{aclKey + TABLE_SEPARATOR + "RULE_*"}})
							if err != nil {
								return err
							}
							err = app.setAclRuleDataInConfigDb(d, app.ruleTableMap, true)
							if err != nil {
								return err
							}
						}
					case UPDATE:
						if !isAclEntriesSubtree {
							err = app.setAclDataInConfigDb(d, app.aclTableMap, false)
							//err = d.ModEntry(app.aclTs, db.Key{Comp: []string{aclKey}}, app.aclTableMap[aclKey])
							if err != nil {
								return err
							}
						}
						if *app.ygotTarget == aclSet || isAclEntriesSubtree {
							err = app.setAclRuleDataInConfigDb(d, app.ruleTableMap, false)
						}
					case DELETE:
						if *app.ygotTarget == aclSet {
							err = d.DeleteKeys(app.ruleTs, db.Key{Comp: []string{aclKey + TABLE_SEPARATOR + "*"}})
							if err != nil {
								return err
							}
							err = d.DeleteEntry(app.aclTs, db.Key{Comp: []string{aclKey}})
						} else if isAclEntriesSubtree {
							err = d.DeleteKeys(app.ruleTs, db.Key{Comp: []string{aclKey + TABLE_SEPARATOR + "RULE_*"}})
						} else {
							nodeInfo, err := getTargetNodeYangSchema(app.pathInfo.Path, (*app.ygotRoot).(*ocbinds.Device))
							if err != nil {
								return err
							}
							if nodeInfo != nil && nodeInfo.IsLeaf() && nodeInfo.Name == "description" {
								err = d.DeleteEntryFields(app.aclTs, asKey(aclKey), createEmptyDbValue(ACL_DESCRIPTION))
							}
							//err = d.SetEntry(app.aclTs, db.Key{Comp: []string{aclKey}}, app.aclTableMap[aclKey])
						}
					case GET:
						err = app.convertDBAclToInternal(d, db.Key{Comp: []string{aclKey}})
						if err != nil {
							return err
						}
						ygot.BuildEmptyTree(aclSet)
						app.convertInternalToOCAcl(aclKey, acl.AclSets, aclSet)
					}
				}
			}
		} else {
			// All Acls and their rules
			err = app.processCommonToplevelPath(d, acl, opcode, false)
		}
	} else if isSubtreeRequest(app.pathInfo.Template, "/openconfig-acl:acl/interfaces") {
		switch opcode {
		case CREATE, REPLACE, UPDATE:
			err = app.setAclBindDataInConfigDb(d, app.aclTableMap, opcode)
		case DELETE:
			err = app.handleBindingsDeletion(d)
		case GET:
			if isSubtreeRequest(app.pathInfo.Template, "/openconfig-acl:acl/interfaces/interface{}") {
				for intfId := range acl.Interfaces.Interface {
					intfData := acl.Interfaces.Interface[intfId]
					ygot.BuildEmptyTree(intfData)
					if isSubtreeRequest(targetUriPath, "/openconfig-acl:acl/interfaces/interface/ingress-acl-sets") {
						err = app.getAclBindingInfoForInterfaceData(d, intfData, intfId, "INGRESS")
					} else if isSubtreeRequest(targetUriPath, "/openconfig-acl:acl/interfaces/interface/egress-acl-sets") {
						err = app.getAclBindingInfoForInterfaceData(d, intfData, intfId, "EGRESS")
					} else {
						// Direction unknown. Check ACL Table for binding information.
						err = app.getAclBindingInfoForInterfaceData(d, intfData, intfId, "INGRESS")
						if err != nil {
							return err
						}
						err = app.getAclBindingInfoForInterfaceData(d, intfData, intfId, "EGRESS")
					}
				}
			} else {
				err = app.getAllBindingsInfo(d)
			}
		}
	} else {
		err = app.processCommonToplevelPath(d, acl, opcode, true)
	}

	if !topmostPath && !isSubtreeRequest(targetUriPath, "/openconfig-acl:acl/acl-sets") && !isSubtreeRequest(targetUriPath, "/openconfig-acl:acl/interfaces") {
		err = tlerr.NotSupported("URL %s is not supported", app.pathInfo.Template)
	}

	return err
}

func (app *AclApp) processCommonToplevelPath(d *db.DB, acl *ocbinds.OpenconfigAcl_Acl, opcode int, isTopmostPath bool) error {
	var err error
	switch opcode {
	case CREATE:
		err = app.setAclDataInConfigDb(d, app.aclTableMap, true)
		if err != nil {
			return err
		}
		err = app.setAclRuleDataInConfigDb(d, app.ruleTableMap, true)
	case REPLACE:
		err = d.DeleteTable(app.aclTs)
		if err != nil {
			return err
		}
		err = d.DeleteTable(app.ruleTs)
		if err != nil {
			return err
		}
		err = app.setAclDataInConfigDb(d, app.aclTableMap, true)
		if err != nil {
			return err
		}
		err = app.setAclRuleDataInConfigDb(d, app.ruleTableMap, true)
	case UPDATE:
		err = app.setAclDataInConfigDb(d, app.aclTableMap, false)
		if err != nil {
			return err
		}
		err = app.setAclRuleDataInConfigDb(d, app.ruleTableMap, false)
	case DELETE:
		err = d.DeleteTable(app.ruleTs)
		if err != nil {
			return err
		}
		err = d.DeleteTable(app.aclTs)
	case GET:
		ygot.BuildEmptyTree(acl)
		err = app.convertDBAclToInternal(d, db.Key{})
		if err != nil {
			return err
		}
		app.convertInternalToOCAcl("", acl.AclSets, nil)
		if isTopmostPath {
			err = app.getAllBindingsInfo(d)
		}
	}
	return err
}

/***********    These are Translation Helper Function   ***********/
func (app *AclApp) convertDBAclRulesToInternal(dbCl *db.DB, aclName string, seqId int64, ruleKey db.Key) error {
	var err error
	if seqId != -1 {
		ruleKey.Comp = []string{aclName, "RULE_" + strconv.FormatInt(int64(seqId), 10)}
	}
	if ruleKey.Len() > 1 {
		ruleName := ruleKey.Get(1)
		if ruleName != "DEFAULT_RULE" {
			ruleData, err := dbCl.GetEntry(app.ruleTs, ruleKey)
			if err != nil {
				return err
			}
			if app.ruleTableMap[aclName] == nil {
				app.ruleTableMap[aclName] = make(map[string]db.Value)
			}
			app.ruleTableMap[aclName][ruleName] = ruleData
		}
	} else {
		ruleKeys, err := dbCl.GetKeys(app.ruleTs)
		if err != nil {
			return err
		}
		for i, _ := range ruleKeys {
			if aclName == ruleKeys[i].Get(0) {
				app.convertDBAclRulesToInternal(dbCl, aclName, -1, ruleKeys[i])
			}
		}
	}
	return err
}

func (app *AclApp) convertDBAclToInternal(dbCl *db.DB, aclkey db.Key) error {
	var err error
	if aclkey.Len() > 0 {
		// Get one particular ACL
		entry, err := dbCl.GetEntry(app.aclTs, aclkey)
		if err != nil {
			return err
		}
		if entry.IsPopulated() {
			app.aclTableMap[aclkey.Get(0)] = entry
			app.ruleTableMap[aclkey.Get(0)] = make(map[string]db.Value)
			err = app.convertDBAclRulesToInternal(dbCl, aclkey.Get(0), -1, db.Key{})
			if err != nil {
				return err
			}
		} else {
			return tlerr.NotFound("Acl %s is not configured", aclkey.Get(0))
		}
	} else {
		// Get all ACLs
		tbl, err := dbCl.GetTable(app.aclTs)
		if err != nil {
			return err
		}
		keys, _ := tbl.GetKeys()
		for i, _ := range keys {
			app.convertDBAclToInternal(dbCl, keys[i])
		}
	}
	return err
}

func (app *AclApp) convertInternalToOCAcl(aclName string, aclSets *ocbinds.OpenconfigAcl_Acl_AclSets, aclSet *ocbinds.OpenconfigAcl_Acl_AclSets_AclSet) {
	if len(aclName) > 0 {
		aclData := app.aclTableMap[aclName]
		if aclSet != nil {
			aclSet.Config.Name = aclSet.Name
			aclSet.Config.Type = aclSet.Type
			aclSet.State.Name = aclSet.Name
			aclSet.State.Type = aclSet.Type

			for k := range aclData.Field {
				if ACL_DESCRIPTION == k {
					descr := aclData.Get(k)
					aclSet.Config.Description = &descr
					aclSet.State.Description = &descr
				} else if "ports@" == k {
					continue
				}
			}

			// openconfig-acl-ext: reflect control-plane / management ACL marking
			// (SONiC CTRLPLANE table + services) back into the OC acl-set.
			if aclData.Get(ACL_TYPE) == SONIC_ACL_TYPE_CTRLPLANE {
				cp := true
				aclSet.Config.ControlPlane = &cp
				aclSet.State.ControlPlane = &cp
				if services := aclData.GetList(ACL_SERVICES); len(services) > 0 {
					aclSet.Config.Services = append([]string{}, services...)
					aclSet.State.Services = append([]string{}, services...)
				}
			}

			app.convertInternalToOCAclRule(aclName, aclSet.Type, -1, aclSet, nil)
		}
	} else {
		for acln := range app.aclTableMap {
			acldata := app.aclTableMap[acln]
			var aclNameStr string
			var aclType ocbinds.E_OpenconfigAcl_ACL_TYPE
			if acldata.Get(ACL_TYPE) == SONIC_ACL_TYPE_IPV4 {
				aclNameStr = strings.Replace(acln, "_"+OPENCONFIG_ACL_TYPE_IPV4, "", 1)
				aclType = ocbinds.OpenconfigAcl_ACL_TYPE_ACL_IPV4
			} else if acldata.Get(ACL_TYPE) == SONIC_ACL_TYPE_IPV6 {
				aclNameStr = strings.Replace(acln, "_"+OPENCONFIG_ACL_TYPE_IPV6, "", 1)
				aclType = ocbinds.OpenconfigAcl_ACL_TYPE_ACL_IPV6
			} else if acldata.Get(ACL_TYPE) == SONIC_ACL_TYPE_L2 {
				aclNameStr = strings.Replace(acln, "_"+OPENCONFIG_ACL_TYPE_L2, "", 1)
				aclType = ocbinds.OpenconfigAcl_ACL_TYPE_ACL_L2
			} else if acldata.Get(ACL_TYPE) == SONIC_ACL_TYPE_CTRLPLANE {
				// A control-plane (CTRLPLANE) table does not carry the OC type in
				// its ACL_TYPE field; recover it from the table-name suffix that
				// was preserved when the acl-set was created.
				aclNameStr, aclType = ctrlPlaneOCNameType(acln)
			}
			aclSetPtr, aclErr := aclSets.NewAclSet(aclNameStr, aclType)
			if aclErr != nil {
				fmt.Println("Error handling: ", aclErr)
			}
			ygot.BuildEmptyTree(aclSetPtr)
			app.convertInternalToOCAcl(acln, nil, aclSetPtr)
		}
	}
}

func (app *AclApp) convertInternalToOCAclRule(aclName string, aclType ocbinds.E_OpenconfigAcl_ACL_TYPE, seqId int64, aclSet *ocbinds.OpenconfigAcl_Acl_AclSets_AclSet, entrySet *ocbinds.OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry) {
	if seqId != -1 {
		ruleName := "RULE_" + strconv.FormatInt(int64(seqId), 10)
		app.convertInternalToOCAclRuleProperties(aclName, ruleName, app.ruleTableMap[aclName][ruleName], aclType, nil, entrySet)
	} else {
		for ruleName := range app.ruleTableMap[aclName] {
			app.convertInternalToOCAclRuleProperties(aclName, ruleName, app.ruleTableMap[aclName][ruleName], aclType, aclSet, nil)
		}
	}
}

// aclRuleCounters returns the packet and byte counts of a rule from COUNTERS_DB.
// Counters are published by the flex counter service only while ACL polling is
// enabled ("counterpoll acl enable"); they read as zero otherwise, which is what
// the CLI reports as "N/A". The reason for a zero is logged for diagnosis.
func (app *AclApp) aclRuleCounters(aclKey, ruleKey string) (pkts uint64, octets uint64) {
	if app.countersDb == nil {
		log.V(2).Infof("aclRuleCounters: no counters db handle for %s:%s", aclKey, ruleKey)
		return 0, 0
	}

	// The whole map is fetched once per request; a rule tree can hold thousands
	// of rules, and every one of them would otherwise re-read it.
	if !app.oidsRead {
		oids, err := app.countersDb.GetMapAll(&db.TableSpec{Name: ACL_COUNTER_RULE_MAP})
		if err != nil {
			log.V(2).Infof("aclRuleCounters: %s unavailable; err=%v", ACL_COUNTER_RULE_MAP, err)
		}
		app.counterOids = oids
		app.oidsRead = true
	}

	oid := app.counterOids.Get(aclKey + ":" + ruleKey)
	if len(oid) == 0 {
		log.V(2).Infof("aclRuleCounters: no counter oid for %s:%s", aclKey, ruleKey)
		return 0, 0
	}

	entry, err := app.countersDb.GetEntry(&db.TableSpec{Name: COUNTERS_TABLE}, asKey(oid))
	if err != nil {
		log.V(2).Infof("aclRuleCounters: no counters for oid %s; err=%v", oid, err)
		return 0, 0
	}

	pkts, pErr := strconv.ParseUint(entry.Get(ACL_COUNTER_ATTR_PACKETS), 10, 64)
	octets, oErr := strconv.ParseUint(entry.Get(ACL_COUNTER_ATTR_BYTES), 10, 64)
	if pErr != nil || oErr != nil {
		log.V(2).Infof("aclRuleCounters: oid %s counters unreadable; pkts=%v octets=%v", oid, pErr, oErr)
	}

	return pkts, octets
}

func (app *AclApp) convertInternalToOCAclRuleProperties(aclName string, ruleName string, ruleData db.Value, aclType ocbinds.E_OpenconfigAcl_ACL_TYPE, aclSet *ocbinds.OpenconfigAcl_Acl_AclSets_AclSet, entrySet *ocbinds.OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry) {
	priority, _ := strconv.ParseInt(ruleData.Get("PRIORITY"), 10, 32)
	seqId := uint32(MAX_PRIORITY - priority)
	//ruleDescr := ruleData.Get("RULE_DESCRIPTION")

	if entrySet == nil {
		if aclSet != nil {
			entrySet_, _ := aclSet.AclEntries.NewAclEntry(seqId)
			entrySet = entrySet_
			ygot.BuildEmptyTree(entrySet)
		}
	}

	entrySet.Config.SequenceId = &seqId
	//entrySet.Config.Description = &ruleDescr
	entrySet.State.SequenceId = &seqId
	//entrySet.State.Description = &ruleDescr

	pkts, octets := app.aclRuleCounters(aclName, ruleName)
	entrySet.State.MatchedPackets = &pkts
	entrySet.State.MatchedOctets = &octets

	ygot.BuildEmptyTree(entrySet.Transport)
	ygot.BuildEmptyTree(entrySet.Actions)

	for ruleKey := range ruleData.Field {
		if "L4_SRC_PORT" == ruleKey || "L4_SRC_PORT_RANGE" == ruleKey {
			port := ruleData.Get(ruleKey)
			srcPort := getTransportSrcDestPorts(port, "src")
			entrySet.Transport.Config.SourcePort, _ = entrySet.Transport.Config.To_OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Transport_Config_SourcePort_Union(srcPort)
			entrySet.Transport.State.SourcePort, _ = entrySet.Transport.State.To_OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Transport_State_SourcePort_Union(srcPort)
		} else if "L4_DST_PORT" == ruleKey || "L4_DST_PORT_RANGE" == ruleKey {
			port := ruleData.Get(ruleKey)
			destPort := getTransportSrcDestPorts(port, "dest")
			entrySet.Transport.Config.DestinationPort, _ = entrySet.Transport.Config.To_OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Transport_Config_DestinationPort_Union(destPort)
			entrySet.Transport.State.DestinationPort, _ = entrySet.Transport.State.To_OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Transport_State_DestinationPort_Union(destPort)
		} else if "TCP_FLAGS" == ruleKey {
			tcpFlags := ruleData.Get(ruleKey)
			entrySet.Transport.Config.TcpFlags = getTransportConfigTcpFlags(tcpFlags)
			entrySet.Transport.State.TcpFlags = getTransportConfigTcpFlags(tcpFlags)
		} else if "PACKET_ACTION" == ruleKey {
			// "FORWARD" is the data-plane (SAI) accept value; "ACCEPT" is the
			// control-plane (caclmgrd/iptables) accept value. Both map to the
			// OpenConfig ACCEPT forwarding-action.
			if "FORWARD" == ruleData.Get(ruleKey) || "ACCEPT" == ruleData.Get(ruleKey) {
				entrySet.Actions.Config.ForwardingAction = ocbinds.OpenconfigAcl_FORWARDING_ACTION_ACCEPT
				entrySet.Actions.State.ForwardingAction = ocbinds.OpenconfigAcl_FORWARDING_ACTION_ACCEPT
			} else {
				entrySet.Actions.Config.ForwardingAction = ocbinds.OpenconfigAcl_FORWARDING_ACTION_DROP
				entrySet.Actions.State.ForwardingAction = ocbinds.OpenconfigAcl_FORWARDING_ACTION_DROP
			}
		}
	}

	if aclType == ocbinds.OpenconfigAcl_ACL_TYPE_ACL_IPV4 {
		if entrySet.Ipv4 == nil {
			entrySet.Ipv4 = &ocbinds.OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Ipv4{}
		}
		ygot.BuildEmptyTree(entrySet.Ipv4)
		for ruleKey := range ruleData.Field {
			if "IP_PROTOCOL" == ruleKey {
				ipProto, _ := strconv.ParseInt(ruleData.Get(ruleKey), 10, 64)
				protocolVal := getIpProtocol(ipProto)
				entrySet.Ipv4.Config.Protocol, _ = entrySet.Ipv4.Config.To_OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Ipv4_Config_Protocol_Union(protocolVal)
				entrySet.Ipv4.State.Protocol, _ = entrySet.Ipv4.State.To_OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Ipv4_State_Protocol_Union(protocolVal)
			} else if "DSCP" == ruleKey {
				var dscp uint8
				dscpData, _ := strconv.ParseInt(ruleData.Get(ruleKey), 10, 64)
				dscp = uint8(dscpData)
				entrySet.Ipv4.Config.Dscp = &dscp
				entrySet.Ipv4.State.Dscp = &dscp
			} else if "SRC_IP" == ruleKey {
				addr := ruleData.Get(ruleKey)
				entrySet.Ipv4.Config.SourceAddress = &addr
				entrySet.Ipv4.State.SourceAddress = &addr
			} else if "DST_IP" == ruleKey {
				addr := ruleData.Get(ruleKey)
				entrySet.Ipv4.Config.DestinationAddress = &addr
				entrySet.Ipv4.State.DestinationAddress = &addr
			} else if "ICMP_TYPE" == ruleKey {
				num, err := strconv.ParseUint(ruleData.Get(ruleKey), 10, 8)
				if err != nil {
					log.Warningf("acl %s rule %s: bad ICMP_TYPE %q: %v", aclName, ruleName, ruleData.Get(ruleKey), err)
					continue
				}
				aclIcmpv4EmptyTree(entrySet.Ipv4)
				t := uint8(num)
				entrySet.Ipv4.Icmpv4.Config.Type = &t
				entrySet.Ipv4.Icmpv4.State.Type = &t
			} else if "ICMP_CODE" == ruleKey {
				num, err := strconv.ParseUint(ruleData.Get(ruleKey), 10, 8)
				if err != nil {
					log.Warningf("acl %s rule %s: bad ICMP_CODE %q: %v", aclName, ruleName, ruleData.Get(ruleKey), err)
					continue
				}
				aclIcmpv4EmptyTree(entrySet.Ipv4)
				c := uint8(num)
				entrySet.Ipv4.Icmpv4.Config.Code = &c
				entrySet.Ipv4.Icmpv4.State.Code = &c
			}
		}
	} else if aclType == ocbinds.OpenconfigAcl_ACL_TYPE_ACL_IPV6 {
		if entrySet.Ipv6 == nil {
			entrySet.Ipv6 = &ocbinds.OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Ipv6{}
		}
		ygot.BuildEmptyTree(entrySet.Ipv6)
		for ruleKey := range ruleData.Field {
			if "IP_PROTOCOL" == ruleKey {
				ipProto, _ := strconv.ParseInt(ruleData.Get(ruleKey), 10, 64)
				protocolVal := getIpProtocol(ipProto)
				entrySet.Ipv6.Config.Protocol, _ = entrySet.Ipv6.Config.To_OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Ipv6_Config_Protocol_Union(protocolVal)
				entrySet.Ipv6.State.Protocol, _ = entrySet.Ipv6.State.To_OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Ipv6_State_Protocol_Union(protocolVal)
			} else if "DSCP" == ruleKey {
				var dscp uint8
				dscpData, _ := strconv.ParseInt(ruleData.Get(ruleKey), 10, 64)
				dscp = uint8(dscpData)
				entrySet.Ipv6.Config.Dscp = &dscp
				entrySet.Ipv6.State.Dscp = &dscp
			} else if "SRC_IPV6" == ruleKey {
				addr := ruleData.Get(ruleKey)
				entrySet.Ipv6.Config.SourceAddress = &addr
				entrySet.Ipv6.State.SourceAddress = &addr
			} else if "DST_IPV6" == ruleKey {
				addr := ruleData.Get(ruleKey)
				entrySet.Ipv6.Config.DestinationAddress = &addr
				entrySet.Ipv6.State.DestinationAddress = &addr
			} else if "ICMPV6_TYPE" == ruleKey {
				// Reverse-map the numeric ICMPv6 type back to the standard
				// openconfig-icmpv6-types identity.
				num, err := strconv.ParseUint(ruleData.Get(ruleKey), 10, 8)
				if err != nil {
					log.Warningf("acl %s rule %s: bad ICMPV6_TYPE %q: %v", aclName, ruleName, ruleData.Get(ruleKey), err)
					continue
				}
				// A numeric ICMPv6 type outside the standard openconfig-icmpv6-types
				// identity set has no identityref value to represent it, so it is
				// enforced in the data plane but cannot be surfaced here.
				if id := getIcmpv6TypeId(uint8(num)); id != ocbinds.OpenconfigIcmpv6Types_TYPE_UNSET {
					aclIcmpv6EmptyTree(entrySet.Ipv6)
					entrySet.Ipv6.Icmpv6.Config.Type = id
					entrySet.Ipv6.Icmpv6.State.Type = id
				} else {
					log.Warningf("acl %s rule %s: ICMPV6_TYPE %d has no openconfig-icmpv6-types identity; omitted from GET", aclName, ruleName, num)
				}
			}
		}
	} else if aclType == ocbinds.OpenconfigAcl_ACL_TYPE_ACL_L2 {
		ygot.BuildEmptyTree(entrySet.L2)
		for ruleKey := range ruleData.Field {
			if "ETHER_TYPE" == ruleKey {
				ethType, _ := strconv.ParseUint(strings.Replace(ruleData.Get(ruleKey), "0x", "", -1), 16, 32)
				ethertype := getL2EtherType(ethType)
				entrySet.L2.Config.Ethertype, _ = entrySet.L2.Config.To_OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_L2_Config_Ethertype_Union(ethertype)
				entrySet.L2.State.Ethertype, _ = entrySet.L2.State.To_OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_L2_State_Ethertype_Union(ethertype)
			}
		}
	}
}

func (app *AclApp) convertInternalToOCAclRuleBinding(aclKey string, ruleKey string, priority uint32, seqId int64, direction string, aclSet ygot.GoStruct, entrySet ygot.GoStruct) {
	if seqId == -1 {
		seqId = int64(MAX_PRIORITY - priority)
	}

	pkts, octets := app.aclRuleCounters(aclKey, ruleKey)
	var ruleId uint32 = uint32(seqId)

	if direction == "INGRESS" {
		var ingressEntrySet *ocbinds.OpenconfigAcl_Acl_Interfaces_Interface_IngressAclSets_IngressAclSet_AclEntries_AclEntry
		var ok bool
		if entrySet == nil {
			ingressAclSet := aclSet.(*ocbinds.OpenconfigAcl_Acl_Interfaces_Interface_IngressAclSets_IngressAclSet)
			if ingressEntrySet, ok = ingressAclSet.AclEntries.AclEntry[ruleId]; !ok {
				ingressEntrySet, _ = ingressAclSet.AclEntries.NewAclEntry(ruleId)
			}
		} else {
			ingressEntrySet = entrySet.(*ocbinds.OpenconfigAcl_Acl_Interfaces_Interface_IngressAclSets_IngressAclSet_AclEntries_AclEntry)
		}
		if ingressEntrySet != nil {
			ygot.BuildEmptyTree(ingressEntrySet)
			ingressEntrySet.State.SequenceId = &ruleId
			ingressEntrySet.State.MatchedPackets = &pkts
			ingressEntrySet.State.MatchedOctets = &octets
		}
	} else if direction == "EGRESS" {
		var egressEntrySet *ocbinds.OpenconfigAcl_Acl_Interfaces_Interface_EgressAclSets_EgressAclSet_AclEntries_AclEntry
		var ok bool
		if entrySet == nil {
			egressAclSet := aclSet.(*ocbinds.OpenconfigAcl_Acl_Interfaces_Interface_EgressAclSets_EgressAclSet)
			if egressEntrySet, ok = egressAclSet.AclEntries.AclEntry[ruleId]; !ok {
				egressEntrySet, _ = egressAclSet.AclEntries.NewAclEntry(ruleId)
			}
		} else {
			egressEntrySet = entrySet.(*ocbinds.OpenconfigAcl_Acl_Interfaces_Interface_EgressAclSets_EgressAclSet_AclEntries_AclEntry)
		}
		if egressEntrySet != nil {
			ygot.BuildEmptyTree(egressEntrySet)
			egressEntrySet.State.SequenceId = &ruleId
			egressEntrySet.State.MatchedPackets = &pkts
			egressEntrySet.State.MatchedOctets = &octets
		}
	}
}

func (app *AclApp) convertInternalToOCAclBinding(d *db.DB, aclName string, intfId string, direction string, intfAclSet ygot.GoStruct) error {
	var err error
	if _, ok := app.aclTableMap[aclName]; !ok {
		aclEntry, err1 := d.GetEntry(app.aclTs, db.Key{Comp: []string{aclName}})
		if err1 != nil {
			return err1
		}
		if !contains(aclEntry.GetList("ports"), intfId) {
			return tlerr.InvalidArgs("Acl %s not binded with %s", aclName, intfId)
		}
	}

	if _, ok := app.ruleTableMap[aclName]; !ok {
		ruleKeys, _ := d.GetKeys(app.ruleTs)
		for i, _ := range ruleKeys {
			rulekey := ruleKeys[i]
			// Rulekey has two keys, first aclkey and second rulename
			if rulekey.Get(0) == aclName && rulekey.Get(1) != "DEFAULT_RULE" {
				seqId, _ := strconv.Atoi(strings.Replace(rulekey.Get(1), "RULE_", "", 1))
				app.convertInternalToOCAclRuleBinding(aclName, rulekey.Get(1), 0, int64(seqId), direction, intfAclSet, nil)
			}
		}
	} else {
		for ruleName := range app.ruleTableMap[aclName] {
			if ruleName != "DEFAULT_RULE" {
				seqId, _ := strconv.Atoi(strings.Replace(ruleName, "RULE_", "", 1))
				app.convertInternalToOCAclRuleBinding(aclName, ruleName, 0, int64(seqId), direction, intfAclSet, nil)
			}
		}
	}

	return err
}

func (app *AclApp) getAllBindingsInfo(d *db.DB) error {
	var err error
	acl := app.getAppRootObject()
	if len(app.aclTableMap) == 0 {
		aclKeys, _ := d.GetKeys(app.aclTs)
		for i, _ := range aclKeys {
			aclEntry, _ := d.GetEntry(app.aclTs, aclKeys[i])
			app.aclTableMap[(aclKeys[i]).Get(0)] = aclEntry
		}
	}
	var interfaces []string
	for aclName := range app.aclTableMap {
		aclData := app.aclTableMap[aclName]
		if len(aclData.Get("ports@")) > 0 {
			aclIntfs := aclData.GetList("ports")
			for i, _ := range aclIntfs {
				if !contains(interfaces, aclIntfs[i]) && aclIntfs[i] != "" {
					interfaces = append(interfaces, aclIntfs[i])
				}
			}
		}
	}

	for _, intfId := range interfaces {
		var intfData *ocbinds.OpenconfigAcl_Acl_Interfaces_Interface
		intfData, ok := acl.Interfaces.Interface[intfId]
		if !ok {
			intfData, _ = acl.Interfaces.NewInterface(intfId)
		}
		ygot.BuildEmptyTree(intfData)
		err = app.getAclBindingInfoForInterfaceData(d, intfData, intfId, "INGRESS")
		err = app.getAclBindingInfoForInterfaceData(d, intfData, intfId, "EGRESS")
	}
	return err
}

func (app *AclApp) getAclBindingInfoForInterfaceData(d *db.DB, intfData *ocbinds.OpenconfigAcl_Acl_Interfaces_Interface, intfId string, direction string) error {
	var err error
	if intfData != nil {
		intfData.Config.Id = intfData.Id
		intfData.State.Id = intfData.Id
	}
	if direction == "INGRESS" {
		if intfData.IngressAclSets != nil && len(intfData.IngressAclSets.IngressAclSet) > 0 {
			for ingressAclSetKey, _ := range intfData.IngressAclSets.IngressAclSet {
				aclName := strings.Replace(strings.Replace(ingressAclSetKey.SetName, " ", "_", -1), "-", "_", -1)
				aclType := ingressAclSetKey.Type.ΛMap()["E_OpenconfigAcl_ACL_TYPE"][int64(ingressAclSetKey.Type)].Name
				aclKey := aclName + "_" + aclType

				ingressAclSet := intfData.IngressAclSets.IngressAclSet[ingressAclSetKey]
				if ingressAclSet != nil && ingressAclSet.AclEntries != nil && len(ingressAclSet.AclEntries.AclEntry) > 0 {
					for seqId, _ := range ingressAclSet.AclEntries.AclEntry {
						rulekey := "RULE_" + strconv.Itoa(int(seqId))
						entrySet := ingressAclSet.AclEntries.AclEntry[seqId]
						_, err := d.GetEntry(app.ruleTs, db.Key{Comp: []string{aclKey, rulekey}})
						if err != nil {
							return err
						}
						app.convertInternalToOCAclRuleBinding(aclKey, rulekey, 0, int64(seqId), direction, nil, entrySet)
					}
				} else {
					ygot.BuildEmptyTree(ingressAclSet)
					ingressAclSet.Config = &ocbinds.OpenconfigAcl_Acl_Interfaces_Interface_IngressAclSets_IngressAclSet_Config{SetName: &aclName, Type: ingressAclSetKey.Type}
					ingressAclSet.State = &ocbinds.OpenconfigAcl_Acl_Interfaces_Interface_IngressAclSets_IngressAclSet_State{SetName: &aclName, Type: ingressAclSetKey.Type}
					err = app.convertInternalToOCAclBinding(d, aclKey, intfId, direction, ingressAclSet)
				}
			}
		} else {
			err = app.findAndGetAclBindingInfoForInterfaceData(d, intfId, direction, intfData)
		}
	} else if direction == "EGRESS" {
		if intfData.EgressAclSets != nil && len(intfData.EgressAclSets.EgressAclSet) > 0 {
			for egressAclSetKey, _ := range intfData.EgressAclSets.EgressAclSet {
				aclName := strings.Replace(strings.Replace(egressAclSetKey.SetName, " ", "_", -1), "-", "_", -1)
				aclType := egressAclSetKey.Type.ΛMap()["E_OpenconfigAcl_ACL_TYPE"][int64(egressAclSetKey.Type)].Name
				aclKey := aclName + "_" + aclType

				egressAclSet := intfData.EgressAclSets.EgressAclSet[egressAclSetKey]
				if egressAclSet != nil && egressAclSet.AclEntries != nil && len(egressAclSet.AclEntries.AclEntry) > 0 {
					for seqId, _ := range egressAclSet.AclEntries.AclEntry {
						rulekey := "RULE_" + strconv.Itoa(int(seqId))
						entrySet := egressAclSet.AclEntries.AclEntry[seqId]
						_, err := d.GetEntry(app.ruleTs, db.Key{Comp: []string{aclKey, rulekey}})
						if err != nil {
							return err
						}
						app.convertInternalToOCAclRuleBinding(aclKey, rulekey, 0, int64(seqId), direction, nil, entrySet)
					}
				} else {
					ygot.BuildEmptyTree(egressAclSet)
					egressAclSet.Config = &ocbinds.OpenconfigAcl_Acl_Interfaces_Interface_EgressAclSets_EgressAclSet_Config{SetName: &aclName, Type: egressAclSetKey.Type}
					egressAclSet.State = &ocbinds.OpenconfigAcl_Acl_Interfaces_Interface_EgressAclSets_EgressAclSet_State{SetName: &aclName, Type: egressAclSetKey.Type}
					err = app.convertInternalToOCAclBinding(d, aclKey, intfId, direction, egressAclSet)
				}
			}
		} else {
			err = app.findAndGetAclBindingInfoForInterfaceData(d, intfId, direction, intfData)
		}
	} else {
		log.Error("Unknown direction")
	}
	return err
}

func (app *AclApp) findAndGetAclBindingInfoForInterfaceData(d *db.DB, intfId string, direction string, intfData *ocbinds.OpenconfigAcl_Acl_Interfaces_Interface) error {
	var err error
	if len(app.aclTableMap) == 0 {
		aclKeys, _ := d.GetKeys(app.aclTs)
		for i, _ := range aclKeys {
			aclEntry, _ := d.GetEntry(app.aclTs, aclKeys[i])
			app.aclTableMap[aclKeys[i].Get(0)] = aclEntry
		}
	}

	for aclName, _ := range app.aclTableMap {
		aclData := app.aclTableMap[aclName]
		aclIntfs := aclData.GetList("ports")
		aclType := aclData.Get(ACL_TYPE)
		var aclOrigName string
		var aclOrigType ocbinds.E_OpenconfigAcl_ACL_TYPE
		if SONIC_ACL_TYPE_IPV4 == aclType {
			aclOrigName = strings.Replace(aclName, "_"+OPENCONFIG_ACL_TYPE_IPV4, "", 1)
			aclOrigType = ocbinds.OpenconfigAcl_ACL_TYPE_ACL_IPV4
		} else if SONIC_ACL_TYPE_IPV6 == aclType {
			aclOrigName = strings.Replace(aclName, "_"+OPENCONFIG_ACL_TYPE_IPV6, "", 1)
			aclOrigType = ocbinds.OpenconfigAcl_ACL_TYPE_ACL_IPV6
		} else if SONIC_ACL_TYPE_L2 == aclType {
			aclOrigName = strings.Replace(aclName, "_"+OPENCONFIG_ACL_TYPE_L2, "", 1)
			aclOrigType = ocbinds.OpenconfigAcl_ACL_TYPE_ACL_L2
		}

		if contains(aclIntfs, intfId) && direction == aclData.Get("stage") {
			if direction == "INGRESS" {
				if intfData.IngressAclSets != nil {
					aclSetKey := ocbinds.OpenconfigAcl_Acl_Interfaces_Interface_IngressAclSets_IngressAclSet_Key{SetName: aclOrigName, Type: aclOrigType}
					ingressAclSet, ok := intfData.IngressAclSets.IngressAclSet[aclSetKey]
					if !ok {
						ingressAclSet, _ = intfData.IngressAclSets.NewIngressAclSet(aclOrigName, aclOrigType)
						ygot.BuildEmptyTree(ingressAclSet)
						ingressAclSet.Config = &ocbinds.OpenconfigAcl_Acl_Interfaces_Interface_IngressAclSets_IngressAclSet_Config{SetName: &aclOrigName, Type: aclOrigType}
						ingressAclSet.State = &ocbinds.OpenconfigAcl_Acl_Interfaces_Interface_IngressAclSets_IngressAclSet_State{SetName: &aclOrigName, Type: aclOrigType}
					}
					err = app.convertInternalToOCAclBinding(d, aclName, intfId, direction, ingressAclSet)
					if err != nil {
						return err
					}
				}
			} else if direction == "EGRESS" {
				if intfData.EgressAclSets != nil {
					aclSetKey := ocbinds.OpenconfigAcl_Acl_Interfaces_Interface_EgressAclSets_EgressAclSet_Key{SetName: aclOrigName, Type: aclOrigType}
					egressAclSet, ok := intfData.EgressAclSets.EgressAclSet[aclSetKey]
					if !ok {
						egressAclSet, _ = intfData.EgressAclSets.NewEgressAclSet(aclOrigName, aclOrigType)
						ygot.BuildEmptyTree(egressAclSet)
						egressAclSet.Config = &ocbinds.OpenconfigAcl_Acl_Interfaces_Interface_EgressAclSets_EgressAclSet_Config{SetName: &aclOrigName, Type: aclOrigType}
						egressAclSet.State = &ocbinds.OpenconfigAcl_Acl_Interfaces_Interface_EgressAclSets_EgressAclSet_State{SetName: &aclOrigName, Type: aclOrigType}
					}
					err = app.convertInternalToOCAclBinding(d, aclName, intfId, direction, egressAclSet)
					if err != nil {
						return err
					}
				}
			}
		}
	}
	return err
}

/*func (app *AclApp) isInterfaceBindWithACL(d *db.DB, intfId string) bool {
	var isFound bool = false

	if len(app.aclTableMap) == 0 {
		aclKeys, _ := d.GetKeys(app.aclTs)
		for i, _ := range aclKeys {
			aclEntry, _ := d.GetEntry(app.aclTs, aclKeys[i])
			app.aclTableMap[(aclKeys[i]).Get(0)] = aclEntry
		}
	}

	var interfaces []string
	for aclName := range app.aclTableMap {
		aclData := app.aclTableMap[aclName]
		if len(aclData.Get("ports@")) > 0 {
			aclIntfs := aclData.GetList("ports")
			for i, _ := range aclIntfs {
				if !contains(interfaces, aclIntfs[i]) && aclIntfs[i] != "" {
					interfaces = append(interfaces, aclIntfs[i])
				}
			}
		}
	}

	isFound = contains(interfaces, intfId)
	return isFound
}*/

func (app *AclApp) handleBindingsDeletion(d *db.DB) error {
	var err error

	acl := app.getAppRootObject()
	aclKeys, _ := d.GetKeys(app.aclTs)
	for i, _ := range aclKeys {
		aclEntry, _ := d.GetEntry(app.aclTs, aclKeys[i])
		var isRequestedAclFound = false
		if len(aclEntry.GetList("ports")) > 0 {
			if isSubtreeRequest(app.pathInfo.Template, "/openconfig-acl:acl/interfaces/interface{}") {
				direction := aclEntry.Get("stage")
				if isSubtreeRequest(app.pathInfo.Template, "/openconfig-acl:acl/interfaces/interface{}/ingress-acl-sets") && direction != "INGRESS" {
					return tlerr.InvalidArgs("Acl %s is not Ingress", aclKeys[i].Get(0))
				}
				if isSubtreeRequest(app.pathInfo.Template, "/openconfig-acl:acl/interfaces/interface{}/egress-acl-sets") && direction != "EGRESS" {
					return tlerr.InvalidArgs("Acl %s is not Egress", aclKeys[i].Get(0))
				}
				for intfId := range acl.Interfaces.Interface {
					aclname, acltype := getAclKeysFromStrKey(aclKeys[i].Get(0), aclEntry.Get("type"))
					intfData := acl.Interfaces.Interface[intfId]
					if isSubtreeRequest(app.pathInfo.Template, "/openconfig-acl:acl/interfaces/interface{}/ingress-acl-sets/ingress-acl-set{}{}") {
						for k := range intfData.IngressAclSets.IngressAclSet {
							if aclname == k.SetName {
								if acltype == k.Type {
									isRequestedAclFound = true
								} else {
									return tlerr.InvalidArgs("Acl Type is not matching")
								}
							} else {
								goto SkipDBProcessing
							}
						}
					} else if isSubtreeRequest(app.pathInfo.Template, "/openconfig-acl:acl/interfaces/interface{}/egress-acl-sets/egress-acl-set{}{}") {
						for k := range intfData.EgressAclSets.EgressAclSet {
							if aclname == k.SetName {
								if acltype == k.Type {
									isRequestedAclFound = true
								} else {
									return tlerr.InvalidArgs("Acl Type is not matching")
								}
							} else {
								goto SkipDBProcessing
							}
						}
					}
					intfs := aclEntry.GetList("ports")
					intfs = removeElement(intfs, intfId)
					aclEntry.SetList("ports", intfs)
					err = d.SetEntry(app.aclTs, aclKeys[i], aclEntry)
					if err != nil {
						return err
					}
					// If last interface removed, then remove stage field also
					if len(intfs) == 0 {
						aclEntry.Remove("stage")
					}
				}
			SkipDBProcessing:
			} else {
				aclEntry.Remove("stage")
				aclEntry.SetList("ports", []string{})
				err = d.SetEntry(app.aclTs, aclKeys[i], aclEntry)
				if err != nil {
					return err
				}
			}
		}
		if isRequestedAclFound {
			break
		}
	}

	return err
}

/********************   CREATE related    *******************************/
func (app *AclApp) convertOCAclsToInternal() {
	acl := app.getAppRootObject()
	if acl != nil {
		app.aclTableMap = make(map[string]db.Value)
		if acl.AclSets != nil && len(acl.AclSets.AclSet) > 0 {
			for aclSetKey, _ := range acl.AclSets.AclSet {
				aclSet := acl.AclSets.AclSet[aclSetKey]
				aclKey := getAclKeyStrFromOCKey(aclSetKey.Name, aclSetKey.Type)
				app.aclTableMap[aclKey] = db.Value{Field: map[string]string{}}

				if aclSet.Config != nil {
					if aclSet.Config.Type == ocbinds.OpenconfigAcl_ACL_TYPE_ACL_IPV4 {
						app.aclTableMap[aclKey].Field[ACL_TYPE] = SONIC_ACL_TYPE_IPV4
					} else if aclSet.Config.Type == ocbinds.OpenconfigAcl_ACL_TYPE_ACL_IPV6 {
						app.aclTableMap[aclKey].Field[ACL_TYPE] = SONIC_ACL_TYPE_IPV6
					} else if aclSet.Config.Type == ocbinds.OpenconfigAcl_ACL_TYPE_ACL_L2 {
						app.aclTableMap[aclKey].Field[ACL_TYPE] = SONIC_ACL_TYPE_L2
					}

					// openconfig-acl-ext: a control-plane / management ACL
					// (restrict-access) becomes a SONiC CTRLPLANE table bound to
					// management services. The acl-set key still carries the
					// ACL_IPV4/ACL_IPV6 OC type (used to interpret rule fields and
					// to reconstruct the OC type on read-back from the table name
					// suffix); only the persisted ACL_TABLE:type is overridden.
					if aclSet.Config.ControlPlane != nil && *aclSet.Config.ControlPlane {
						app.aclTableMap[aclKey].Field[ACL_TYPE] = SONIC_ACL_TYPE_CTRLPLANE
						if len(aclSet.Config.Services) > 0 {
							val := app.aclTableMap[aclKey]
							(&val).SetList(ACL_SERVICES, aclSet.Config.Services)
							app.aclTableMap[aclKey] = val
						}
					}

					if aclSet.Config.Description != nil && len(*aclSet.Config.Description) > 0 {
						app.aclTableMap[aclKey].Field[ACL_DESCRIPTION] = *aclSet.Config.Description
					}
				}
			}
		}
	}
}

func (app *AclApp) convertOCAclRulesToInternal(d *db.DB) {
	acl := app.getAppRootObject()
	if acl != nil {
		app.ruleTableMap = make(map[string]map[string]db.Value)
		if acl.AclSets != nil && len(acl.AclSets.AclSet) > 0 {
			for aclSetKey, _ := range acl.AclSets.AclSet {
				aclSet := acl.AclSets.AclSet[aclSetKey]
				aclKey := getAclKeyStrFromOCKey(aclSetKey.Name, aclSetKey.Type)
				app.ruleTableMap[aclKey] = make(map[string]db.Value)

				// Control-plane (CTRLPLANE) ACLs are enforced by caclmgrd, which
				// applies its own default behavior per management service; do not
				// inject the data-plane default-deny rule for them.
				isControlPlane := aclSet.Config != nil && aclSet.Config.ControlPlane != nil && *aclSet.Config.ControlPlane

				if aclSet.AclEntries != nil {
					for seqId, _ := range aclSet.AclEntries.AclEntry {
						entrySet := aclSet.AclEntries.AclEntry[seqId]
						ruleName := "RULE_" + strconv.Itoa(int(seqId))
						app.ruleTableMap[aclKey][ruleName] = db.Value{Field: map[string]string{}}
						convertOCAclRuleToInternalAclRule(app.ruleTableMap[aclKey][ruleName], seqId, aclKey, aclSet.Type, entrySet)
						// caclmgrd uses PACKET_ACTION directly as the iptables
						// jump target (-j ACCEPT/DROP), so a control-plane "accept"
						// must be ACCEPT rather than the data-plane SAI "FORWARD".
						if isControlPlane && app.ruleTableMap[aclKey][ruleName].Field["PACKET_ACTION"] == "FORWARD" {
							app.ruleTableMap[aclKey][ruleName].Field["PACKET_ACTION"] = "ACCEPT"
						}
					}
				}

				yangPathStr, _ := getYangPathFromUri(app.pathInfo.Path)
				if !isControlPlane && yangPathStr != "/openconfig-acl:acl/acl-sets/acl-set/acl-entries" && yangPathStr != "/openconfig-acl:acl/acl-sets/acl-set/acl-entries/acl-entry" {
					app.createDefaultDenyAclRule(d, aclKey, app.ruleTableMap[aclKey])
				}
			}
		}
	}
}

func (app *AclApp) convertOCAclBindingsToInternal() {
	aclObj := app.getAppRootObject()

	if aclObj.Interfaces != nil && len(aclObj.Interfaces.Interface) > 0 {
		aclInterfacesMap := make(map[string][]string)
		// Below code assumes that an ACL can be either INGRESS or EGRESS but not both.
		for intfId, _ := range aclObj.Interfaces.Interface {
			intf := aclObj.Interfaces.Interface[intfId]
			if intf != nil {
				if intf.IngressAclSets != nil && len(intf.IngressAclSets.IngressAclSet) > 0 {
					for inAclKey, _ := range intf.IngressAclSets.IngressAclSet {
						aclName := getAclKeyStrFromOCKey(inAclKey.SetName, inAclKey.Type)
						// TODO: Need to handle Subinterface also
						if intf.InterfaceRef != nil && intf.InterfaceRef.Config.Interface != nil {
							aclInterfacesMap[aclName] = append(aclInterfacesMap[aclName], *intf.InterfaceRef.Config.Interface)
						} else {
							aclInterfacesMap[aclName] = append(aclInterfacesMap[aclName], *intf.Id)
						}
						if len(app.aclTableMap) == 0 {
							app.aclTableMap[aclName] = db.Value{Field: map[string]string{}}
						}
						app.aclTableMap[aclName].Field["stage"] = "INGRESS"
					}
				}

				if intf.EgressAclSets != nil && len(intf.EgressAclSets.EgressAclSet) > 0 {
					for outAclKey, _ := range intf.EgressAclSets.EgressAclSet {
						aclName := getAclKeyStrFromOCKey(outAclKey.SetName, outAclKey.Type)
						if intf.InterfaceRef != nil && intf.InterfaceRef.Config.Interface != nil {
							aclInterfacesMap[aclName] = append(aclInterfacesMap[aclName], *intf.InterfaceRef.Config.Interface)
						} else {
							aclInterfacesMap[aclName] = append(aclInterfacesMap[aclName], *intf.Id)
						}
						if len(app.aclTableMap) == 0 {
							app.aclTableMap[aclName] = db.Value{Field: map[string]string{}}
						}
						app.aclTableMap[aclName].Field["stage"] = "EGRESS"
					}
				}
			}
		}
		for k, _ := range aclInterfacesMap {
			val := app.aclTableMap[k]
			(&val).SetList("ports", aclInterfacesMap[k])
		}
	}
}

func (app *AclApp) createDefaultDenyAclRule(d *db.DB, aclName string, rulesInfo map[string]db.Value) {
	existingRuleEntry, err := d.GetEntry(app.ruleTs, db.Key{Comp: []string{aclName, "DEFAULT_RULE"}})
	// If Default Rule already exists, Do not add new Default Rule
	if existingRuleEntry.IsPopulated() && err == nil {
		return
	}
	m := make(map[string]string)
	rulesInfo["DEFAULT_RULE"] = db.Value{Field: m}
	rulesInfo["DEFAULT_RULE"].Field["PRIORITY"] = strconv.FormatInt(int64(MIN_PRIORITY), 10)
	rulesInfo["DEFAULT_RULE"].Field["PACKET_ACTION"] = "DROP"
	rulesInfo["DEFAULT_RULE"].Field["IP_TYPE"] = "ANY"
}

func convertOCAclRuleToInternalAclRule(ruleData db.Value, seqId uint32, aclName string, aclType ocbinds.E_OpenconfigAcl_ACL_TYPE, rule *ocbinds.OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry) {
	ruleIndex := seqId
	ruleData.Field["PRIORITY"] = strconv.FormatInt(int64(MAX_PRIORITY-ruleIndex), 10)
	// Rule Description is not supported in Sonic. So commenting this out.
	/*
	   if rule.Config != nil && rule.Config.Description != nil {
	       ruleData.Field["RULE_DESCRIPTION"] = *rule.Config.Description
	   }
	*/

	if ocbinds.OpenconfigAcl_ACL_TYPE_ACL_IPV4 == aclType {
		convertOCToInternalIPv4(ruleData, aclName, ruleIndex, rule)
	} else if ocbinds.OpenconfigAcl_ACL_TYPE_ACL_IPV6 == aclType {
		convertOCToInternalIPv6(ruleData, aclName, ruleIndex, rule)
	} else if ocbinds.OpenconfigAcl_ACL_TYPE_ACL_L2 == aclType {
		convertOCToInternalL2(ruleData, aclName, ruleIndex, rule)
	} /*else if ocbinds.OpenconfigAcl_ACL_TYPE_ACL_MIXED == aclType {
	  } */

	convertOCToInternalTransport(ruleData, aclName, ruleIndex, rule)
	convertOCToInternalInputInterface(ruleData, aclName, ruleIndex, rule)
	convertOCToInternalInputAction(ruleData, aclName, ruleIndex, rule)
}

func convertOCToInternalL2(ruleData db.Value, aclName string, ruleIndex uint32, rule *ocbinds.OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry) {
	if rule.L2 == nil {
		return
	}
	if rule.L2.Config.Ethertype != nil && util.IsTypeStructPtr(reflect.TypeOf(rule.L2.Config.Ethertype)) {
		ethertypeType := reflect.TypeOf(rule.L2.Config.Ethertype).Elem()
		var b bytes.Buffer
		switch ethertypeType {
		case reflect.TypeOf(ocbinds.OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_L2_Config_Ethertype_Union_E_OpenconfigPacketMatchTypes_ETHERTYPE{}):
			v := (rule.L2.Config.Ethertype).(*ocbinds.OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_L2_Config_Ethertype_Union_E_OpenconfigPacketMatchTypes_ETHERTYPE)
			//ruleData["ETHER_TYPE"] = v.E_OpenconfigPacketMatchTypes_ETHERTYPE.ΛMap()["E_OpenconfigPacketMatchTypes_ETHERTYPE"][int64(v.E_OpenconfigPacketMatchTypes_ETHERTYPE)].Name
			fmt.Fprintf(&b, "0x%0.4x", ETHERTYPE_MAP[v.E_OpenconfigPacketMatchTypes_ETHERTYPE])
			ruleData.Field["ETHER_TYPE"] = b.String()
		case reflect.TypeOf(ocbinds.OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_L2_Config_Ethertype_Union_Uint16{}):
			v := (rule.L2.Config.Ethertype).(*ocbinds.OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_L2_Config_Ethertype_Union_Uint16)
			fmt.Fprintf(&b, "0x%0.4x", v.Uint16)
			ruleData.Field["ETHER_TYPE"] = b.String()
		}
	}
}

// aclIcmpv4EmptyTree ensures the icmpv4 match container and its Config/State
// children exist on an IPv4 ACL entry before its type/code leaves are populated.
// It is called lazily from the ICMP_TYPE/ICMP_CODE branches only, so rules that
// carry no ICMP fields never get an icmpv4 container in their GET output.
//
// ygot.BuildEmptyTree on the parent Ipv4 does not build icmpv4's Config/State in
// the targeted-subtree GET flow (a GET of .../ipv4/icmpv4): the translib
// framework pre-allocates the icmpv4 target container but leaves Config/State
// nil, and BuildEmptyTree only descends into nil children. BuildEmptyTree itself
// only fills nil fields, so calling it here unconditionally is safe whether
// Icmpv4 was just allocated or was pre-allocated empty by the framework; guarding
// it behind the nil check would leave Config/State nil and crash on assignment.
func aclIcmpv4EmptyTree(ipv4 *ocbinds.OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Ipv4) {
	if ipv4.Icmpv4 == nil {
		ipv4.Icmpv4 = &ocbinds.OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Ipv4_Icmpv4{}
	}
	ygot.BuildEmptyTree(ipv4.Icmpv4)
}

// aclIcmpv6EmptyTree is the ICMPv6 counterpart of aclIcmpv4EmptyTree.
func aclIcmpv6EmptyTree(ipv6 *ocbinds.OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Ipv6) {
	if ipv6.Icmpv6 == nil {
		ipv6.Icmpv6 = &ocbinds.OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Ipv6_Icmpv6{}
	}
	// See aclIcmpv4EmptyTree: BuildEmptyTree must run even when the framework
	// pre-allocated an empty Icmpv6 target container (targeted /ipv6/icmpv6 GET).
	ygot.BuildEmptyTree(ipv6.Icmpv6)
}

func convertOCToInternalIPv4(ruleData db.Value, aclName string, ruleIndex uint32, rule *ocbinds.OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry) {
	if rule.Ipv4 == nil {
		return
	}
	// ipv4/config and ipv4/icmpv4 are sibling containers, so a subtree write
	// that only touches icmpv4 leaves Config nil; guard the config-field
	// derefs accordingly.
	if rule.Ipv4.Config != nil {
		if rule.Ipv4.Config.Protocol != nil && util.IsTypeStructPtr(reflect.TypeOf(rule.Ipv4.Config.Protocol)) {
			protocolType := reflect.TypeOf(rule.Ipv4.Config.Protocol).Elem()
			switch protocolType {
			case reflect.TypeOf(ocbinds.OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Ipv4_Config_Protocol_Union_E_OpenconfigPacketMatchTypes_IP_PROTOCOL{}):
				v := (rule.Ipv4.Config.Protocol).(*ocbinds.OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Ipv4_Config_Protocol_Union_E_OpenconfigPacketMatchTypes_IP_PROTOCOL)
				//ruleData["IP_PROTOCOL"] = v.E_OpenconfigPacketMatchTypes_IP_PROTOCOL.ΛMap()["E_OpenconfigPacketMatchTypes_IP_PROTOCOL"][int64(v.E_OpenconfigPacketMatchTypes_IP_PROTOCOL)].Name
				ruleData.Field["IP_PROTOCOL"] = strconv.FormatInt(int64(IP_PROTOCOL_MAP[v.E_OpenconfigPacketMatchTypes_IP_PROTOCOL]), 10)
			case reflect.TypeOf(ocbinds.OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Ipv4_Config_Protocol_Union_Uint8{}):
				v := (rule.Ipv4.Config.Protocol).(*ocbinds.OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Ipv4_Config_Protocol_Union_Uint8)
				ruleData.Field["IP_PROTOCOL"] = strconv.FormatInt(int64(v.Uint8), 10)
			}
		}

		if rule.Ipv4.Config.Dscp != nil {
			ruleData.Field["DSCP"] = strconv.FormatInt(int64(*rule.Ipv4.Config.Dscp), 10)
		}
		if rule.Ipv4.Config.SourceAddress != nil {
			ruleData.Field["SRC_IP"] = *rule.Ipv4.Config.SourceAddress
		}
		if rule.Ipv4.Config.DestinationAddress != nil {
			ruleData.Field["DST_IP"] = *rule.Ipv4.Config.DestinationAddress
		}
	}
	// openconfig-packet-match ICMPv4 type/code match (SONiC ICMP_TYPE and
	// ICMP_CODE). Both are the numeric values, so no identity map is needed.
	// Restricting ICMP means pairing these with IP_PROTOCOL 1 and a drop
	// action, for example to drop ICMP redirects (type 5).
	if rule.Ipv4.Icmpv4 != nil && rule.Ipv4.Icmpv4.Config != nil {
		if rule.Ipv4.Icmpv4.Config.Type != nil {
			ruleData.Field["ICMP_TYPE"] = strconv.FormatInt(int64(*rule.Ipv4.Icmpv4.Config.Type), 10)
		}
		if rule.Ipv4.Icmpv4.Config.Code != nil {
			ruleData.Field["ICMP_CODE"] = strconv.FormatInt(int64(*rule.Ipv4.Icmpv4.Config.Code), 10)
		}
	}
	ruleData.Field["IP_TYPE"] = "IPV4ANY"
}

func convertOCToInternalIPv6(ruleData db.Value, aclName string, ruleIndex uint32, rule *ocbinds.OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry) {
	if rule.Ipv6 == nil {
		return
	}
	// ipv6/config and ipv6/icmpv6 are sibling containers, so a subtree write
	// that only touches icmpv6 leaves Config nil; guard the config-field
	// derefs accordingly.
	if rule.Ipv6.Config != nil {
		if rule.Ipv6.Config.Protocol != nil && util.IsTypeStructPtr(reflect.TypeOf(rule.Ipv6.Config.Protocol)) {
			protocolType := reflect.TypeOf(rule.Ipv6.Config.Protocol).Elem()
			switch protocolType {
			case reflect.TypeOf(ocbinds.OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Ipv6_Config_Protocol_Union_E_OpenconfigPacketMatchTypes_IP_PROTOCOL{}):
				v := (rule.Ipv6.Config.Protocol).(*ocbinds.OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Ipv6_Config_Protocol_Union_E_OpenconfigPacketMatchTypes_IP_PROTOCOL)
				//ruleData["IP_PROTOCOL"] = v.E_OpenconfigPacketMatchTypes_IP_PROTOCOL.ΛMap()["E_OpenconfigPacketMatchTypes_IP_PROTOCOL"][int64(v.E_OpenconfigPacketMatchTypes_IP_PROTOCOL)].Name
				ruleData.Field["IP_PROTOCOL"] = strconv.FormatInt(int64(IP_PROTOCOL_MAP[v.E_OpenconfigPacketMatchTypes_IP_PROTOCOL]), 10)
			case reflect.TypeOf(ocbinds.OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Ipv6_Config_Protocol_Union_Uint8{}):
				v := (rule.Ipv6.Config.Protocol).(*ocbinds.OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Ipv6_Config_Protocol_Union_Uint8)
				ruleData.Field["IP_PROTOCOL"] = strconv.FormatInt(int64(v.Uint8), 10)
			}
		}

		if rule.Ipv6.Config.Dscp != nil {
			ruleData.Field["DSCP"] = strconv.FormatInt(int64(*rule.Ipv6.Config.Dscp), 10)
		}
		if rule.Ipv6.Config.SourceAddress != nil {
			ruleData.Field["SRC_IPV6"] = *rule.Ipv6.Config.SourceAddress
		}
		if rule.Ipv6.Config.DestinationAddress != nil {
			ruleData.Field["DST_IPV6"] = *rule.Ipv6.Config.DestinationAddress
		}
		if rule.Ipv6.Config.SourceFlowLabel != nil {
			ruleData.Field["SRC_FLOWLABEL"] = strconv.FormatInt(int64(*rule.Ipv6.Config.SourceFlowLabel), 10)
		}
		if rule.Ipv6.Config.DestinationFlowLabel != nil {
			ruleData.Field["DST_FLOWLABEL"] = strconv.FormatInt(int64(*rule.Ipv6.Config.DestinationFlowLabel), 10)
		}
	}
	// openconfig-packet-match: ICMPv6 type match (SONiC ICMPV6_TYPE). The
	// standard model uses an identityref (openconfig-icmpv6-types); map it to
	// the numeric ICMPv6 type. ICMPv6 code mapping is a follow-up.
	if rule.Ipv6.Icmpv6 != nil && rule.Ipv6.Icmpv6.Config != nil {
		if rule.Ipv6.Icmpv6.Config.Type != ocbinds.OpenconfigIcmpv6Types_TYPE_UNSET {
			if num, ok := ICMPV6_TYPE_MAP[rule.Ipv6.Icmpv6.Config.Type]; ok {
				ruleData.Field["ICMPV6_TYPE"] = strconv.FormatInt(int64(num), 10)
			}
		}
	}
	ruleData.Field["IP_TYPE"] = "IPV6ANY"
}

func convertOCToInternalTransport(ruleData db.Value, aclName string, ruleIndex uint32, rule *ocbinds.OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry) {
	if rule.Transport == nil {
		return
	}
	if rule.Transport.Config.SourcePort != nil && util.IsTypeStructPtr(reflect.TypeOf(rule.Transport.Config.SourcePort)) {
		sourceportType := reflect.TypeOf(rule.Transport.Config.SourcePort).Elem()
		switch sourceportType {
		case reflect.TypeOf(ocbinds.OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Transport_Config_SourcePort_Union_E_OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Transport_Config_SourcePort{}):
			v := (rule.Transport.Config.SourcePort).(*ocbinds.OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Transport_Config_SourcePort_Union_E_OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Transport_Config_SourcePort)
			ruleData.Field["L4_SRC_PORT"] = v.E_OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Transport_Config_SourcePort.ΛMap()["E_OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Transport_Config_SourcePort"][int64(v.E_OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Transport_Config_SourcePort)].Name
		case reflect.TypeOf(ocbinds.OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Transport_Config_SourcePort_Union_String{}):
			v := (rule.Transport.Config.SourcePort).(*ocbinds.OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Transport_Config_SourcePort_Union_String)
			ruleData.Field["L4_SRC_PORT_RANGE"] = strings.Replace(v.String, "..", "-", 1)
		case reflect.TypeOf(ocbinds.OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Transport_Config_SourcePort_Union_Uint16{}):
			v := (rule.Transport.Config.SourcePort).(*ocbinds.OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Transport_Config_SourcePort_Union_Uint16)
			ruleData.Field["L4_SRC_PORT"] = strconv.FormatInt(int64(v.Uint16), 10)
		}
	}

	if rule.Transport.Config.DestinationPort != nil && util.IsTypeStructPtr(reflect.TypeOf(rule.Transport.Config.DestinationPort)) {
		destportType := reflect.TypeOf(rule.Transport.Config.DestinationPort).Elem()
		switch destportType {
		case reflect.TypeOf(ocbinds.OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Transport_Config_DestinationPort_Union_E_OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Transport_Config_DestinationPort{}):
			v := (rule.Transport.Config.DestinationPort).(*ocbinds.OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Transport_Config_DestinationPort_Union_E_OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Transport_Config_DestinationPort)
			ruleData.Field["L4_DST_PORT"] = v.E_OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Transport_Config_DestinationPort.ΛMap()["E_OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Transport_Config_DestinationPort"][int64(v.E_OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Transport_Config_DestinationPort)].Name
		case reflect.TypeOf(ocbinds.OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Transport_Config_DestinationPort_Union_String{}):
			v := (rule.Transport.Config.DestinationPort).(*ocbinds.OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Transport_Config_DestinationPort_Union_String)
			ruleData.Field["L4_DST_PORT_RANGE"] = strings.Replace(v.String, "..", "-", 1)
		case reflect.TypeOf(ocbinds.OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Transport_Config_DestinationPort_Union_Uint16{}):
			v := (rule.Transport.Config.DestinationPort).(*ocbinds.OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Transport_Config_DestinationPort_Union_Uint16)
			ruleData.Field["L4_DST_PORT"] = strconv.FormatInt(int64(v.Uint16), 10)
		}
	}

	var tcpFlags uint32 = 0x00
	if len(rule.Transport.Config.TcpFlags) > 0 {
		for _, flag := range rule.Transport.Config.TcpFlags {
			switch flag {
			case ocbinds.OpenconfigPacketMatchTypes_TCP_FLAGS_TCP_FIN:
				tcpFlags |= 0x01
			case ocbinds.OpenconfigPacketMatchTypes_TCP_FLAGS_TCP_SYN:
				tcpFlags |= 0x02
			case ocbinds.OpenconfigPacketMatchTypes_TCP_FLAGS_TCP_RST:
				tcpFlags |= 0x04
			case ocbinds.OpenconfigPacketMatchTypes_TCP_FLAGS_TCP_PSH:
				tcpFlags |= 0x08
			case ocbinds.OpenconfigPacketMatchTypes_TCP_FLAGS_TCP_ACK:
				tcpFlags |= 0x10
			case ocbinds.OpenconfigPacketMatchTypes_TCP_FLAGS_TCP_URG:
				tcpFlags |= 0x20
			case ocbinds.OpenconfigPacketMatchTypes_TCP_FLAGS_TCP_ECE:
				tcpFlags |= 0x40
			case ocbinds.OpenconfigPacketMatchTypes_TCP_FLAGS_TCP_CWR:
				tcpFlags |= 0x80
			}
		}
		var b bytes.Buffer
		fmt.Fprintf(&b, "0x%0.2x/0x%0.2x", tcpFlags, tcpFlags)
		ruleData.Field["TCP_FLAGS"] = b.String()
	}
}

func convertOCToInternalInputInterface(ruleData db.Value, aclName string, ruleIndex uint32, rule *ocbinds.OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry) {
	if rule.InputInterface != nil && rule.InputInterface.InterfaceRef != nil {
		ruleData.Field["IN_PORTS"] = *rule.InputInterface.InterfaceRef.Config.Interface
	}
}

func convertOCToInternalInputAction(ruleData db.Value, aclName string, ruleIndex uint32, rule *ocbinds.OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry) {
	if rule.Actions != nil && rule.Actions.Config != nil {
		switch rule.Actions.Config.ForwardingAction {
		case ocbinds.OpenconfigAcl_FORWARDING_ACTION_ACCEPT:
			ruleData.Field["PACKET_ACTION"] = "FORWARD"
		case ocbinds.OpenconfigAcl_FORWARDING_ACTION_DROP, ocbinds.OpenconfigAcl_FORWARDING_ACTION_REJECT:
			ruleData.Field["PACKET_ACTION"] = "DROP"
		default:
		}
	}
}

func (app *AclApp) handleRuleFieldsDeletion(d *db.DB, aclKey string, ruleKey string) error {
	var err error

	ruleEntry, err := d.GetEntry(app.ruleTs, asKey(aclKey, ruleKey))
	if err != nil {
		return err
	}
	nodeInfo, err := getTargetNodeYangSchema(app.pathInfo.Path, (*app.ygotRoot).(*ocbinds.Device))
	if err != nil {
		return err
	}
	if nodeInfo.IsLeaf() {
		switch nodeInfo.Name {
		case "description":
			(&ruleEntry).Remove("RULE_DESCRIPTION")
		// L2
		case "ethertype":
			(&ruleEntry).Remove("ETHER_TYPE")
			// IPv4/IPv6
		case "source-address":
			if strings.Contains(app.pathInfo.Path, "ipv4/config") {
				(&ruleEntry).Remove("SRC_IP")
			} else if strings.Contains(app.pathInfo.Path, "ipv6/config") {
				(&ruleEntry).Remove("SRC_IPV6")
			}
		case "destination-address":
			if strings.Contains(app.pathInfo.Path, "ipv4/config") {
				(&ruleEntry).Remove("DST_IP")
			} else if strings.Contains(app.pathInfo.Path, "ipv6/config") {
				(&ruleEntry).Remove("DST_IPV6")
			}
		case "dscp":
			(&ruleEntry).Remove("DSCP")
		case "protocol":
			(&ruleEntry).Remove("IP_PROTOCOL")
			// ICMP type/code. The leaf is named the same under icmpv4 and
			// icmpv6, so the path decides which SONiC field it maps to.
		case "type":
			if strings.Contains(app.pathInfo.Path, "icmpv4/config") {
				(&ruleEntry).Remove("ICMP_TYPE")
			} else if strings.Contains(app.pathInfo.Path, "icmpv6/config") {
				(&ruleEntry).Remove("ICMPV6_TYPE")
			}
		case "code":
			if strings.Contains(app.pathInfo.Path, "icmpv4/config") {
				(&ruleEntry).Remove("ICMP_CODE")
			} else if strings.Contains(app.pathInfo.Path, "icmpv6/config") {
				(&ruleEntry).Remove("ICMPV6_CODE")
			}
			// transport
		case "source-port":
			(&ruleEntry).Remove("L4_SRC_PORT")
			(&ruleEntry).Remove("L4_SRC_PORT_RANGE")
		case "destination-port":
			(&ruleEntry).Remove("L4_DST_PORT")
			(&ruleEntry).Remove("L4_DST_PORT_RANGE")
			// actions
		case "forwarding-action":
			(&ruleEntry).Remove("PACKET_ACTION")
			//input-interface
		case "interface":
			(&ruleEntry).Remove("IN_PORTS")
			//case "subinterface":
		}
	} else if nodeInfo.IsContainer() {
		targetType := reflect.TypeOf(*app.ygotTarget)
		switch targetType.Elem().Name() {
		case "OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_L2", "OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_L2_Config":
			(&ruleEntry).Remove("ETHER_TYPE")
			// The icmpv4 match fields live under the sibling ipv4/icmpv4
			// container, so they must only be cleared when the whole ipv4
			// container is deleted, not when only ipv4/config is deleted. On a
			// DELETE the config container is empty, so ygot collapses the
			// target to the parent Ipv4 struct for both paths; the request
			// path is the reliable discriminator.
		case "OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Ipv4", "OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Ipv4_Config":
			(&ruleEntry).Remove("IP_PROTOCOL")
			(&ruleEntry).Remove("SRC_IP")
			(&ruleEntry).Remove("DST_IP")
			(&ruleEntry).Remove("DSCP")
			if !strings.Contains(app.pathInfo.Path, "ipv4/config") {
				(&ruleEntry).Remove("ICMP_TYPE")
				(&ruleEntry).Remove("ICMP_CODE")
			}
		case "OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Ipv4_Icmpv4", "OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Ipv4_Icmpv4_Config":
			(&ruleEntry).Remove("ICMP_TYPE")
			(&ruleEntry).Remove("ICMP_CODE")
			// Same sibling-container reasoning as ipv4 above for icmpv6.
		case "OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Ipv6", "OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Ipv6_Config":
			(&ruleEntry).Remove("IP_PROTOCOL")
			(&ruleEntry).Remove("SRC_IPV6")
			(&ruleEntry).Remove("DST_IPV6")
			(&ruleEntry).Remove("DSCP")
			if !strings.Contains(app.pathInfo.Path, "ipv6/config") {
				(&ruleEntry).Remove("ICMPV6_TYPE")
				(&ruleEntry).Remove("ICMPV6_CODE")
			}
		case "OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Ipv6_Icmpv6", "OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Ipv6_Icmpv6_Config":
			(&ruleEntry).Remove("ICMPV6_TYPE")
			(&ruleEntry).Remove("ICMPV6_CODE")
		case "OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Transport", "OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Transport_Config":
			(&ruleEntry).Remove("L4_SRC_PORT")
			(&ruleEntry).Remove("L4_SRC_PORT_RANGE")
			(&ruleEntry).Remove("L4_DST_PORT")
			(&ruleEntry).Remove("L4_DST_PORT_RANGE")
			(&ruleEntry).Remove("TCP_FLAGS")
		case "OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_InputInterface", "OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_InputInterface_InterfaceRef", "OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_InputInterface_InterfaceRef_Config":
			(&ruleEntry).Remove("IN_PORTS")
		case "OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Actions", "OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Actions_Config":
			(&ruleEntry).Remove("PACKET_ACTION")
		}
	} else if nodeInfo.IsLeafList() {
		switch nodeInfo.Name {
		case "tcp-flags":
			(&ruleEntry).Remove("TCP_FLAGS")
		}
	} else {
		log.Error("This yang type is not handled currently")
	}
	err = d.SetEntry(app.ruleTs, asKey(aclKey, ruleKey), ruleEntry)

	return err
}

func (app *AclApp) setAclDataInConfigDb(d *db.DB, aclData map[string]db.Value, createFlag bool) error {
	var err error
	for key := range aclData {
		existingEntry, err := d.GetEntry(app.aclTs, db.Key{Comp: []string{key}})
		// If Create ACL request comes and ACL already exists, throw error
		if createFlag && existingEntry.IsPopulated() {
			return tlerr.AlreadyExists("Acl %s already exists", key)
		}
		if createFlag || (!createFlag && err != nil && !existingEntry.IsPopulated()) {
			err := d.CreateEntry(app.aclTs, db.Key{Comp: []string{key}}, aclData[key])
			if err != nil {
				return err
			}
		} else {
			if existingEntry.IsPopulated() {
				if existingEntry.Get(ACL_DESCRIPTION) != aclData[key].Field[ACL_DESCRIPTION] {
					err := d.ModEntry(app.aclTs, db.Key{Comp: []string{key}}, aclData[key])
					if err != nil {
						return err
					}
				}
			}
		}
	}
	return err
}

func (app *AclApp) setAclRuleDataInConfigDb(d *db.DB, ruleData map[string]map[string]db.Value, createFlag bool) error {
	var err error
	for aclName := range ruleData {
		for ruleName := range ruleData[aclName] {
			existingRuleEntry, err := d.GetEntry(app.ruleTs, db.Key{Comp: []string{aclName, ruleName}})
			// If Create Rule request comes and Rule already exists, throw error
			if createFlag && existingRuleEntry.IsPopulated() {
				return tlerr.AlreadyExists("Rule %s already exists", ruleName)
			}
			if createFlag || (!createFlag && err != nil && !existingRuleEntry.IsPopulated()) {
				err := d.CreateEntry(app.ruleTs, db.Key{Comp: []string{aclName, ruleName}}, ruleData[aclName][ruleName])
				if err != nil {
					return err
				}
			} else {
				if existingRuleEntry.IsPopulated() && ruleName != "DEFAULT_RULE" {
					err := d.ModEntry(app.ruleTs, db.Key{Comp: []string{aclName, ruleName}}, ruleData[aclName][ruleName])
					if err != nil {
						return err
					}
				}
			}
		}
	}
	return err
}

func (app *AclApp) setAclBindDataInConfigDb(d *db.DB, aclData map[string]db.Value, opcode int) error {
	var err error
	for aclKey, aclInfo := range aclData {
		// Get ACL info from DB
		dbAcl, err := d.GetEntry(app.aclTs, db.Key{Comp: []string{aclKey}})
		if err != nil {
			return err
		}
		if REPLACE == opcode {
			dbAcl.SetList("ports", aclInfo.GetList("ports"))
			dbAcl.Set("stage", aclInfo.Get("stage"))
		} else {
			dbAclIntfs := dbAcl.GetList("ports")
			if len(dbAclIntfs) > 0 {
				dbAclDirec := dbAcl.Get("stage")
				newDirec := aclInfo.Get("stage")
				if (UPDATE == opcode) && (len(dbAclDirec) > 0) && (len(newDirec) > 0) && (dbAclDirec != newDirec) {
					return tlerr.InvalidArgs("Acl direction of %s not allowed when it is already configured as %s", newDirec, dbAclDirec)
				}
				// Merge interfaces from DB to list in aclInfo and set back in DB
				intfs := aclInfo.GetList("ports")
				for _, ifId := range dbAclIntfs {
					if !contains(intfs, ifId) {
						intfs = append(intfs, ifId)
					}
				}
				dbAcl.SetList("ports", intfs)
			} else {
				dbAcl.SetList("ports", aclInfo.GetList("ports"))
			}

			if len(dbAcl.Get("stage")) == 0 {
				dbAcl.Set("stage", aclInfo.Get("stage"))
			}
		}
		err = d.SetEntry(app.aclTs, db.Key{Comp: []string{aclKey}}, dbAcl)
		//err = d.ModEntry(app.aclTs, db.Key{Comp: []string{aclKey}}, dbAcl)
		if err != nil {
			return err
		}
	}
	return err
}

func getIpProtocol(proto int64) interface{} {
	for k, v := range IP_PROTOCOL_MAP {
		if uint8(proto) == v {
			return k
		}
	}
	return uint8(proto)
}

// getIcmpv6TypeId reverse-maps a numeric ICMPv6 type to its standard
// openconfig-icmpv6-types identity, or UNSET if unknown.
func getIcmpv6TypeId(num uint8) ocbinds.E_OpenconfigIcmpv6Types_TYPE {
	for k, v := range ICMPV6_TYPE_MAP {
		if num == v {
			return k
		}
	}
	return ocbinds.OpenconfigIcmpv6Types_TYPE_UNSET
}

func getTransportSrcDestPorts(portVal string, portType string) interface{} {
	var portRange string = ""

	portNum, err := strconv.Atoi(portVal)
	if err != nil && strings.Contains(portVal, "-") {
		// CONFIG_DB stores ranges as "<low>-<high>", but the OpenConfig
		// port-num-range type requires "<low>..<high>".
		portRange = strings.Replace(portVal, "-", "..", 1)
	}

	if len(portRange) > 0 {
		return portRange
	} else if portNum > 0 {
		return uint16(portNum)
	} else {
		if "src" == portType {
			return ocbinds.OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Transport_Config_SourcePort_ANY
		} else if "dest" == portType {
			return ocbinds.OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Transport_Config_DestinationPort_ANY
		}
	}
	return nil
}

func getTransportConfigTcpFlags(tcpFlags string) []ocbinds.E_OpenconfigPacketMatchTypes_TCP_FLAGS {
	var flags []ocbinds.E_OpenconfigPacketMatchTypes_TCP_FLAGS
	if len(tcpFlags) > 0 {
		flagStr := strings.Split(tcpFlags, "/")[0]
		flagNumber, _ := strconv.ParseUint(strings.Replace(flagStr, "0x", "", -1), 16, 32)
		for i := 0; i < 8; i++ {
			mask := 1 << uint(i)
			if (int(flagNumber) & mask) > 0 {
				switch int(flagNumber) & mask {
				case 0x01:
					flags = append(flags, ocbinds.OpenconfigPacketMatchTypes_TCP_FLAGS_TCP_FIN)
				case 0x02:
					flags = append(flags, ocbinds.OpenconfigPacketMatchTypes_TCP_FLAGS_TCP_SYN)
				case 0x04:
					flags = append(flags, ocbinds.OpenconfigPacketMatchTypes_TCP_FLAGS_TCP_RST)
				case 0x08:
					flags = append(flags, ocbinds.OpenconfigPacketMatchTypes_TCP_FLAGS_TCP_PSH)
				case 0x10:
					flags = append(flags, ocbinds.OpenconfigPacketMatchTypes_TCP_FLAGS_TCP_ACK)
				case 0x20:
					flags = append(flags, ocbinds.OpenconfigPacketMatchTypes_TCP_FLAGS_TCP_URG)
				case 0x40:
					flags = append(flags, ocbinds.OpenconfigPacketMatchTypes_TCP_FLAGS_TCP_ECE)
				case 0x80:
					flags = append(flags, ocbinds.OpenconfigPacketMatchTypes_TCP_FLAGS_TCP_CWR)
				default:
				}
			}
		}
	}
	return flags
}

func getL2EtherType(etherType uint64) interface{} {
	for k, v := range ETHERTYPE_MAP {
		if uint32(etherType) == v {
			return k
		}
	}
	return uint16(etherType)
}

// ctrlPlaneOCNameType reconstructs the OpenConfig acl-set name and type for a
// control-plane (CTRLPLANE) table whose ACL_TABLE:type no longer encodes the OC
// type. The OC type is preserved in the composite table-name suffix
// (e.g. "MGMT_SSH_ACL_IPV4"); default to ACL_IPV4 if no known suffix is found.
func ctrlPlaneOCNameType(aclKey string) (string, ocbinds.E_OpenconfigAcl_ACL_TYPE) {
	switch {
	case strings.HasSuffix(aclKey, "_"+OPENCONFIG_ACL_TYPE_IPV6):
		return strings.Replace(aclKey, "_"+OPENCONFIG_ACL_TYPE_IPV6, "", 1), ocbinds.OpenconfigAcl_ACL_TYPE_ACL_IPV6
	case strings.HasSuffix(aclKey, "_"+OPENCONFIG_ACL_TYPE_L2):
		return strings.Replace(aclKey, "_"+OPENCONFIG_ACL_TYPE_L2, "", 1), ocbinds.OpenconfigAcl_ACL_TYPE_ACL_L2
	default:
		return strings.Replace(aclKey, "_"+OPENCONFIG_ACL_TYPE_IPV4, "", 1), ocbinds.OpenconfigAcl_ACL_TYPE_ACL_IPV4
	}
}

func getAclKeysFromStrKey(aclKey string, aclType string) (string, ocbinds.E_OpenconfigAcl_ACL_TYPE) {
	var aclOrigName string
	var aclOrigType ocbinds.E_OpenconfigAcl_ACL_TYPE

	if SONIC_ACL_TYPE_IPV4 == aclType {
		aclOrigName = strings.Replace(aclKey, "_"+OPENCONFIG_ACL_TYPE_IPV4, "", 1)
		aclOrigType = ocbinds.OpenconfigAcl_ACL_TYPE_ACL_IPV4
	} else if SONIC_ACL_TYPE_IPV6 == aclType {
		aclOrigName = strings.Replace(aclKey, "_"+OPENCONFIG_ACL_TYPE_IPV6, "", 1)
		aclOrigType = ocbinds.OpenconfigAcl_ACL_TYPE_ACL_IPV6
	} else if SONIC_ACL_TYPE_L2 == aclType {
		aclOrigName = strings.Replace(aclKey, "_"+OPENCONFIG_ACL_TYPE_L2, "", 1)
		aclOrigType = ocbinds.OpenconfigAcl_ACL_TYPE_ACL_L2
	}
	return aclOrigName, aclOrigType
}

// getAclTypeOCEnumFromName returns the ACL_TYPE enum from name
func getAclTypeOCEnumFromName(val string) (ocbinds.E_OpenconfigAcl_ACL_TYPE, error) {
	switch val {
	case "ACL_IPV4", "openconfig-acl:ACL_IPV4":
		return ocbinds.OpenconfigAcl_ACL_TYPE_ACL_IPV4, nil
	case "ACL_IPV6", "openconfig-acl:ACL_IPV6":
		return ocbinds.OpenconfigAcl_ACL_TYPE_ACL_IPV6, nil
	case "ACL_L2", "openconfig-acl:ACL_L2":
		return ocbinds.OpenconfigAcl_ACL_TYPE_ACL_L2, nil
	default:
		return ocbinds.OpenconfigAcl_ACL_TYPE_UNSET,
			tlerr.NotSupported("ACL Type '%s' not supported", val)
	}
}

func convertSonicAclTypeToOC(aclType string) (ocbinds.E_OpenconfigAcl_ACL_TYPE, string) {
	switch aclType {
	case SONIC_ACL_TYPE_IPV4:
		return ocbinds.OpenconfigAcl_ACL_TYPE_ACL_IPV4, "ACL_IPV4"
	case SONIC_ACL_TYPE_IPV6:
		return ocbinds.OpenconfigAcl_ACL_TYPE_ACL_IPV6, "ACL_IPV6"
	case SONIC_ACL_TYPE_L2:
		return ocbinds.OpenconfigAcl_ACL_TYPE_ACL_L2, "ACL_L2"
	default:
		return ocbinds.OpenconfigAcl_ACL_TYPE_UNSET, ""
	}
}

func getAclKeyStrFromOCKey(aclname string, acltype ocbinds.E_OpenconfigAcl_ACL_TYPE) string {
	aclT := acltype.ΛMap()["E_OpenconfigAcl_ACL_TYPE"][int64(acltype)].Name
	return getAclKeyStr(aclname, aclT)
}

// getAclKeyStr builds the ACL_TABLE key for an acl-set name and its OC type name.
// SONiC table names carry neither spaces nor dashes, so both are substituted the
// same way on every path that reaches the db, reads and subscriptions included.
func getAclKeyStr(aclName string, aclType string) string {
	aclN := strings.Replace(strings.Replace(aclName, " ", "_", -1), "-", "_", -1)
	return aclN + "_" + aclType
}

/*
Check if targetUriPath is child (subtree) of nodePath
The return value can be used to decide if subtrees needs
to visited to fill the data or not.
*/
func isSubtreeRequest(targetUriPath string, nodePath string) bool {
	return strings.HasPrefix(targetUriPath, nodePath)
}

func (app *AclApp) translateSubscribe(req translateSubRequest) (translateSubResponse, error) {
	ymap := yangMapTree{
		subtree: map[string]*yangMapTree{
			"acl-sets/acl-set": {
				mapFunc: app.translateSubscribeAclSet,
				subtree: map[string]*yangMapTree{
					"acl-entries/acl-entry": {
						mapFunc: app.translateSubscribeAclEntry,
					},
				},
			},
			"interfaces": {
				mapFunc: app.translateSubscribeAclBindings,
			},
		}}

	nb := notificationInfoBuilder{
		pathInfo: NewPathInfo(req.path),
		yangMap:  ymap,
	}
	return nb.Build()
}

func (app *AclApp) translateSubscribeAclSet(nb *notificationInfoBuilder) error {
	aclName := nb.pathInfo.StringVar("name", "*")
	aclType := nb.pathInfo.StringVar("type", "*")

	nb.New().PathKey("name", aclName).PathKey("type", aclType)
	nb.Table(db.ConfigDB, ACL_TABLE).Key(getAclKeyStr(aclName, aclType))
	if nb.SetFieldPrefix("config") {
		nb.Field("description", ACL_DESCRIPTION)
	}
	if nb.SetFieldPrefix("state") {
		nb.Field("description", ACL_DESCRIPTION)
	}

	return nil
}

func (app *AclApp) translateSubscribeAclEntry(nb *notificationInfoBuilder) error {
	aclName := nb.pathInfo.StringVar("name", "*")
	aclType := nb.pathInfo.StringVar("type", "*")
	ruleSeq := nb.pathInfo.StringVar("sequence-id", "*")

	nb.New().PathKey("sequence-id", ruleSeq)
	nb.Table(db.ConfigDB, RULE_TABLE).Key(getAclKeyStr(aclName, aclType), "RULE_"+ruleSeq)

	ipAclFieldSetter := func(prefix string) {
		if nb.SetFieldPrefix(prefix) {
			nb.Field("source-address", "SRC_IP")
			nb.Field("destination-address", "DST_IP")
			nb.Field("dscp", "DSCP")
			nb.Field("protocol", "IP_PROTOCOL")
		}
	}
	ipv6AclFieldSetter := func(prefix string) {
		if nb.SetFieldPrefix(prefix) {
			nb.Field("source-address", "SRC_IPV6")
			nb.Field("destination-address", "DST_IPV6")
			nb.Field("dscp", "DSCP")
			nb.Field("protocol", "IP_PROTOCOL")
		}
	}
	ipTransportFieldSetter := func(prefix string) {
		if nb.SetFieldPrefix(prefix) {
			nb.Field("source-port", "L4_SRC_PORT")
			nb.Field("source-port", "L4_SRC_PORT_RANGE")
			nb.Field("destination-port", "L4_DST_PORT")
			nb.Field("destination-port", "L4_DST_PORT_RANGE")
			nb.Field("tcp-flags", "TCP_FLAGS")
		}
	}
	// The icmpv4/icmpv6 match leaves live under their own container (not
	// ipv4/config), so they need their own prefix registration for ON_CHANGE
	// subscriptions to fire when ICMP_TYPE/ICMP_CODE change in CONFIG_DB.
	icmpv4FieldSetter := func(prefix string) {
		if nb.SetFieldPrefix(prefix) {
			nb.Field("type", "ICMP_TYPE")
			nb.Field("code", "ICMP_CODE")
		}
	}
	icmpv6FieldSetter := func(prefix string) {
		if nb.SetFieldPrefix(prefix) {
			nb.Field("type", "ICMPV6_TYPE")
			nb.Field("code", "ICMPV6_CODE")
		}
	}
	if wildcardMatch(aclType, "ACL_IPV4") {
		ipAclFieldSetter("ipv4/config")
		ipAclFieldSetter("ipv4/state")
		icmpv4FieldSetter("ipv4/icmpv4/config")
		icmpv4FieldSetter("ipv4/icmpv4/state")
		ipTransportFieldSetter("transport/config")
		ipTransportFieldSetter("transport/state")
	}
	if wildcardMatch(aclType, "ACL_IPV6") {
		ipv6AclFieldSetter("ipv6/config")
		ipv6AclFieldSetter("ipv6/state")
		icmpv6FieldSetter("ipv6/icmpv6/config")
		icmpv6FieldSetter("ipv6/icmpv6/state")
		ipTransportFieldSetter("transport/config")
		ipTransportFieldSetter("transport/state")
	}
	if nb.SetFieldPrefix("actions/config") {
		nb.Field("forwarding-action", "PACKET_ACTION")
	}
	if nb.SetFieldPrefix("actions/state") {
		nb.Field("forwarding-action", "PACKET_ACTION")
	}

	return nil
}

func (app *AclApp) translateSubscribeAclBindings(nb *notificationInfoBuilder) error {
	nb.New().OnChange(false).Preferred(Sample)
	return nil
}

func (app *AclApp) processSubscribe(req processSubRequest) (processSubResponse, error) {
	resp := processSubResponse{
		path: req.path,
	}

	switch req.table.Name {
	case ACL_TABLE:
		if path.GetElemAt(resp.path, 2) != "acl-set" {
			return resp, tlerr.New("Unknown path template: %v", path.String(req.path))
		}

		_, aclTypeStr := convertSonicAclTypeToOC(req.entry.Get(ACL_TYPE))
		aclName := strings.TrimSuffix(req.key.Get(0), "_"+aclTypeStr)
		resolveKeyAt(resp.path, 2, "name", aclName)
		resolveKeyAt(resp.path, 2, "type", aclTypeStr)

	case RULE_TABLE:
		ruleName := req.key.Get(1)
		if !strings.HasPrefix(ruleName, "RULE_") {
			// The default-deny rule translib maintains itself has no
			// sequence-id, and the read path leaves it out of acl-entries too.
			return resp, tlerr.New("Not an acl-entry: %s", ruleName)
		}

		aclName := req.key.Get(0)
		aclEntry, err := req.dbs[db.ConfigDB].GetEntry(&db.TableSpec{Name: ACL_TABLE}, asKey(aclName))
		if err != nil {
			return resp, err
		}

		_, aclTypeStr := convertSonicAclTypeToOC(aclEntry.Get(ACL_TYPE))
		aclName = strings.TrimSuffix(aclName, "_"+aclTypeStr)
		resolveKeyAt(resp.path, 2, "name", aclName)
		resolveKeyAt(resp.path, 2, "type", aclTypeStr)
		resolveKeyAt(resp.path, 4, "sequence-id", strings.TrimPrefix(ruleName, "RULE_"))

	default:
		return resp, tlerr.New("Unknown table: %s", req.table.Name)
	}

	return resp, nil
}

// resolveKeyAt fills a key the subscribe path template left as a wildcard, and
// leaves a key the client spelled out alone.
//
// The value derived from the DB key is only an approximation of what was asked
// for: a dash is not allowed in a SONiC table name, so an acl-set named
// "ACL-12-IPV4" is stored as "ACL_12_IPV4" and reads back with underscores.
// Overwriting the requested name with that form makes translib's check that the
// resolved path still matches the requested template fail, and the notification
// is dropped - which is how a subscription carrying a wildcard rule key ended up
// reporting nothing at all for such an acl-set.
func resolveKeyAt(p *gnmi.Path, index int, name, value string) {
	if index < path.Len(p) {
		if cur, ok := p.Elem[index].Key[name]; ok && cur != "*" {
			return
		}
	}
	path.SetKeyAt(p, index, name, value)
}
