//////////////////////////////////////////////////////////////////////////
//
// Copyright 2024 Dell, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
//////////////////////////////////////////////////////////////////////////

package transformer

import (
	"errors"
	"net"
	"strconv"
	"strings"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	"github.com/Azure/sonic-mgmt-common/translib/ocbinds"
	"github.com/Azure/sonic-mgmt-common/translib/tlerr"
	log "github.com/golang/glog"
	"github.com/openconfig/ygot/ygot"
)

const (
	NEIGH_TN           = "NEIGH"
	NEIGH_MAC_FIELD    = "neigh"
	NEIGH_FAMILY_FIELD = "family"
	NEIGH_FAMILY_IPV4  = "IPv4"
	NEIGH_FAMILY_IPV6  = "IPv6"
)

type ipv4NeighborConfig = ocbinds.OpenconfigInterfaces_Interfaces_Interface_Subinterfaces_Subinterface_Ipv4_Neighbors_Neighbor_Config
type ipv4NeighborState = ocbinds.OpenconfigInterfaces_Interfaces_Interface_Subinterfaces_Subinterface_Ipv4_Neighbors_Neighbor_State
type ipv4Neighbor = ocbinds.OpenconfigInterfaces_Interfaces_Interface_Subinterfaces_Subinterface_Ipv4_Neighbors_Neighbor
type ipv6NeighborConfig = ocbinds.OpenconfigInterfaces_Interfaces_Interface_Subinterfaces_Subinterface_Ipv6_Neighbors_Neighbor_Config
type ipv6NeighborState = ocbinds.OpenconfigInterfaces_Interfaces_Interface_Subinterfaces_Subinterface_Ipv6_Neighbors_Neighbor_State
type ipv6Neighbor = ocbinds.OpenconfigInterfaces_Interfaces_Interface_Subinterfaces_Subinterface_Ipv6_Neighbors_Neighbor

func validateSubintfIndexZero(idx string) error {
	i64, err := strconv.ParseUint(idx, 10, 32)
	if err != nil {
		return tlerr.InvalidArgsError{Format: "Invalid subinterface index"}
	}
	if uint32(i64) != 0 {
		return tlerr.NotSupported("Only subinterface index 0 is supported for interface neighbors")
	}
	return nil
}

func normalizeNeighMac(mac string) (string, error) {
	mac = strings.TrimSpace(mac)
	if mac == "" {
		return "", errors.New("link-layer-address is empty")
	}
	hw, err := net.ParseMAC(mac)
	if err != nil {
		return "", tlerr.InvalidArgsError{Format: "Invalid link-layer-address: " + mac}
	}
	return hw.String(), nil
}

func neighIPsEqual(left, right string) bool {
	leftNorm, errLeft := getNormalizedIpStr(left)
	rightNorm, errRight := getNormalizedIpStr(right)
	if errLeft != nil || errRight != nil {
		return left == right
	}
	return leftNorm == rightNorm
}

func isStaticNeighEntry(entry *db.Value, family string) bool {
	if entry == nil || !entry.IsPopulated() {
		return false
	}
	mac, ok := entry.Field[NEIGH_MAC_FIELD]
	if !ok || mac == "" {
		return false
	}
	entryFamily, ok := entry.Field[NEIGH_FAMILY_FIELD]
	if ok && entryFamily != "" && !strings.EqualFold(entryFamily, family) {
		return false
	}
	return true
}

func addNeighToMap(neighMap map[string]map[string]db.Value, ifName, neighIP, mac, family string) {
	if _, ok := neighMap[NEIGH_TN]; !ok {
		neighMap[NEIGH_TN] = make(map[string]db.Value)
	}
	neighMap[NEIGH_TN][ifName+PIPE+neighIP] = db.Value{Field: map[string]string{
		NEIGH_MAC_FIELD:    mac,
		NEIGH_FAMILY_FIELD: family,
	}}
}

func populateIpv4NeighborConfig(neighObj *ipv4Neighbor, neighIP, mac string) {
	if neighObj.Config == nil {
		neighObj.Config = &ipv4NeighborConfig{}
	}
	neighObj.Config.Ip = &neighIP
	neighObj.Config.LinkLayerAddress = &mac
	if neighObj.Ip == nil {
		neighObj.Ip = &neighIP
	}
}

func populateIpv6NeighborConfig(neighObj *ipv6Neighbor, neighIP, mac string) {
	if neighObj.Config == nil {
		neighObj.Config = &ipv6NeighborConfig{}
	}
	neighObj.Config.Ip = &neighIP
	neighObj.Config.LinkLayerAddress = &mac
	if neighObj.Ip == nil {
		neighObj.Ip = &neighIP
	}
}

func populateIpv4NeighborState(neighObj *ipv4Neighbor, neighIP, mac string) {
	if neighObj.State == nil {
		neighObj.State = &ipv4NeighborState{}
	}
	neighObj.State.Ip = &neighIP
	neighObj.State.LinkLayerAddress = &mac
	neighObj.State.Origin = ocbinds.OpenconfigIfIp_NeighborOrigin_STATIC
}

func populateIpv6NeighborState(neighObj *ipv6Neighbor, neighIP, mac string) {
	if neighObj.State == nil {
		neighObj.State = &ipv6NeighborState{}
	}
	neighObj.State.Ip = &neighIP
	neighObj.State.LinkLayerAddress = &mac
	neighObj.State.Origin = ocbinds.OpenconfigIfIp_NeighborOrigin_STATIC
}

func isNeighborLeafGetPath(targetUriPath string) bool {
	return strings.HasSuffix(targetUriPath, "/ip") ||
		strings.HasSuffix(targetUriPath, "/link-layer-address") ||
		strings.HasSuffix(targetUriPath, "/origin")
}

func getOrCreateIpv4Neighbor(neighbors *ocbinds.OpenconfigInterfaces_Interfaces_Interface_Subinterfaces_Subinterface_Ipv4_Neighbors, ipStr string) (*ipv4Neighbor, error) {
	if neighbors == nil {
		return nil, errors.New("ipv4 neighbors container is nil")
	}
	neighObj, ok := neighbors.Neighbor[ipStr]
	if !ok {
		var err error
		neighObj, err = neighbors.NewNeighbor(ipStr)
		if err != nil {
			return nil, err
		}
	}
	return neighObj, nil
}

func getOrCreateIpv6Neighbor(neighbors *ocbinds.OpenconfigInterfaces_Interfaces_Interface_Subinterfaces_Subinterface_Ipv6_Neighbors, ipStr string) (*ipv6Neighbor, error) {
	if neighbors == nil {
		return nil, errors.New("ipv6 neighbors container is nil")
	}
	neighObj, ok := neighbors.Neighbor[ipStr]
	if !ok {
		var err error
		neighObj, err = neighbors.NewNeighbor(ipStr)
		if err != nil {
			return nil, err
		}
	}
	return neighObj, nil
}

var YangToDb_subintf_ipv4_neigh_key_xfmr KeyXfmrYangToDb = func(inParams XfmrParams) (string, error) {
	pathInfo := NewPathInfo(inParams.uri)
	neighIP := pathInfo.Var("ip")
	log.Infof("YangToDb_subintf_ipv4_neigh_key_xfmr: neighbor IP %s", neighIP)
	return neighIP, nil
}

var DbToYang_subintf_ipv4_neigh_key_xfmr KeyXfmrDbToYang = func(inParams XfmrParams) (map[string]interface{}, error) {
	return make(map[string]interface{}), nil
}

var YangToDb_subintf_ipv6_neigh_key_xfmr KeyXfmrYangToDb = YangToDb_subintf_ipv4_neigh_key_xfmr
var DbToYang_subintf_ipv6_neigh_key_xfmr KeyXfmrDbToYang = DbToYang_subintf_ipv4_neigh_key_xfmr

var YangToDb_intf_ipv4_neigh_xfmr SubTreeXfmrYangToDb = func(inParams XfmrParams) (map[string]map[string]db.Value, error) {
	return yangToDbIntfNeighXfmr(inParams, false)
}

var YangToDb_intf_ipv6_neigh_xfmr SubTreeXfmrYangToDb = func(inParams XfmrParams) (map[string]map[string]db.Value, error) {
	return yangToDbIntfNeighXfmr(inParams, true)
}

var DbToYang_intf_ipv4_neigh_xfmr SubTreeXfmrDbToYang = func(inParams XfmrParams) error {
	return dbToYangIntfNeighXfmr(inParams, false)
}

var DbToYang_intf_ipv6_neigh_xfmr SubTreeXfmrDbToYang = func(inParams XfmrParams) error {
	return dbToYangIntfNeighXfmr(inParams, true)
}

func yangToDbIntfNeighXfmr(inParams XfmrParams, ipv6 bool) (map[string]map[string]db.Value, error) {
	neighMap := make(map[string]map[string]db.Value)
	family := NEIGH_FAMILY_IPV4
	ipVersion := "IPv4"
	if ipv6 {
		family = NEIGH_FAMILY_IPV6
		ipVersion = "IPv6"
	}

	pathInfo := NewPathInfo(inParams.uri)
	ifName := pathInfo.Var("name")
	idx := pathInfo.Var("index")
	uriNeighIP := pathInfo.Var("ip")

	if err := validateSubintfIndexZero(idx); err != nil {
		return neighMap, err
	}

	intfType, _, ierr := getIntfTypeByName(ifName)
	if intfType == IntfTypeUnset || ierr != nil {
		return neighMap, errors.New("Invalid interface type")
	}
	if intfType != IntfTypeEthernet && intfType != IntfTypePortChannel {
		if inParams.oper == DELETE && !isNeighborPath(inParams.requestUri) {
			return neighMap, nil
		}
		return neighMap, tlerr.NotSupported(ipVersion + " neighbors are only supported on Ethernet and PortChannel interfaces")
	}

	if inParams.oper == DELETE {
		var intfObj *ocbinds.OpenconfigInterfaces_Interfaces_Interface
		if intfsObj := getIntfsRoot(inParams.ygRoot); intfsObj != nil && intfsObj.Interface != nil {
			intfObj, _ = intfsObj.Interface[ifName]
		}
		return intfNeighDel(inParams.d, ifName, uriNeighIP, intfObj, ipv6)
	}

	intfsObj := getIntfsRoot(inParams.ygRoot)
	if intfsObj == nil || intfsObj.Interface == nil {
		return neighMap, errors.New("Interface not specified")
	}
	intfObj, ok := intfsObj.Interface[ifName]
	if !ok || intfObj == nil {
		return neighMap, errors.New("Interface entry not found in Ygot tree")
	}

	if inParams.invokeCRUSubtreeOnce != nil {
		*inParams.invokeCRUSubtreeOnce = true
	}

	if intfObj.Subinterfaces == nil || intfObj.Subinterfaces.Subinterface == nil {
		return neighMap, tlerr.InvalidArgsError{Format: "Subinterface not specified"}
	}
	subIntfObj, ok := intfObj.Subinterfaces.Subinterface[0]
	if !ok || subIntfObj == nil {
		return neighMap, tlerr.InvalidArgsError{Format: "Subinterface index 0 doesn't exist"}
	}

	if ipv6 {
		if subIntfObj.Ipv6 == nil {
			ygot.BuildEmptyTree(subIntfObj.Ipv6)
		}
		if subIntfObj.Ipv6.Neighbors == nil {
			subIntfObj.Ipv6.Neighbors = &ocbinds.OpenconfigInterfaces_Interfaces_Interface_Subinterfaces_Subinterface_Ipv6_Neighbors{}
		}
		return processIpv6Neighbors(neighMap, subIntfObj, ifName, uriNeighIP, family, ipVersion, inParams)
	}

	if subIntfObj.Ipv4 == nil {
		ygot.BuildEmptyTree(subIntfObj.Ipv4)
	}
	if subIntfObj.Ipv4.Neighbors == nil {
		subIntfObj.Ipv4.Neighbors = &ocbinds.OpenconfigInterfaces_Interfaces_Interface_Subinterfaces_Subinterface_Ipv4_Neighbors{}
	}
	return processIpv4Neighbors(neighMap, subIntfObj, ifName, uriNeighIP, family, ipVersion, inParams)
}

func processIpv4Neighbors(neighMap map[string]map[string]db.Value, subIntfObj *ocbinds.OpenconfigInterfaces_Interfaces_Interface_Subinterfaces_Subinterface,
	ifName, uriNeighIP, family, ipVersion string, inParams XfmrParams) (map[string]map[string]db.Value, error) {

	neighbors := subIntfObj.Ipv4.Neighbors
	if neighbors.Neighbor == nil || len(neighbors.Neighbor) == 0 {
		if uriNeighIP == "" {
			return neighMap, tlerr.InvalidArgsError{Format: ipVersion + " neighbor list is empty"}
		}
		if macStr := neighMacFromParam(inParams); macStr != "" {
			ipStr, err := getNormalizedIpStr(uriNeighIP)
			if err != nil {
				return neighMap, tlerr.InvalidArgsError{Format: err.Error()}
			}
			mac, err := normalizeNeighMac(macStr)
			if err != nil {
				return neighMap, err
			}
			addNeighToMap(neighMap, ifName, ipStr, mac, family)
			return neighMap, nil
		}
		neighObj, err := neighbors.NewNeighbor(uriNeighIP)
		if err != nil {
			return neighMap, err
		}
		neighbors.Neighbor = map[string]*ipv4Neighbor{uriNeighIP: neighObj}
	}

	for neighIP, neighObj := range neighbors.Neighbor {
		if uriNeighIP != "" && !neighIPsEqual(neighIP, uriNeighIP) {
			continue
		}
		if neighObj == nil || neighObj.Config == nil {
			continue
		}
		cfg := neighObj.Config
		ipStr := neighIP
		if cfg.Ip != nil && *cfg.Ip != "" {
			ipStr = *cfg.Ip
		}
		ipStr, err := getNormalizedIpStr(ipStr)
		if err != nil {
			return neighMap, tlerr.InvalidArgsError{Format: err.Error()}
		}
		if cfg.LinkLayerAddress == nil || *cfg.LinkLayerAddress == "" {
			return neighMap, tlerr.InvalidArgsError{Format: "link-layer-address not present"}
		}
		mac, err := normalizeNeighMac(*cfg.LinkLayerAddress)
		if err != nil {
			return neighMap, err
		}
		addNeighToMap(neighMap, ifName, ipStr, mac, family)
	}

	if len(neighMap[NEIGH_TN]) == 0 {
		return neighMap, tlerr.InvalidArgsError{Format: "No valid " + ipVersion + " neighbor entries to configure"}
	}
	return neighMap, nil
}

func processIpv6Neighbors(neighMap map[string]map[string]db.Value, subIntfObj *ocbinds.OpenconfigInterfaces_Interfaces_Interface_Subinterfaces_Subinterface,
	ifName, uriNeighIP, family, ipVersion string, inParams XfmrParams) (map[string]map[string]db.Value, error) {

	neighbors := subIntfObj.Ipv6.Neighbors
	if neighbors.Neighbor == nil || len(neighbors.Neighbor) == 0 {
		if uriNeighIP == "" {
			return neighMap, tlerr.InvalidArgsError{Format: ipVersion + " neighbor list is empty"}
		}
		if macStr := neighMacFromParam(inParams); macStr != "" {
			ipStr, err := getNormalizedIpStr(uriNeighIP)
			if err != nil {
				return neighMap, tlerr.InvalidArgsError{Format: err.Error()}
			}
			mac, err := normalizeNeighMac(macStr)
			if err != nil {
				return neighMap, err
			}
			addNeighToMap(neighMap, ifName, ipStr, mac, family)
			return neighMap, nil
		}
		neighObj, err := neighbors.NewNeighbor(uriNeighIP)
		if err != nil {
			return neighMap, err
		}
		neighbors.Neighbor = map[string]*ipv6Neighbor{uriNeighIP: neighObj}
	}

	for neighIP, neighObj := range neighbors.Neighbor {
		if uriNeighIP != "" && !neighIPsEqual(neighIP, uriNeighIP) {
			continue
		}
		if neighObj == nil || neighObj.Config == nil {
			continue
		}
		cfg := neighObj.Config
		ipStr := neighIP
		if cfg.Ip != nil && *cfg.Ip != "" {
			ipStr = *cfg.Ip
		}
		ipStr, err := getNormalizedIpStr(ipStr)
		if err != nil {
			return neighMap, tlerr.InvalidArgsError{Format: err.Error()}
		}
		if cfg.LinkLayerAddress == nil || *cfg.LinkLayerAddress == "" {
			return neighMap, tlerr.InvalidArgsError{Format: "link-layer-address not present"}
		}
		mac, err := normalizeNeighMac(*cfg.LinkLayerAddress)
		if err != nil {
			return neighMap, err
		}
		addNeighToMap(neighMap, ifName, ipStr, mac, family)
	}

	if len(neighMap[NEIGH_TN]) == 0 {
		return neighMap, tlerr.InvalidArgsError{Format: "No valid " + ipVersion + " neighbor entries to configure"}
	}
	return neighMap, nil
}

func neighMacFromParam(inParams XfmrParams) string {
	if inParams.param == nil {
		return ""
	}
	if mac, ok := inParams.param.(*string); ok && mac != nil {
		return *mac
	}
	return ""
}

func collectTargetNeighIPs(uriNeighIP string, intfObj *ocbinds.OpenconfigInterfaces_Interfaces_Interface, ipv6 bool) (map[string]bool, error) {
	targetIPs := make(map[string]bool)
	if uriNeighIP != "" {
		ipStr, err := getNormalizedIpStr(uriNeighIP)
		if err != nil {
			return targetIPs, tlerr.InvalidArgsError{Format: err.Error()}
		}
		targetIPs[ipStr] = true
		return targetIPs, nil
	}

	if intfObj == nil || intfObj.Subinterfaces == nil || intfObj.Subinterfaces.Subinterface == nil {
		return targetIPs, nil
	}
	subIntf, ok := intfObj.Subinterfaces.Subinterface[0]
	if !ok || subIntf == nil {
		return targetIPs, nil
	}

	if ipv6 {
		if subIntf.Ipv6 != nil && subIntf.Ipv6.Neighbors != nil && subIntf.Ipv6.Neighbors.Neighbor != nil {
			for neighIP := range subIntf.Ipv6.Neighbors.Neighbor {
				if ipStr, err := getNormalizedIpStr(neighIP); err == nil {
					targetIPs[ipStr] = true
				}
			}
		}
		return targetIPs, nil
	}

	if subIntf.Ipv4 != nil && subIntf.Ipv4.Neighbors != nil && subIntf.Ipv4.Neighbors.Neighbor != nil {
		for neighIP := range subIntf.Ipv4.Neighbors.Neighbor {
			if ipStr, err := getNormalizedIpStr(neighIP); err == nil {
				targetIPs[ipStr] = true
			}
		}
	}
	return targetIPs, nil
}

func intfNeighDel(d *db.DB, ifName, uriNeighIP string, intfObj *ocbinds.OpenconfigInterfaces_Interfaces_Interface, ipv6 bool) (map[string]map[string]db.Value, error) {
	neighMap := make(map[string]map[string]db.Value)
	neighMap[NEIGH_TN] = make(map[string]db.Value)
	family := NEIGH_FAMILY_IPV4
	if ipv6 {
		family = NEIGH_FAMILY_IPV6
	}

	ts := &db.TableSpec{Name: NEIGH_TN}
	keys, err := d.GetKeys(ts)
	if err != nil {
		return neighMap, err
	}

	targetIPs, err := collectTargetNeighIPs(uriNeighIP, intfObj, ipv6)
	if err != nil {
		return neighMap, err
	}

	for _, key := range keys {
		if len(key.Comp) < 2 || key.Comp[0] != ifName {
			continue
		}
		neighIP := key.Comp[1]
		if len(targetIPs) > 0 {
			match := false
			for targetIP := range targetIPs {
				if neighIPsEqual(neighIP, targetIP) {
					match = true
					break
				}
			}
			if !match {
				continue
			}
		}
		entry, derr := d.GetEntry(ts, key)
		if derr != nil || !isStaticNeighEntry(&entry, family) {
			continue
		}
		neighMap[NEIGH_TN][key.Comp[0]+PIPE+key.Comp[1]] = db.Value{Field: map[string]string{}}
	}

	if len(neighMap[NEIGH_TN]) == 0 && uriNeighIP != "" {
		return neighMap, tlerr.NotFound("Static %s neighbor %s on %s not found", family, uriNeighIP, ifName)
	}

	return neighMap, nil
}

func dbToYangIntfNeighXfmr(inParams XfmrParams, ipv6 bool) error {
	pathInfo := NewPathInfo(inParams.uri)
	ifName := pathInfo.Var("name")
	targetUriPath := pathInfo.YangPath
	uriNeighIP := pathInfo.Var("ip")
	family := NEIGH_FAMILY_IPV4
	if ipv6 {
		family = NEIGH_FAMILY_IPV6
	}

	if err := validateSubintfIndexZero(pathInfo.Var("index")); err != nil {
		return err
	}

	intfType, _, ierr := getIntfTypeByName(ifName)
	if intfType == IntfTypeUnset || ierr != nil {
		return errors.New("Invalid interface type")
	}
	if intfType != IntfTypeEthernet && intfType != IntfTypePortChannel {
		return nil
	}

	deviceObj := (*inParams.ygRoot).(*ocbinds.Device)
	if deviceObj.Interfaces == nil {
		deviceObj.Interfaces = &ocbinds.OpenconfigInterfaces_Interfaces{}
	}
	intfsObj := deviceObj.Interfaces
	ygot.BuildEmptyTree(intfsObj)
	intfObj, ok := intfsObj.Interface[ifName]
	if !ok {
		intfObj, _ = intfsObj.NewInterface(ifName)
	}
	ygot.BuildEmptyTree(intfObj)
	if intfObj.Subinterfaces == nil {
		intfObj.Subinterfaces = &ocbinds.OpenconfigInterfaces_Interfaces_Interface_Subinterfaces{}
	}
	ygot.BuildEmptyTree(intfObj.Subinterfaces)
	subIntf, ok := intfObj.Subinterfaces.Subinterface[0]
	if !ok {
		var err error
		subIntf, err = intfObj.Subinterfaces.NewSubinterface(0)
		if err != nil {
			return err
		}
	}
	ygot.BuildEmptyTree(subIntf)

	ts := &db.TableSpec{Name: NEIGH_TN}
	keys, err := inParams.d.GetKeys(ts)
	if err != nil {
		return err
	}

	for _, key := range keys {
		if len(key.Comp) < 2 || key.Comp[0] != ifName {
			continue
		}
		neighIP := key.Comp[1]
		if uriNeighIP != "" && !neighIPsEqual(neighIP, uriNeighIP) {
			continue
		}
		entry, derr := inParams.d.GetEntry(ts, key)
		if derr != nil || !isStaticNeighEntry(&entry, family) {
			continue
		}
		mac := entry.Field[NEIGH_MAC_FIELD]
		ipStr, ierr := getNormalizedIpStr(neighIP)
		if ierr != nil {
			ipStr = neighIP
		}

		if ipv6 {
			if subIntf.Ipv6 == nil {
				ygot.BuildEmptyTree(subIntf.Ipv6)
			}
			if subIntf.Ipv6.Neighbors == nil {
				subIntf.Ipv6.Neighbors = &ocbinds.OpenconfigInterfaces_Interfaces_Interface_Subinterfaces_Subinterface_Ipv6_Neighbors{}
			}
			neighObj, nerr := getOrCreateIpv6Neighbor(subIntf.Ipv6.Neighbors, ipStr)
			if nerr != nil {
				return nerr
			}
			populateIpv6NeighborConfig(neighObj, ipStr, mac)
			populateIpv6NeighborState(neighObj, ipStr, mac)
		} else {
			if subIntf.Ipv4 == nil {
				ygot.BuildEmptyTree(subIntf.Ipv4)
			}
			if subIntf.Ipv4.Neighbors == nil {
				subIntf.Ipv4.Neighbors = &ocbinds.OpenconfigInterfaces_Interfaces_Interface_Subinterfaces_Subinterface_Ipv4_Neighbors{}
			}
			neighObj, nerr := getOrCreateIpv4Neighbor(subIntf.Ipv4.Neighbors, ipStr)
			if nerr != nil {
				return nerr
			}
			populateIpv4NeighborConfig(neighObj, ipStr, mac)
			populateIpv4NeighborState(neighObj, ipStr, mac)
		}

		if isNeighborLeafGetPath(targetUriPath) {
			break
		}
	}

	return nil
}

func subscribeIntfNeigh(inParams XfmrSubscInParams) (XfmrSubscOutParams, error) {
	var result XfmrSubscOutParams
	pathInfo := NewPathInfo(inParams.uri)
	origTargetUriPath := pathInfo.YangPath

	if inParams.subscProc == TRANSLATE_EXISTS {
		// Defer parent-table checks to the YangToDb/DbToYang subtree callback.
		result.isVirtualTbl = true
		return result, nil
	}

	if inParams.subscProc == TRANSLATE_SUBSCRIBE {
		ifBasePath := "/openconfig-interfaces:interfaces/interface"
		targetUriPath := origTargetUriPath
		if strings.HasPrefix(targetUriPath, ifBasePath) {
			targetUriPath = targetUriPath[len(ifBasePath):]
		}
		if strings.HasPrefix(targetUriPath, "/subinterfaces/subinterface") {
			targetUriPath = targetUriPath[len("/subinterfaces/subinterface"):]
		}
		if idx := strings.Index(targetUriPath, "/openconfig-if-ip:ipv4/neighbors"); idx >= 0 {
			targetUriPath = targetUriPath[idx+len("/openconfig-if-ip:ipv4/neighbors"):]
		} else if idx := strings.Index(targetUriPath, "/ipv4/neighbors"); idx >= 0 {
			targetUriPath = targetUriPath[idx+len("/ipv4/neighbors"):]
		} else if idx := strings.Index(targetUriPath, "/openconfig-if-ip:ipv6/neighbors"); idx >= 0 {
			targetUriPath = targetUriPath[idx+len("/openconfig-if-ip:ipv6/neighbors"):]
		} else if idx := strings.Index(targetUriPath, "/ipv6/neighbors"); idx >= 0 {
			targetUriPath = targetUriPath[idx+len("/ipv6/neighbors"):]
		}

		neighborConfigPath := "/neighbor/config"
		if targetUriPath == "" || targetUriPath == "/neighbor" {
			result.isVirtualTbl = true
			return result, nil
		}

		result.onChange = OnchangeEnable
		result.nOpts = &notificationOpts{pType: OnChange}
		result.isVirtualTbl = false

		ifKey := pathInfo.StringVar("name", "*")
		neighIP := pathInfo.Var("ip")
		if neighIP == "" {
			neighIP = "*"
		} else if ipStr, err := getNormalizedIpStr(neighIP); err == nil {
			neighIP = ipStr
		}

		keyName := ifKey + PIPE + neighIP
		if targetUriPath == neighborConfigPath || strings.HasPrefix(targetUriPath, neighborConfigPath+"/") {
			result.dbDataMap = RedisDbSubscribeMap{db.ConfigDB: {NEIGH_TN: {keyName: {}}}}
		} else {
			result.isVirtualTbl = true
		}
		return result, nil
	}

	result.isVirtualTbl = true
	return result, nil
}

var Subscribe_intf_ipv4_neigh_xfmr SubTreeXfmrSubscribe = subscribeIntfNeigh
var Subscribe_intf_ipv6_neigh_xfmr SubTreeXfmrSubscribe = subscribeIntfNeigh
