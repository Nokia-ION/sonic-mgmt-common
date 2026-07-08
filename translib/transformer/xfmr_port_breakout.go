package transformer

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Azure/sonic-mgmt-common/cvl/custom_validation"
	"github.com/Azure/sonic-mgmt-common/translib/db"
	lvl "github.com/Azure/sonic-mgmt-common/translib/log"
	"github.com/Azure/sonic-mgmt-common/translib/ocbinds"
	"github.com/Azure/sonic-mgmt-common/translib/platform"
	"github.com/Azure/sonic-mgmt-common/translib/tlerr"
	log "github.com/golang/glog"
	"github.com/openconfig/ygot/ygot"
)

var (
	cfgMutex sync.RWMutex
)

const (
	BREAKOUT_CFG_TBL = "BREAKOUT_CFG"
	// SFLOW_STATE_INTF_TBL is the sFLOW session table in the Appl DB.
	// Defined locally here (mirrors nokia-umf translib/transformer/xfmr_sflow.go)
	// since the sflow transformer is not part of this port.
	SFLOW_STATE_INTF_TBL = "SFLOW_SESSION_TABLE"
)

type channelGroup struct {
	numBrkouts  uint8
	speed       string
	numPhyChnls uint8
}

var ocSpeedMap = map[ocbinds.E_OpenconfigIfEthernet_ETHERNET_SPEED]string{
	ocbinds.OpenconfigIfEthernet_ETHERNET_SPEED_SPEED_1GB:   "1G",
	ocbinds.OpenconfigIfEthernet_ETHERNET_SPEED_SPEED_5GB:   "5G",
	ocbinds.OpenconfigIfEthernet_ETHERNET_SPEED_SPEED_10GB:  "10G",
	ocbinds.OpenconfigIfEthernet_ETHERNET_SPEED_SPEED_25GB:  "25G",
	ocbinds.OpenconfigIfEthernet_ETHERNET_SPEED_SPEED_40GB:  "40G",
	ocbinds.OpenconfigIfEthernet_ETHERNET_SPEED_SPEED_50GB:  "50G",
	ocbinds.OpenconfigIfEthernet_ETHERNET_SPEED_SPEED_100GB: "100G",
	ocbinds.OpenconfigIfEthernet_ETHERNET_SPEED_SPEED_200GB: "200G",
	ocbinds.OpenconfigIfEthernet_ETHERNET_SPEED_SPEED_400GB: "400G",
	ocbinds.OpenconfigIfEthernet_ETHERNET_SPEED_SPEED_800GB: "800G",
}

/* Transformer specific functions */

func init() {
	XlateFuncBind("YangToDb_port_breakout_subtree_xfmr", YangToDb_port_breakout_subtree_xfmr)
	XlateFuncBind("DbToYang_port_breakout_subtree_xfmr", DbToYang_port_breakout_subtree_xfmr)
	XlateFuncBind("Subscribe_port_breakout_subtree_xfmr", Subscribe_port_breakout_subtree_xfmr)
}

func getDpbRoot(s *ygot.GoStruct) map[string]*ocbinds.OpenconfigPlatform_Components_Component {
	deviceObj := (*s).(*ocbinds.Device)
	return deviceObj.Components.Component
}

var Subscribe_port_breakout_subtree_xfmr = func(inParams XfmrSubscInParams) (XfmrSubscOutParams, error) {
	defer log.V(lvl.DEBUG).Info("Returning Subscribe_port_breakout_subtree_xfmr")

	ifName := platform.InterfaceNameFromPort(NewPathInfo(inParams.uri).Var("name"))

	if ifName == "" {
		// no need to verify dB data if we are requesting ALL components
		return XfmrSubscOutParams{isVirtualTbl: true}, nil
	}

	return XfmrSubscOutParams{dbDataMap: RedisDbSubscribeMap{db.ConfigDB: {BREAKOUT_CFG_TBL: {ifName: {}}}}}, nil
}

func populateChannelGroupsForSameSpeed(ifName, mode string) ([]channelGroup, error) {
	mode = strings.TrimSpace(mode)

	numBrkout, err := platform.IntfCountFromBrkoutMode(mode)
	if err != nil {
		return nil, err
	}

	idx := strings.Index(mode, "x")
	if idx == -1 {
		return nil, tlerr.New("Invalid breakout mode: %s.", mode)
	}

	speed := mode[idx+1:]

	lanes, err := platform.InterfaceLanes(ifName)
	if err != nil {
		log.V(lvl.DEBUG).Infof("Invalid primary interface %v (%v)", ifName, err)
		return nil, tlerr.NotSupported("Invalid primary interface %s (%v).", ifName, err)
	}

	numPhyCh := len(lanes)

	chGrp := channelGroup{
		numBrkouts:  uint8(numBrkout),
		speed:       speed,
		numPhyChnls: uint8(numPhyCh / numBrkout),
	}

	return []channelGroup{chGrp}, nil
}

func populateChannelGroupsForMixedSpeed(modes string) ([]channelGroup, error) {
	modes = strings.TrimSpace(modes)

	var chGrps []channelGroup
	for _, mode := range strings.Split(modes, "+") {
		numBrkout, err := platform.IntfCountFromBrkoutMode(mode)
		if err != nil {
			return nil, err
		}

		numPhyCh, err := platform.NumPhyChFromBrkoutMode(mode)
		if err != nil {
			return nil, err
		}

		speed, err := platform.SpeedFromBrkoutMode(mode)
		if err != nil {
			return nil, err
		}

		chGrp := channelGroup{
			numBrkouts:  uint8(numBrkout),
			speed:       strconv.Itoa(speed) + "G",
			numPhyChnls: uint8(numPhyCh / numBrkout),
		}

		chGrps = append(chGrps, chGrp)
	}

	return chGrps, nil
}

func populateChannelGroups(ifName, mode string) ([]channelGroup, error) {
	if strings.Contains(mode, "+") {
		return populateChannelGroupsForMixedSpeed(mode)
	}

	return populateChannelGroupsForSameSpeed(ifName, mode)
}

func fillChGroup(index int, chGrp channelGroup, grps *ocbinds.OpenconfigPlatform_Components_Component_Port_BreakoutMode_Groups) error {
	var err error

	idx := uint8(index)
	grp, ok := grps.Group[idx]
	if !ok || grp == nil {
		grp, err = grps.NewGroup(idx)
		if err != nil {
			return err
		}
	}
	ygot.BuildEmptyTree(grp)

	grp.Config.Index, grp.State.Index = &idx, &idx
	grp.Config.NumBreakouts, grp.State.NumBreakouts = &chGrp.numBrkouts, &chGrp.numBrkouts
	grp.Config.NumPhysicalChannels, grp.State.NumPhysicalChannels = &chGrp.numPhyChnls, &chGrp.numPhyChnls

	for ocSpeed, speed := range ocSpeedMap {
		if speed == chGrp.speed {
			grp.Config.BreakoutSpeed = ocSpeed
			grp.State.BreakoutSpeed = ocSpeed
		}
	}

	return nil
}

var DbToYang_port_breakout_subtree_xfmr SubTreeXfmrDbToYang = func(inParams XfmrParams) error {
	platObj := getDpbRoot(inParams.ygRoot)
	if platObj == nil || len(platObj) < 1 {
		log.V(lvl.ERROR).Info("DbToYang_port_breakout_subtree_xfmr: empty component.")
		return tlerr.NotSupported("DbToYang_port_breakout_subtree_xfmr: empty component.")
	}

	pathInfo := NewPathInfo(inParams.uri)
	port := pathInfo.Var("name")
	ifName := platform.InterfaceNameFromPort(port)
	if ifName == "" {
		log.V(lvl.ERROR).Info("DbToYang_port_breakout_subtree_xfmr: ifName is empty.")
		return tlerr.InvalidArgs("DbToYang_port_breakout_subtree_xfmr: ifName is empty.")
	}

	entry, err := inParams.d.GetEntry(&db.TableSpec{Name: BREAKOUT_CFG_TBL}, db.Key{Comp: []string{ifName}})
	if err != nil {
		return tlerr.NotFound("Failed to read DB entry, BREAKOUT_CFG|", ifName)
	}

	comp, ok := platObj[port]
	if !ok {
		return tlerr.NotSupported("Breakout not supported on %s.", port)
	}

	mode, err := sanitizeMode(entry.Get("brkout_mode"))
	if err != nil {
		return err
	}

	chGrps, err := populateChannelGroups(ifName, mode)
	if err != nil {
		return err
	}

	ygot.BuildEmptyTree(comp)
	ygot.BuildEmptyTree(comp.Port)
	ygot.BuildEmptyTree(comp.Port.BreakoutMode)
	ygot.BuildEmptyTree(comp.Port.BreakoutMode.Groups)

	if index := pathInfo.Var("index"); index != "" {
		idx, err := strconv.Atoi(index)
		if err != nil {
			return err
		}
		if idx >= len(chGrps) {
			return tlerr.New("Invalid breakout index: %d.", idx)
		}
		return fillChGroup(idx, chGrps[idx], comp.Port.BreakoutMode.Groups)
	}

	// Populate for all indices.
	for idx, grp := range chGrps {
		if err = fillChGroup(idx, grp, comp.Port.BreakoutMode.Groups); err != nil {
			return err
		}
	}

	return nil
}

func processMd(mode string) (string, error) {
	mode = strings.TrimSpace(mode)
	idx := strings.Index(mode, "G")
	if idx == -1 {
		return "", tlerr.New("Breakout mode is malformed: %s.", mode)
	}

	return mode[:idx+1] + "(" + mode[idx+1:] + ")", nil
}

func sanitizeMode(mode string) (string, error) {
	if strings.Contains(mode, "+") {
		mode = strings.ReplaceAll(mode, "+", "_")
		mode = strings.ReplaceAll(mode, "(", "")
		return strings.ReplaceAll(mode, ")", ""), nil
	}

	if strings.Contains(mode, "_") {
		var mds []string
		modes := strings.Split(mode, "_")
		for _, md := range modes {
			smd, err := processMd(md)
			if err != nil {
				return "", err
			}

			mds = append(mds, smd)
		}

		return strings.Join(mds, "+"), nil
	}

	return mode, nil
}

func validateIndex(grps map[uint8]*ocbinds.OpenconfigPlatform_Components_Component_Port_BreakoutMode_Groups_Group, port string) error {
	s := map[uint8]bool{}
	for k, grp := range grps {
		if grp.Index == nil || grp.Config == nil || grp.Config.Index == nil {
			log.V(lvl.ERROR).Infof("index is empty for group %v for port %v", k, port)
			return tlerr.InvalidArgs("index is empty for group %v for port %v", k, port)
		}
		idx := *grp.Index
		if idx != *grp.Config.Index {
			log.V(lvl.ERROR).Infof("Breakout index doesn't match: %d vs %d.", idx, *grp.Config.Index)
			return tlerr.InvalidArgs("Breakout index doesn't match: %d vs %d.", idx, *grp.Config.Index)
		}

		if _, ok := s[idx]; ok {
			log.V(lvl.ERROR).Infof("Breakout index %d already exists!", idx)
			return tlerr.InvalidArgs("Breakout index %d already exists!", idx)
		}
		s[idx] = true

		if idx >= uint8(len(grps)) {
			log.V(lvl.ERROR).Infof("Breakout index %d is not strictly ordered (starting from 0 index)!", idx)
			return tlerr.InvalidArgs("Breakout index %d is not strictly ordered (starting from 0 index)!", idx)
		}
	}

	return nil
}

func populateBrkoutMode(grps map[uint8]*ocbinds.OpenconfigPlatform_Components_Component_Port_BreakoutMode_Groups_Group, port, ifName string) (string, error) {
	if err := validateIndex(grps, port); err != nil {
		return "", err
	}

	lanes, err := platform.InterfaceLanes(ifName)
	if err != nil {
		log.V(lvl.DEBUG).Infof("Invalid primary interface %v (%v)", ifName, err)
		return "", tlerr.NotSupported("Invalid primary interface %s (%v).", ifName, err)
	}
	maxLanes := uint8(len(lanes))
	var totalLanes uint8

	var modes []string
	// The breakout mode has to be strictly ordered based on the group index.
	// Hence the grps map has to be iterated in order (b/211691772)
	// validateIndex function has already verified for duplicates and
	// if the index is < len(grps).
	for k := uint8(0); k < uint8(len(grps)); k++ {
		grp := grps[k]
		if grp.Config == nil {
			return "", tlerr.InvalidArgs("group config is empty for group %v for port %v", k, port)
		}
		if grp.Config.BreakoutSpeed == ocbinds.OpenconfigIfEthernet_ETHERNET_SPEED_UNSET {
			log.V(lvl.ERROR).Infof("breakout-speed is empty for group %v for port %v", k, port)
			return "", tlerr.InvalidArgs("breakout-speed is empty for group %v for port %v", k, port)
		}
		speed, ok := ocSpeedMap[grp.Config.BreakoutSpeed]
		if !ok {
			log.V(lvl.ERROR).Infof("Invalid breakout speed: (%v) for group %v, port %v", grp.Config.BreakoutSpeed, k, port)
			return "", tlerr.InvalidArgs("Invalid breakout speed: (%v) for group %v, port %v", grp.Config.BreakoutSpeed, k, port)
		}

		if grp.Config.NumBreakouts == nil {
			log.V(lvl.ERROR).Infof("num-breakouts is empty for group %v for port %v", k, port)
			return "", tlerr.InvalidArgs("num-breakouts is empty for group %v for port %v", k, port)
		}
		numBreakouts := *grp.Config.NumBreakouts
		mode := fmt.Sprintf("%dx%s", numBreakouts, speed)

		if grp.Config.NumPhysicalChannels == nil {
			return "", tlerr.InvalidArgs("num-physical-channels is empty for group %v for port %v", k, port)
		}
		numPhyCh := *grp.Config.NumPhysicalChannels
		// NumPhysicalChannels should not have invalid values such as 0 or greater than max lanes
		if numPhyCh <= 0 || numPhyCh > maxLanes {
			return "", tlerr.InvalidArgs("Invalid num-physical-channels: (%v) for group %v, port %v", numPhyCh, k, port)
		}
		numLanes := numPhyCh * numBreakouts
		totalLanes += numLanes
		mode = fmt.Sprintf("%s(%d)", mode, numLanes)

		modes = append(modes, mode)
	}

	// NumPhysicalChannels * NumBreakouts should be equal to max lanes
	if totalLanes != maxLanes {
		return "", tlerr.InvalidArgs("total lanes (%v) should be equal to max lanes (%v) for port %v", totalLanes, maxLanes, port)
	}

	// Same speed breakout shouldn't include NumPhysicalChannels. If it still does, remove from the generated mode!
	if len(modes) == 1 && strings.Contains(modes[0], "(") {
		return modes[0][:strings.Index(modes[0], "(")], nil
	}

	return strings.Join(modes, "+"), nil
}

func getRefCount(cfgDB *db.DB, stdb *db.DB, port string) (int, error) {
	_, err := cfgDB.GetEntry(&db.TableSpec{Name: "PORT"}, db.Key{Comp: []string{port}})
	if err != nil {
		return 0, nil
	}

	dpbEntry, err := stdb.GetEntry(&db.TableSpec{Name: "PORT_TABLE"}, db.Key{Comp: []string{port}})
	if err != nil {
		return -1, err
	}

	refCount, err := strconv.Atoi(dpbEntry.Get("ref_count"))
	if err != nil {
		log.V(lvl.ERROR).Info(err.Error())
		return -1, err
	}

	return refCount, nil
}

func foundFlowProgramming(cfgdb *db.DB, appstdb *db.DB, stdb *db.DB, ports []platform.InterfaceProperties) error {

	for _, p := range ports {
		ifName := p.Name
		refCount, err := getRefCount(cfgdb, stdb, ifName)
		if err != nil {
			return err
		}
		// By default, an unnumbered subinterface is created from the config_db.json file.
		if _, err = cfgdb.GetEntry(&db.TableSpec{Name: "INTERFACE"}, db.Key{Comp: []string{ifName}}); err != nil {
			log.V(lvl.WARNING).Infof("INTERFACE not found in Config DB for port %v; err = %v", ifName, err)
		}
		intfEntryCnt := 0
		if _, err = appstdb.GetEntry(&db.TableSpec{Name: "INTF_TABLE"}, db.Key{Comp: []string{ifName}}); err != nil {
			log.V(lvl.WARNING).Infof("INTF_TABLE not found in Appl DB for port %v; err = %v", ifName, err)
		} else {
			intfEntryCnt++
		}

		// Check if the interface is a lag member, and increment the interface entry count.
		if lagEntryKeys, err := appstdb.GetKeysPattern(&(db.TableSpec{Name: "LAG_MEMBER_TABLE"}), db.Key{[]string{"*", ifName}}); err != nil || len(lagEntryKeys) == 0 {
			log.V(lvl.WARNING).Infof("LAG_MEMBER_TABLE entry not found in Appl DB for port %v; err = %v", ifName, err)
		} else {
			intfEntryCnt++
		}

		// Check if the interface has a corresponding SFLOW entry, and increment the interface entry count.
		if _, err = appstdb.GetEntry(&db.TableSpec{Name: SFLOW_STATE_INTF_TBL}, db.Key{Comp: []string{ifName}}); err != nil {
			log.V(lvl.WARNING).Infof("SFLOW_SESSION_TABLE entry not found in Appl DB for port %v; err = %v", ifName, err)
		} else {
			intfEntryCnt++
		}

		// Check if the interface has corresponding QoS Buffer Queue config tables, and increment the interface entry count.
		if qosEntryKeys, err := appstdb.GetKeysPattern(&(db.TableSpec{Name: "BUFFER_QUEUE_TABLE"}), db.Key{[]string{ifName, "*"}}); err != nil || len(qosEntryKeys) == 0 {
			log.V(lvl.WARNING).Infof("BUFFER_QUEUE_TABLE entry not found in Appl DB for port %v; err = %v", ifName, err)
		} else {
			intfEntryCnt += len(qosEntryKeys)
		}

		// Check if the interface has corresponding QoS Buffer PG config tables, and increment the interface entry count.
		if qosBufferPGKeys, err := appstdb.GetKeysPattern(&(db.TableSpec{Name: "BUFFER_PG_TABLE"}), db.Key{[]string{ifName, "*"}}); err != nil || len(qosBufferPGKeys) == 0 {
			log.V(lvl.WARNING).Infof("BUFFER_PG_TABLE entry not found in Appl DB for port %v; err = %v", ifName, err)
		} else {
			intfEntryCnt += len(qosBufferPGKeys)
		}

		if refCount != intfEntryCnt {
			custom_validation.SetRefCountCheckStatus(true)
			return tlerr.InvalidArgs("YangToDb_port_breakout_subtree_xfmr: port %s is in use! ref count: %d, interface count: %d", ifName, refCount, intfEntryCnt)
		}
	}
	return nil
}

var YangToDb_port_breakout_subtree_xfmr SubTreeXfmrYangToDb = func(inParams XfmrParams) (map[string]map[string]db.Value, error) {
	log.V(lvl.DEBUG).Infof("DPB: %v\n", inParams)

	platObj := getDpbRoot(inParams.ygRoot)
	if platObj == nil || len(platObj) < 1 {
		log.V(lvl.DEBUG).Info("YangToDb_port_breakout_subtree_xfmr: Empty component.")
		return nil, tlerr.NotSupported("YangToDb_port_breakout_subtree_xfmr: Empty component.")
	}
	pathInfo := NewPathInfo(inParams.uri)
	portName := pathInfo.Var("name")
	ifName := platform.InterfaceNameFromPort(portName)
	if ifName == "" {
		log.V(lvl.DEBUG).Info("YangToDb_port_breakout_subtree_xfmr: ifName is empty")
		return nil, tlerr.InvalidArgs("YangToDb_port_breakout_subtree_xfmr: ifName is empty")
	}

	dpb_entry, err := inParams.d.GetEntry(&db.TableSpec{Name: BREAKOUT_CFG_TBL}, db.Key{Comp: []string{ifName}})
	if err != nil {
		return nil, err
	}
	log.V(lvl.DEBUG).Info("Read DB entry BREAKOUT_CFG for interface " + ifName)
	log.V(lvl.DEBUG).Info("CURRENT: ", dpb_entry)

	log.V(lvl.DEBUG).Info("DPB Path: ", pathInfo, ", DPB ifName: ", ifName, ", DPB Platform Object: ", platObj[portName])
	if inParams.oper == DELETE {
		log.V(lvl.DEBUG).Info("DEL breakout config BREAKOUT_CFG for interface " + ifName)
		log.V(lvl.DEBUG).Info("CURRENT: ", dpb_entry)
		fromModePerPlatform, err := sanitizeMode(dpb_entry.Get("brkout_mode"))
		if err != nil {
			return nil, err
		}
		if ports, err := platform.IntfsFromBrkoutMode(ifName, fromModePerPlatform); err == nil {
			log.V(lvl.DEBUG).Info("PORTS TO BE DELETED: ", ports)
		}
		err = modifyPortFootprint(ifName, fromModePerPlatform, "", inParams)
		if err != nil {
			return nil, err
		}
		return map[string]map[string]db.Value{
			BREAKOUT_CFG_TBL: map[string]db.Value{
				ifName: db.Value{
					Field: map[string]string{
						"brkout_mode": "",
					},
				},
			},
		}, nil
	}

	// toModePerPlatform is the target brkout_mode in platform.json format. Example: 2x100G(4)+1x200G(4)
	toModePerPlatform, err := populateBrkoutMode(platObj[portName].Port.BreakoutMode.Groups.Group, portName, ifName)
	if err != nil {
		return nil, err
	}
	log.V(lvl.DEBUG).Info("Breakout mode to set: ", toModePerPlatform)

	// fromModePerSonic is the previous brkout_mode in SONIC format. Example: 2x100G4_1x200G4
	fromModePerSonic := dpb_entry.Get("brkout_mode")
	if fromModePerSonic == "" {
		log.V(lvl.DEBUG).Info("brkout_mode field not present in DB.")
		if fromModePerSonic, err = platform.DefaultBrkoutMode(ifName); err != nil {
			return nil, tlerr.NotFound("brkout_mode field not found in DB or platform.json file.")
		}
	}

	fromModePerPlatform, err := sanitizeMode(fromModePerSonic)
	if err != nil {
		return nil, err
	}
	toModePerSonic, err := sanitizeMode(toModePerPlatform)
	if err != nil {
		return nil, err
	}

	log.V(lvl.DEBUG).Infof("DPB: from_mode = %v; to_mode = %v", fromModePerPlatform, toModePerPlatform)

	if inParams.oper == REPLACE {
		err = modifyPortFootprint(ifName, fromModePerPlatform, toModePerPlatform, inParams)
		if err != nil {
			log.V(lvl.DEBUG).Info("Breakout failed for ", ifName)
			return nil, err
		}
	}
	return map[string]map[string]db.Value{
		BREAKOUT_CFG_TBL: map[string]db.Value{
			ifName: db.Value{
				Field: map[string]string{
					"brkout_mode": toModePerSonic,
					"port":        portName,
					"lanes":       platform.PlatformIntfLanesStr(ifName),
				},
			},
		},
	}, nil
}

/* Breakout action, shutdown, remove dependent configs, remove ports, add ports */
func modifyPortFootprint(pport, from_mode, to_mode string, inParams XfmrParams) error {
	currPortsInDb, err := generateDeployedPortsList(pport, from_mode)
	if err != nil {
		log.V(lvl.ERROR).Infof("No breakout happen for %v. Err: %v", pport, err)
		return err
	}

	portsInCfg, err := generatePortInConfigPortsList(pport, to_mode, getIntfsRoot(inParams.ygRoot), currPortsInDb, inParams.oper)
	if err != nil {
		log.V(lvl.ERROR).Infof("No breakout happen for %v. Err: %v", pport, err)
		return err
	}

	laneSetChanged, err := haveLaneSetsChanged(pport, from_mode, to_mode)
	if err != nil {
		log.V(lvl.ERROR).Infof("No breakout happen for %v. Err: %v", pport, err)
		return err
	}

	// 1. No breakout action if no change in breakout mode or only port-speed change.
	if haveSamePortNames(portsInCfg, currPortsInDb) && !laneSetChanged {
		log.V(lvl.DEBUG).Info("No change in port breakout mode.")
		return nil
	}

	// 2. Only remove the port with lane set changed
	portsToDelete := findPortsToDelete(currPortsInDb, portsInCfg)

	appstdb, err := db.NewDB(getDBOptions(db.ApplDB))
	if err != nil {
		return err
	}
	defer appstdb.DeleteDB()
	stdb, err := db.NewDB(getDBOptions(db.StateDB))
	if err != nil {
		return err
	}
	defer stdb.DeleteDB()

	// 3. Check the if there is flow on the port.
	if err := foundFlowProgramming(inParams.d, appstdb, stdb, portsToDelete); err != nil {
		return err
	}

	// 4. Remove ports.
	delMap, err := removePorts(inParams.d, appstdb, portsToDelete, portsInCfg)
	if err != nil {
		return err
	}
	updateSubOpDataMap(delMap, DELETE, inParams)
	log.V(lvl.DEBUG).Info("PORTS IN CONFIG DB: ", currPortsInDb)

	// 5. Add ports.
	addMap := addPorts(portsInCfg)
	updateSubOpDataMap(addMap, CREATE, inParams)
	log.V(lvl.DEBUG).Info("PORTS IN CONFIG: ", portsInCfg)

	// 6. Lock ports by writing pending_delete to DB directly, set unlock required signal.
	if err := updatePendingDelete(portsToDelete, inParams); err != nil {
		return err
	}

	*inParams.pCascadeDelTbl = append(*inParams.pCascadeDelTbl, "PORT")
	return nil
}

func findPortsToDelete(currPortsInDb []platform.InterfaceProperties, portsInCfg []platform.InterfaceProperties) []platform.InterfaceProperties {
	deletablePorts := make(map[string]platform.InterfaceProperties)

	// Construct the map to track deletable ports.
	// All ports in config_db are possible to be deleted
	for _, cur := range currPortsInDb {
		deletablePorts[cur.Name] = cur
	}

	// Filter out the no lane change and no speed change port from deletablePorts
	for _, cfg := range portsInCfg {
		if cur, ok := deletablePorts[cfg.Name]; ok && slices.Equal(cfg.Lanes, cur.Lanes) && cfg.SpeedMbps == cur.SpeedMbps {
			delete(deletablePorts, cfg.Name)
		}
	}

	var portsToDelete []platform.InterfaceProperties
	for _, port := range deletablePorts {
		portsToDelete = append(portsToDelete, port)
	}
	return portsToDelete
}
func generatePortInConfigPortsList(pport, to_mode string, intfsObj *ocbinds.OpenconfigInterfaces_Interfaces, currPortsInDb []platform.InterfaceProperties, oper Operation) ([]platform.InterfaceProperties, error) {
	target_ports, err := platform.IntfsFromBrkoutMode(pport, to_mode)
	if err != nil {
		return nil, err
	}

	// With DELETE op, use default breakout mode's ports
	if oper == DELETE {
		return target_ports, nil
	}

	// Default intent to deployed state, in case /interfaces omitted
	if intfsObj == nil {
		return currPortsInDb, nil
	}

	var portsInCfg []platform.InterfaceProperties
	// Check if ports generated by target dbp mode need to be added to config_db
	for _, port := range target_ports {
		if _, ok := intfsObj.Interface[port.Name]; ok {
			portsInCfg = append(portsInCfg, port)
		}
	}

	return portsInCfg, nil
}

func generateDeployedPortsList(pport, from_mode string) ([]platform.InterfaceProperties, error) {
	curr_ports, err := platform.IntfsFromBrkoutMode(pport, from_mode)
	if err != nil {
		return nil, err
	}

	// Fetch a Redis Client to check for interfaces that exist in the ConfigDB.
	// The inParams DB object cannot be used here because the DB API also accesses
	// the transaction map (intent) to check for existing keys, sometimes returning
	// keys that are not in the ConfigDB yet.
	cfgDb := db.RedisClient(db.ConfigDB)
	defer db.CloseRedisClient(cfgDb)

	var currPortsInDb []platform.InterfaceProperties
	for _, port := range curr_ports {
		exists, err := cfgDb.Exists(context.Background(), fmt.Sprintf("PORT|%v", port.Name)).Result()
		if err == nil && exists > 0 {
			currPortsInDb = append(currPortsInDb, port)
		}
	}

	return currPortsInDb, nil
}

// Check the if two port lists are with the same port name.
// lists are ordered by port name.
func haveSamePortNames(a, b []platform.InterfaceProperties) bool {
	if len(a) != len(b) {
		return false
	}
	for i, _ := range a {
		if a[i].Name != b[i].Name {
			return false
		}
	}
	return true
}

// If the deployed port footprint and intented port footprint are the same,
func haveLaneSetsChanged(pport, from_mode, to_mode string) (bool, error) {
	// check if the lane set is the same.
	fromLanes, err := platform.LaneSetFromBrkoutMode(pport, from_mode)
	if err != nil {
		return false, err
	}
	toLanes, err := platform.LaneSetFromBrkoutMode(pport, to_mode)
	if err != nil {
		return false, err
	}

	if reflect.DeepEqual(fromLanes, toLanes) {
		return false, nil
	}
	return true, nil
}

func addPorts(ports []platform.InterfaceProperties) map[db.DBNum]map[string]map[string]db.Value {
	subOpMap := make(map[db.DBNum]map[string]map[string]db.Value)
	addMap := make(map[string]map[string]db.Value)
	entryMap := make(map[string]db.Value)
	interfaceEntryMap := make(map[string]db.Value)

	for _, port := range ports {
		/* Convert the int array of lanes to a string. */
		lanesStr := make([]string, len(port.Lanes))
		for i, l := range port.Lanes {
			lanesStr[i] = strconv.Itoa(l)
		}
		lanes := strings.Join(lanesStr, ",")

		m := make(map[string]string)
		value := db.Value{Field: m}
		value.Set("index", strconv.Itoa(port.Index))
		value.Set("lanes", lanes)
		value.Set("alias", port.Alias)
		value.Set("speed", strconv.Itoa(port.SpeedMbps))
		entryMap[port.Name] = value

		n := make(map[string]string)
		interfaceMapValue := db.Value{Field: n}
		interfaceMapValue.Set("unnumbered_enabled", "true")
		interfaceEntryMap[port.Name] = interfaceMapValue
	}

	addMap["PORT"] = entryMap
	addMap["INTERFACE"] = interfaceEntryMap
	// NOTE (nokia-cpn port): The UMF code re-added PORTCHANNEL_MEMBER entries for
	// breakout ports using the interfaces transformer's package-global pcMembers
	// map (translib/transformer/xfmr_intf.go). That transformer is not part of this
	// port, so the pcMembers-driven PORTCHANNEL_MEMBER handling is intentionally
	// dropped here.
	log.V(lvl.DEBUG).Info("DPB: CREATE Map", addMap)
	subOpMap[db.ConfigDB] = addMap
	return subOpMap
}

func removePorts(cfgdb *db.DB, d *db.DB, ports_i []platform.InterfaceProperties, ports_n []platform.InterfaceProperties) (map[db.DBNum]map[string]map[string]db.Value, error) {
	subOpMap := make(map[db.DBNum]map[string]map[string]db.Value)
	delMap := make(map[string]map[string]db.Value)
	intfDelMap := make(map[string]db.Value)
	portDelMap := make(map[string]db.Value)
	portExistsMap := make(map[string]bool)
	portChannelMemberDelMap := make(map[string]db.Value)
	sflowDelMap := make(map[string]db.Value)
	portQosDelMap := make(map[string]db.Value)
	qosQueueDelMap := make(map[string]db.Value)
	qosBufferQueueDelMap := make(map[string]db.Value)
	qosBufferPGDelMap := make(map[string]db.Value)

	for _, port := range ports_n {
		portExistsMap[port.Name] = true
	}
	for _, port := range ports_i {
		// Field value map is null to indicate entire entry delete.
		/* Delete the PORT table entry if the port is not modified
		(deleted, then added back).
		*/
		ifName := port.Name
		if !portExistsMap[ifName] {
			portDelMap[ifName] = db.Value{}
		} else {
			intfDelMap[ifName] = db.Value{}
		}
		pcKeys, err := d.GetKeysPattern(&db.TableSpec{Name: "LAG_MEMBER_TABLE"}, db.Key{Comp: []string{"*", ifName}})
		if err != nil || len(pcKeys) == 0 {
			log.V(lvl.DEBUG).Infof("Failed to get keys from LAG_MEMBER_TABLE table for intf %v", ifName)
		} else {
			for _, key := range pcKeys {
				if key.Len() < 2 {
					continue
				}
				portChannelMemberKey := key.Get(0) + "|" + key.Get(1)
				portChannelMemberDelMap[portChannelMemberKey] = db.Value{}
			}
		}
		// Check if SFLOW_SESSION table is present in Config DB before adding it to the delete list.
		// This is beacuse default SFLOW_SESSION_TABLE entries for ports are always added by sflowmgr
		// when ports are created.
		if _, err = cfgdb.GetEntry(&db.TableSpec{Name: SFLOW_INTF_TBL}, db.Key{Comp: []string{ifName}}); err != nil {
			log.V(lvl.DEBUG).Infof("Config DB entry for table to be deleted does not exist for intf %v", ifName)
		} else {
			// Also check if the corresponding entry is present in Appl DB for presence
			// of valid sFlow reference created in the BE.
			if _, err = d.GetEntry(&db.TableSpec{Name: SFLOW_STATE_INTF_TBL}, db.Key{Comp: []string{ifName}}); err != nil {
				log.V(lvl.DEBUG).Infof("Failed to get DB entry for SFLOW_SESSION_TABLE table for intf %v", ifName)
			} else {
				sflowDelMap[ifName] = db.Value{}
			}
		}
		// Port QoS Map
		if _, err = cfgdb.GetEntry(&db.TableSpec{Name: "PORT_QOS_MAP"}, db.Key{Comp: []string{ifName}}); err != nil {
			log.V(lvl.DEBUG).Infof("Config DB entry for table to be deleted does not exist for intf %v", ifName)
		} else {
			portQosDelMap[ifName] = db.Value{}
		}
		// QoS Queue Tables
		qosKeys, err := cfgdb.GetKeysPattern(&db.TableSpec{Name: "QUEUE"}, db.Key{Comp: []string{ifName, "*"}})
		if err != nil || len(qosKeys) == 0 {
			log.V(lvl.DEBUG).Infof("Failed to get keys from QUEUE table for intf %v", ifName)
		} else {
			for _, key := range qosKeys {
				if key.Len() < 2 {
					continue
				}
				queueKey := key.Get(0) + "|" + key.Get(1)
				qosQueueDelMap[queueKey] = db.Value{}
			}
		}
		// QoS Buffer Queue Tables
		qosBufKeys, err := cfgdb.GetKeysPattern(&db.TableSpec{Name: "BUFFER_QUEUE"}, db.Key{Comp: []string{ifName, "*"}})
		if err != nil || len(qosBufKeys) == 0 {
			log.V(lvl.DEBUG).Infof("Failed to get keys from BUFFER_QUEUE table for intf %v", ifName)
		} else {
			for _, key := range qosBufKeys {
				if key.Len() < 2 {
					continue
				}
				bufQueueKey := key.Get(0) + "|" + key.Get(1)
				qosBufferQueueDelMap[bufQueueKey] = db.Value{}
			}
		}
		// QoS Buffer PG Tables
		qosBufPGKeys, err := cfgdb.GetKeysPattern(&db.TableSpec{Name: "BUFFER_PG"}, db.Key{Comp: []string{ifName, "*"}})
		if err != nil || len(qosBufPGKeys) == 0 {
			log.V(lvl.DEBUG).Infof("Failed to get keys from BUFFER_PG table for intf %v", ifName)
		} else {
			for _, key := range qosBufPGKeys {
				if key.Len() == 2 {
					bufPGKey := key.Get(0) + "|" + key.Get(1)
					qosBufferPGDelMap[bufPGKey] = db.Value{}
				}
			}
		}
	}
	if len(portDelMap) != 0 {
		delMap["PORT"] = portDelMap
	}
	if len(intfDelMap) != 0 {
		delMap["INTERFACE"] = intfDelMap
	}
	if len(portChannelMemberDelMap) != 0 {
		delMap[PORTCHANNEL_MEMBER_TN] = portChannelMemberDelMap
	}
	if len(sflowDelMap) != 0 {
		delMap[SFLOW_INTF_TBL] = sflowDelMap
	}
	if len(portQosDelMap) != 0 {
		delMap["PORT_QOS_MAP"] = portQosDelMap
	}
	if len(qosQueueDelMap) != 0 {
		delMap["QUEUE"] = qosQueueDelMap
	}
	if len(qosBufferQueueDelMap) != 0 {
		delMap["BUFFER_QUEUE"] = qosBufferQueueDelMap
	}
	if len(qosBufferPGDelMap) != 0 {
		delMap["BUFFER_PG"] = qosBufferPGDelMap
	}
	log.V(lvl.DEBUG).Info("DPB: DELETE Map", delMap)
	subOpMap[db.ConfigDB] = delMap
	return subOpMap, nil
}

func updateSubOpDataMap(subOpMap map[db.DBNum]map[string]map[string]db.Value, oper Operation, inParams XfmrParams) {
	if len(subOpMap[db.ConfigDB]) == 0 {
		return
	}
	if inParams.subOpDataMap[oper] == nil {
		inParams.subOpDataMap[oper] = &subOpMap
		return
	}
	if (*inParams.subOpDataMap[oper])[db.ConfigDB] == nil {
		(*inParams.subOpDataMap[oper])[db.ConfigDB] = make(map[string]map[string]db.Value)
	}
	mapCopy((*inParams.subOpDataMap[oper])[db.ConfigDB], subOpMap[db.ConfigDB])
}

func updatePendingDelete(delPorts []platform.InterfaceProperties, inParams XfmrParams) error {
	rclient, err := db.NewRedisConfigDBClient()
	if err != nil {
		return err
	}
	defer db.CloseRedisClient(rclient)
	m := make(map[string]interface{})
	m["phase"] = "pending_delete"
	m["timestamp"] = strconv.FormatInt(time.Now().Unix(), 10)
	u := make(map[string]interface{})
	u["NULL"] = "NULL"
	// Lock for writing
	cfgMutex.Lock()
	defer cfgMutex.Unlock()
	for _, port := range delPorts {
		if err := rclient.HMSet(context.Background(), "PORT_STATE|"+port.Name, m).Err(); err != nil {
			log.V(lvl.ERROR).Infof("Error in locking port %v during DPB: %v", port.Name, err.Error())
			return err
		}
		log.V(lvl.INFO).Infof("Locked port %v during DPB.", port.Name)
		if err := rclient.HMSet(context.Background(), "PORT_UNLOCK|"+port.Name, u).Err(); err != nil {
			log.V(lvl.ERROR).Infof("Error in setting unlock required for port %v during DPB: %v", port.Name, err.Error())
			return err
		}
		log.V(lvl.INFO).Infof("Set unlock required for port %v during DPB.", port.Name)
	}
	return nil
}
