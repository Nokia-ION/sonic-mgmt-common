////////////////////////////////////////////////////////////////////////////////
//                                                                            //
//  Copyright 2026 Nokia, Inc.                                                //
//                                                                            //
//  Licensed under the Apache License, Version 2.0 (the "License");          //
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

package transformer

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	"github.com/Azure/sonic-mgmt-common/translib/ocbinds"
	"github.com/Azure/sonic-mgmt-common/translib/tlerr"
	"github.com/openconfig/ygot/ygot"
)

const (
	niBgpPeerGroupTbl        = "BGP_PEER_GROUP"
	niBgpPgTargetCfg         = "peer-group-config"
	niBgpPgPeerGroupNameLeaf = "peer-group-name"
)

var niBgpPgCfgReqUriRe = regexp.MustCompile(`^/openconfig-network-instance:network-instances/network-instance\[name=([^\]]+)\]/protocols/protocol\[identifier=BGP\]\[name=bgp\]/bgp/peer-groups/peer-group\[peer-group-name=([^\]]+)\]/config(?:/(peer-group-name))?$`)

type niBgpPgReqInfo struct {
	niName        string
	peerGroupName string
	targetLeaf    string // "", "peer-group-name"
	targetPath    string
}

func init() {
	XlateFuncBind("YangToDb_ni_bgp_pg_cfg_xfmr", YangToDb_ni_bgp_pg_cfg_xfmr)
	XlateFuncBind("DbToYang_ni_bgp_pg_cfg_xfmr", DbToYang_ni_bgp_pg_cfg_xfmr)
}

func parseNiBgpPgRequestUri(uri string) (*niBgpPgReqInfo, error) {
	m := niBgpPgCfgReqUriRe.FindStringSubmatch(uri)
	if len(m) >= 3 {
		leaf := ""
		if len(m) >= 4 {
			leaf = m[3]
		}
		return &niBgpPgReqInfo{
			niName:        m[1],
			peerGroupName: m[2],
			targetLeaf:    leaf,
			targetPath:    niBgpPgTargetCfg,
		}, nil
	}
	return nil, tlerr.InvalidArgsError{
		Format: "unsupported path for network-instance BGP peer-group config mapping: " + uri,
	}
}

func getNiBgpPeerGroupCfgObj(s *ygot.GoStruct, niName, peerGroup string, createIfMissing bool) *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_PeerGroups_PeerGroup_Config {
	if s == nil || *s == nil {
		return nil
	}

	deviceObj, ok := (*s).(*ocbinds.Device)
	if !ok || deviceObj == nil {
		return nil
	}
	ygot.BuildEmptyTree(deviceObj)

	if deviceObj.NetworkInstances == nil {
		if !createIfMissing {
			return nil
		}
		deviceObj.NetworkInstances = &ocbinds.OpenconfigNetworkInstance_NetworkInstances{}
	}
	niObj, ok := deviceObj.NetworkInstances.NetworkInstance[niName]
	if !ok || niObj == nil {
		if !createIfMissing {
			return nil
		}
		var err error
		niObj, err = deviceObj.NetworkInstances.NewNetworkInstance(niName)
		if err != nil || niObj == nil {
			return nil
		}
	}

	if niObj.Protocols == nil {
		if !createIfMissing {
			return nil
		}
		niObj.Protocols = &ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols{}
	}
	if niObj.Protocols.Protocol == nil {
		if !createIfMissing {
			return nil
		}
		niObj.Protocols.Protocol = make(map[ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Key]*ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol)
	}

	pkey := ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Key{
		Identifier: ocbinds.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_BGP,
		Name:       "bgp",
	}
	pObj, ok := niObj.Protocols.Protocol[pkey]
	if !ok || pObj == nil {
		if !createIfMissing {
			return nil
		}
		var err error
		pObj, err = niObj.Protocols.NewProtocol(ocbinds.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_BGP, "bgp")
		if err != nil || pObj == nil {
			return nil
		}
	}
	ygot.BuildEmptyTree(pObj)

	if pgObj, ok := pObj.Bgp.PeerGroups.PeerGroup[peerGroup]; ok && pgObj != nil {
		ygot.BuildEmptyTree(pgObj)
		return pgObj.Config
	}
	if !createIfMissing {
		return nil
	}
	pgObj, err := pObj.Bgp.PeerGroups.NewPeerGroup(peerGroup)
	if err != nil || pgObj == nil {
		return nil
	}
	ygot.BuildEmptyTree(pgObj)
	return pgObj.Config
}

func makeNiBgpPgResMap(niName, pgName string, fields map[string]string) map[string]map[string]db.Value {
	return map[string]map[string]db.Value{
		niBgpPeerGroupTbl: {
			niName + "|" + pgName: db.Value{Field: fields},
		},
	}
}

func parseNiBgpPgNameFromParam(param interface{}) (string, bool) {
	switch v := param.(type) {
	case string:
		s := strings.TrimSpace(v)
		return s, s != ""
	case *string:
		if v != nil {
			s := strings.TrimSpace(*v)
			return s, s != ""
		}
	}
	return "", false
}

var YangToDb_ni_bgp_pg_cfg_xfmr SubTreeXfmrYangToDb = func(inParams XfmrParams) (map[string]map[string]db.Value, error) {
	res := make(map[string]map[string]db.Value)
	reqInfo, err := parseNiBgpPgRequestUri(inParams.requestUri)
	if err != nil {
		return res, err
	}

	if inParams.oper == DELETE {
		if reqInfo.targetLeaf == niBgpPgPeerGroupNameLeaf {
			// Key leaf delete implies peer-group node deletion.
			return makeNiBgpPgResMap(reqInfo.niName, reqInfo.peerGroupName, nil), nil
		}
		return res, tlerr.NotSupportedError{Format: "DELETE supported only for /peer-groups/peer-group/config/peer-group-name", Path: inParams.requestUri}
	}

	switch reqInfo.targetLeaf {
	case "", niBgpPgPeerGroupNameLeaf:
		if pg, ok := parseNiBgpPgNameFromParam(inParams.param); ok && pg != reqInfo.peerGroupName {
			return res, tlerr.InvalidArgsError{Format: fmt.Sprintf("peer-group-name payload (%s) does not match key (%s)", pg, reqInfo.peerGroupName)}
		}
		// Ensure the key entry exists; no field write needed for key leaf.
		return makeNiBgpPgResMap(reqInfo.niName, reqInfo.peerGroupName, map[string]string{}), nil
	default:
		return res, tlerr.NotSupportedError{Format: "unsupported leaf under peer-group config", Path: inParams.requestUri}
	}
}

var DbToYang_ni_bgp_pg_cfg_xfmr SubTreeXfmrDbToYang = func(inParams XfmrParams) error {
	reqInfo, err := parseNiBgpPgRequestUri(inParams.requestUri)
	if err != nil {
		return err
	}
	cfgDb := inParams.dbs[db.ConfigDB]
	if cfgDb == nil {
		return tlerr.InvalidArgsError{Format: "ConfigDB nil for network-instance BGP peer-group config"}
	}

	_, err = cfgDb.GetEntry(&db.TableSpec{Name: niBgpPeerGroupTbl}, db.Key{Comp: []string{reqInfo.niName, reqInfo.peerGroupName}})
	if err != nil {
		return err
	}
	cfgObj := getNiBgpPeerGroupCfgObj(inParams.ygRoot, reqInfo.niName, reqInfo.peerGroupName, true)
	if cfgObj == nil {
		return tlerr.InvalidArgsError{Format: "Unable to locate BGP peer-group config object in YANG tree"}
	}
	if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == niBgpPgPeerGroupNameLeaf {
		pg := reqInfo.peerGroupName
		cfgObj.PeerGroupName = &pg
	}
	return nil
}
