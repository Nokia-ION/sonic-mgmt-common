//////////////////////////////////////////////////////////////////////////
//
// Copyright 2020 Dell, Inc.
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
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	lvl "github.com/Azure/sonic-mgmt-common/translib/log"
	"github.com/Azure/sonic-mgmt-common/translib/ocbinds"
	"github.com/Azure/sonic-mgmt-common/translib/platform"
	"github.com/Azure/sonic-mgmt-common/translib/tlerr"
	log "github.com/golang/glog"
	"github.com/openconfig/ygot/ygot"
)

const (
	PSU_TBL             = "PSU_INFO"
	FAN_TBL             = "FAN_INFO"
	FAN_TRAY_TBL        = "FAN_DRAWER_INFO"
	TRANSCEIVER_TBL     = "TRANSCEIVER_INFO"
	TRANSCEIVER_STATUS  = "TRANSCEIVER_STATUS"
	TRANSCEIVER_CONTROL = "TRANSCEIVER_CONTROL"
	TRANSCEIVER_DOM     = "TRANSCEIVER_DOM_SENSOR"
	FPGA_TBL            = "FPGA_INFO"
	PORT_TBL            = "PORT_TABLE"
	BREAKOUT_TBL        = "BREAKOUT_CFG"
	PORT_BREAKOUT       = "PORT_BREAKOUT"
	STORAGE_INFO        = "STORAGE_INFO"
	DISK_CARRIER_TBL    = "DISK_CARRIER_INFO"
	SWITCH_EVENT        = "SWITCH_EVENT"
	NODE_CFG_TBL        = "NODE_CFG"
	SW_COMP_TBL         = "SW_COMP_INFO"
	CHASSIS_CFG         = "CHASSIS_CFG"
	CHASSIS_TBL         = "CHASSIS_INFO"
	HSM_TBL             = "FW_SECURITY_INFO"
	PCIE_TBL            = "PCIE_DEVICE"
	POWER_INFO_TBL      = "POWER_INFO"
	PHYS_ENTITY_TBL     = "PHYSICAL_ENTITY_INFO"
	CONTROLLER_CARD_TBL = "CONTROLLER_CARD_INFO"
	LINECARD_TBL        = "LINECARD_INFO"
	HUMIDITY_SENSOR_TBL = "HUMIDITY_INFO"
	TEMP_TBL            = "TEMPERATURE_INFO"
	OCS_PORT_TBL        = "OCS_PORT_INFO"
	CURRENT_INFO_TBL    = "CURRENT_INFO"
	VOLTAGE_INFO_TBL    = "VOLTAGE_INFO"
	FAULT_INFO_TBL      = "FAULT_INFO"
	EEPROM_TBL          = "EEPROM_INFO"

	// SYS_EEPROM_NAME is the synthetic component name under which the system
	// EEPROM contents (EEPROM_INFO table) are exposed. Previously served by the
	// standalone pfm_app.go; folded into this transformer so a single handler
	// owns the /openconfig-platform:components tree.
	SYS_EEPROM_NAME = "System Eeprom"

	XCVR_LANE_LIMIT = 8

	XCVR_KEY_PREFIX     = "Ethernet"
	STORAGE_NAME_PREFIX = "/*"
	IC_NAME_PREFIX      = "integrated_circuit"
	CHASSIS_PREFIX      = "chassis"
	NW_STACK_PREFIX     = "network_stack"
	OS_PREFIX           = "os"
	BOOTL_PREFIX        = "boot_loader"
	HVL_PREFIX          = "haven"
	DTL_PREFIX          = "dauntless"
	VR_PREFIX           = "voltage_regulator"
	PB_PREFIX           = "power_brick"
	PH_PREFIX           = "hotswap"
	PSEQ_PREFIX         = "power_sequencer"
	FAN_TRAY_PREFIX     = "FanTray"
	FAN_TRAY_SUFFIX     = "_fan_drawer"

	/** Transceiver status values **/
	SFP_STATUS_REMOVED  = "0"
	SFP_STATUS_INSERTED = "1"

	/** Upper-level URIs **/
	COMP     = "/openconfig-platform:components/component"
	COMP_ST  = "/openconfig-platform:components/component/state"
	COMP_CFG = "/openconfig-platform:components/component/config"

	/** Config container name **/
	COMP_CONFIG_NAME = "/openconfig-platform:components/component/config/name"

	/** Supported oc-platform component state URIs **/
	COMP_STATE_DESCR             = "/openconfig-platform:components/component/state/description"
	COMP_STATE_EMPTY             = "/openconfig-platform:components/component/state/empty"
	COMP_STATE_FIRM_VER          = "/openconfig-platform:components/component/state/firmware-version"
	COMP_STATE_HW_VER            = "/openconfig-platform:components/component/state/hardware-version"
	COMP_STATE_ID                = "/openconfig-platform:components/component/state/id"
	COMP_STATE_LOCATION          = "/openconfig-platform:components/component/state/location"
	COMP_STATE_INSTALL_POSITION  = "/openconfig-platform:components/component/state/install-position"
	COMP_STATE_INSTALL_COMPONENT = "/openconfig-platform:components/component/state/install-component"
	COMP_STATE_MFG_DATE          = "/openconfig-platform:components/component/state/mfg-date"
	COMP_STATE_MFG_NAME          = "/openconfig-platform:components/component/state/mfg-name"
	COMP_STATE_NAME              = "/openconfig-platform:components/component/state/name"
	COMP_STATE_OPER_STATUS       = "/openconfig-platform:components/component/state/oper-status"
	COMP_STATE_PART_NO           = "/openconfig-platform:components/component/state/part-no"
	COMP_STATE_REMOVABLE         = "/openconfig-platform:components/component/state/removable"
	COMP_STATE_MODEL_NAME        = "/openconfig-platform:components/component/state/model-name"
	COMP_STATE_SERIAL_NO         = "/openconfig-platform:components/component/state/serial-no"
	COMP_STATE_SW_VER            = "/openconfig-platform:components/component/state/software-version"
	COMP_STATE_TYPE              = "/openconfig-platform:components/component/state/type"
	COMP_STATE_PARENT            = "/openconfig-platform:components/component/state/parent"
	COMP_STATE_OC_FQ_NAME        = "/openconfig-platform:components/component/state/openconfig-pins-platform:fully-qualified-name"
	COMP_STATE_TEMP_CTR          = "/openconfig-platform:components/component/state/temperature"
	COMP_STATE_TEMP              = "/openconfig-platform:components/component/state/temperature/instant"
	COMP_STATE_TEMP_MAX          = "/openconfig-platform:components/component/state/temperature/max"
	COMP_STATE_TEMP_INTV         = "/openconfig-platform:components/component/state/temperature/interval"

	/** Supported PCIe component URIs **/
	COMP_STATE_PCIE                          = "/openconfig-platform:components/component/state/pcie"
	COMP_STATE_PCIE_CORRECTABLE_ERRORS       = "/openconfig-platform:components/component/state/pcie/correctable-errors"
	COMP_STATE_PCIE_CORRECTABLE_ERRORS_TOTAL = "/openconfig-platform:components/component/state/pcie/correctable-errors/total-errors"

	/** Supported Storage Component URIs **/
	COMP_STORAGE    = "/openconfig-platform:components/component/storage"
	COMP_STORAGE_ST = "/openconfig-platform:components/component/storage/state"

	/** Supported POWER SUPPLY URIs **/
	COMP_PS       = "/openconfig-platform:components/component/power-supply"
	COMP_PS_STATE = "/openconfig-platform:components/component/power-supply/state"

	/** Supported Fan URIs **/
	COMP_FAN       = "/openconfig-platform:components/component/fan"
	COMP_FAN_ST    = "/openconfig-platform:components/component/fan/state"
	COMP_FAN_SPEED = "/openconfig-platform:components/component/fan/state/openconfig-platform-fan:speed"

	/** Supported Xcvr URIs **/
	XCVR_BASE_PREFIX  = "/openconfig-platform:components/component/openconfig-platform-transceiver:transceiver"
	XCVR_BASE_STATE   = "/openconfig-platform:components/component/openconfig-platform-transceiver:transceiver/state"
	XCVR_FORM_FACTOR  = "/openconfig-platform:components/component/openconfig-platform-transceiver:transceiver/state/form-factor"
	XCVR_ETH_PMD      = "/openconfig-platform:components/component/openconfig-platform-transceiver:transceiver/state/ethernet-pmd"
	XCVR_BASE_PC      = "/openconfig-platform:components/component/openconfig-platform-transceiver:transceiver/physical-channels"
	XCVR_BASE_CHANNEL = "/openconfig-platform:components/component/openconfig-platform-transceiver:transceiver/physical-channels/channel"

	/** Support Temperature Sensor URIs **/
	TEMP_COMP    = "/openconfig-platform:components/component/state/temperature"
	TEMP_INSTANT = "/openconfig-platform:components/component/state/temperature/instant"

	/** Supported Integrated Circuit URIs **/
	COMP_IC         = "/openconfig-platform:components/component/integrated-circuit"
	COMP_IC_CFG     = "/openconfig-platform:components/component/integrated-circuit/config"
	COMP_IC_ST      = "/openconfig-platform:components/component/integrated-circuit/state"
	COMP_IC_PLC     = "/openconfig-platform:components/component/integrated-circuit/openconfig-platform-pipeline-counters:pipeline-counters"
	COMP_IC_MEM     = "/openconfig-platform:components/component/integrated-circuit/openconfig-platform-integrated-circuit:memory"
	COMP_IC_MEM_CPE = "/openconfig-platform:components/component/integrated-circuit/openconfig-platform-integrated-circuit:memory/state/corrected-parity-errors"
	COMP_IC_MEM_TPE = "/openconfig-platform:components/component/integrated-circuit/openconfig-platform-integrated-circuit:memory/state/total-parity-errors"

	/** Supported oc-platform component config URIs **/
	COMP_CFG_OC_FQ_NAME = "/openconfig-platform:components/component/config/openconfig-pins-platform:fully-qualified-name"

	/** Supported Firmware URIs **/
	FIRMWARE_CHASSIS                     = "/openconfig-platform:components/component/chassis"
	FIRMWARE_CHASSIS_ALARMS              = "/openconfig-platform:components/component/chassis/openconfig-pins-platform-chassis:alarms"
	FIRMWARE_CHASSIS_ALARMS_STATE        = "/openconfig-platform:components/component/chassis/openconfig-pins-platform-chassis:alarms/state"
	FIRMWARE_CHASSIS_ALARMS_STATE_STATUS = "/openconfig-platform:components/component/chassis/openconfig-pins-platform-chassis:alarms/state/status"
	FIRMWARE_CHASSIS_CONFIG              = "/openconfig-platform:components/component/chassis/config"
	FIRMWARE_CHASSIS_STATE               = "/openconfig-platform:components/component/chassis/state"
	FIRMWARE_CHASSIS_OC_PLATFORM         = "/openconfig-platform:components/component/chassis/state/openconfig-pins-platform-chassis:platform"
	FIRMWARE_CHASSIS_OC_BASE_MAC         = "/openconfig-platform:components/component/chassis/state/openconfig-pins-platform-chassis:base-mac-address"
	FIRMWARE_CHASSIS_OC_NUM_MAC          = "/openconfig-platform:components/component/chassis/state/openconfig-pins-platform-chassis:mac-address-pool-size"
	FIRMWARE_CHASSIS_OC_CPU_TYPE         = "/openconfig-platform:components/component/chassis/state/openconfig-pins-platform-chassis:cpu-type"

	/** Supported Software Module URIs **/
	COMP_SW_MOD                 = "/openconfig-platform:components/component/software-module"
	COMP_SW_MOD_ST              = "/openconfig-platform:components/component/software-module/state"
	SW_MODULE_STATE_MODULE_TYPE = "/openconfig-platform:components/component/software-module/state/openconfig-platform-software:module-type"

	SW_BOOT_LOADER_STATE_TYPE = "/openconfig-platform:components/component/boot-loader/state/openconfig-platform-boot-loader:type"

	/** Supported Port URIs **/
	COMP_PORT              = "/openconfig-platform:components/component/port"
	COMP_PORT_CFG          = "/openconfig-platform:components/component/port/config"
	COMP_PORT_ST           = "/openconfig-platform:components/component/port/state"
	PORT_CONFIG_OC_PORT_ID = "/openconfig-platform:components/component/port/config/openconfig-pins-platform-port:port-id"
	PORT_STATE_OC_PORT_ID  = "/openconfig-platform:components/component/port/state/openconfig-pins-platform-port:port-id"

	/** Supported Subcomponent URIs **/
	COMP_SUB = "/openconfig-platform:components/component/subcomponents"

	/** Supported OCS Port URIs **/
	COMP_PINS_OCS_PORT                                           = "/openconfig-platform:components/component/openconfig-pins-platform-ocs-port:pins-ocs-port"
	COMP_PINS_OCS_PORT_CFG                                       = "/openconfig-platform:components/component/openconfig-pins-platform-ocs-port:pins-ocs-port/config"
	COMP_PINS_OCS_PORT_CFG_ENABLED                               = "/openconfig-platform:components/component/openconfig-pins-platform-ocs-port:pins-ocs-port/config/enabled"
	COMP_PINS_OCS_PORT_CFG_ALIAS                                 = "/openconfig-platform:components/component/openconfig-pins-platform-ocs-port:pins-ocs-port/config/alias"
	COMP_PINS_OCS_PORT_ST                                        = "/openconfig-platform:components/component/openconfig-pins-platform-ocs-port:pins-ocs-port/state"
	COMP_PINS_OCS_PORT_ST_ENABLED                                = "/openconfig-platform:components/component/openconfig-pins-platform-ocs-port:pins-ocs-port/state/enabled"
	COMP_PINS_OCS_PORT_ST_ALIAS                                  = "/openconfig-platform:components/component/openconfig-pins-platform-ocs-port:pins-ocs-port/state/alias"
	COMP_PINS_OCS_PORT_ST_DIRECTION                              = "/openconfig-platform:components/component/openconfig-pins-platform-ocs-port:pins-ocs-port/state/direction"
	COMP_PINS_OCS_PORT_ST_INDEX                                  = "/openconfig-platform:components/component/openconfig-pins-platform-ocs-port:pins-ocs-port/state/index"
	COMP_PINS_OCS_PORT_ST_STATUS                                 = "/openconfig-platform:components/component/openconfig-pins-platform-ocs-port:pins-ocs-port/state/status"
	COMP_PINS_OCS_PORT_ST_CONNECTION                             = "/openconfig-platform:components/component/openconfig-pins-platform-ocs-port:pins-ocs-port/state/connection"
	COMP_PINS_OCS_PORT_ST_CONNECTION_PEER                        = "/openconfig-platform:components/component/openconfig-pins-platform-ocs-port:pins-ocs-port/state/connection/peer"
	COMP_PINS_OCS_PORT_ST_CONNECTION_CONNECTED                   = "/openconfig-platform:components/component/openconfig-pins-platform-ocs-port:pins-ocs-port/state/connection/connected"
	COMP_PINS_OCS_PORT_ST_CONNECTION_COUNTERS                    = "/openconfig-platform:components/component/openconfig-pins-platform-ocs-port:pins-ocs-port/state/connection/counters"
	COMP_PINS_OCS_PORT_ST_CONNECTION_COUNTERS_CORRECTABLE_ERRORS = "/openconfig-platform:components/component/openconfig-pins-platform-ocs-port:pins-ocs-port/state/connection/counters/correctable-error-count"
	COMP_PINS_OCS_PORT_ST_CONNECTION_COUNTERS_TOTAL_ERRORS       = "/openconfig-platform:components/component/openconfig-pins-platform-ocs-port:pins-ocs-port/state/connection/counters/total-error-count"
	COMP_PINS_OCS_PORT_ST_COUNTERS                               = "/openconfig-platform:components/component/openconfig-pins-platform-ocs-port:pins-ocs-port/state/counters"
	COMP_PINS_OCS_PORT_ST_COUNTERS_CORRECTABLE_ERRORS            = "/openconfig-platform:components/component/openconfig-pins-platform-ocs-port:pins-ocs-port/state/counters/correctable-error-count"
	COMP_PINS_OCS_PORT_ST_COUNTERS_TOTAL_ERRORS                  = "/openconfig-platform:components/component/openconfig-pins-platform-ocs-port:pins-ocs-port/state/counters/total-error-count"

	/** Supported Healthz URIs **/
	COMP_HEALTHZ = "/openconfig-platform:components/component/openconfig-platform-healthz:healthz"
	COMP_FAULTS  = "/openconfig-platform:components/component/openconfig-platform-healthz:healthz/openconfig-platform-healthz-fault:faults"

	/** XCVR Config Values **/
	FORCE_LOW_POWER = "force_low_power"
)

type PSUInfo struct {
	Capacity          string
	Enabled           bool
	Input_Current     ocbinds.Binary
	Input_Voltage     ocbinds.Binary
	Manufacturer      string
	Model_Name        string
	Output_Current    ocbinds.Binary
	Output_Power      ocbinds.Binary
	Output_Voltage    ocbinds.Binary
	Presence          bool
	Part_Number       string
	Serial_Number     string
	Status            bool
	Parent            string
	Location          string
	Volt_Type         string
	Install_Component string
	Install_Position  string
	Removable         bool
}

type FanInfo struct {
	hardwareRev      string
	isReplaceable    bool
	location         string
	mfgDate          string
	model            string
	parent           string
	partNo           string
	presence         bool
	pwm              string
	serial           string
	speed            string
	status           bool
	installComponent string
	installPosition  string
}

type XcvrLane struct {
	RxPowerLane string
	TxBiasLane  string
	TxPowerLane string
	TxDisable   string
}

type XcvrInfo struct {
	/* Most are strings since media sends 'N/A' when data is not available
	   Conversion will be done before sending along */
	Presence          bool
	Lanes             [XCVR_LANE_LIMIT]XcvrLane
	Temperature       string
	EthPmd            string
	Parent            string
	MfgName           string
	ModuleState       string
	MfgDate           string
	PartNo            string
	SerialNo          string
	HardwareRev       string
	Type              string
	LatestFirmwareVer string
	ForceLowPower     string
}

type PhysicalEntityInfo struct {
	Parent           string
	PositionInParent string
	InstallComponent string
}

type TempSensorInfo struct {
	Crit_High_Threshold string
	Crit_Low_Threshold  string
	Current             string
	High_Threshold      string
	Low_Threshold       string
	Name                string
	Description         string
	Warning_Status      string
	Timestamp           string
	EntityInfo          PhysicalEntityInfo
}

type SensorInfo struct {
	Unit                string
	Crit_High_Threshold string
	Crit_Low_Threshold  string
	Instant             string
	Name                string
	Warning_Status      string
	Timestamp           string
	Replaceable         string
	EntityInfo          PhysicalEntityInfo

	// Available, but not plumbed
	// High_Threshold      string
	// Low_Threshold       string
	// MaxValue       string
	// MinValue       string
}

type HumiditySensorInfo struct {
	Name             string
	Parent           string
	RelativeHumidity string
}

/*StorageInfo structure read from State DB*/
type StorageInfo struct {
	PartNo                         string
	SerialNo                       string
	Parent                         string
	OperStatus                     string
	Empty                          string
	IOErrors                       string
	Removable                      string
	WriteAmplificationFactor       string
	RawReadErrorRate               string
	ThroughputPerformance          string
	ReallocatedSectorCount         string
	PowerOnSeconds                 string
	SsdLifeLeft                    string
	PowerCycleCount                string
	UncorrectableSectorCountOnLine string
	NumPureSpare                   string
	NumInitialInvalidBlock         string
	SlcTotalEraseCount             string
	SlcMaxEraseCount               string
	SlcMinEraseCount               string
	SlcAvgEraseCount               string
	TlcTotalEraseCount             string
	TlcMaxEraseCount               string
	TlcMinEraseCount               string
	TlcAvgEraseCount               string
	WearLevelingCount              string
	ProgramFailCount               string
	EraseFailCount                 string
	PowerOffRetractCount           string
	Temperature                    string
	HardwareEccRecovered           string
	ReallocationEventCount         string
	UdmaCrcErrorsCount             string
	AvailableReservedSpace         string
	WriteSectorCount               string
	ReadSectorCount                string
	FlashWriteCount                string
}

type DiskCarrierInfo struct {
	Model_Name    string
	Name          string
	Parent        string
	Part_Number   string
	Serial_Number string
}

/*ICInfo structure read from State DB*/
type ICInfo struct {
	Node_Id                   string
	Name                      string
	Parent                    string
	QualifiedName             string
	UnrecoverableParityErrors uint64
	CorrectedParityErrors     uint64
}

/*PortInfo structure read from DB*/
type PortInfo struct {
	Name            string
	Parent          string
	PortID          string
	BreakoutMode    string
	FirmwareVersion string
}

/*FirmwareInfo Chassis structure read from State DB*/
type FirmwareInfo struct {
	AlarmStatus     bool
	Description     string
	Name            string
	ModelName       string
	FirmwareVersion string
	HardwareVersion string
	MfgDate         string
	OperStatus      string
	PartNo          string
	SerialNo        string
	QualifiedName   string
	BaseMac         string
	MacPoolSize     string
	CpuType         string
}

/*FpgaInfo structure read from State DB*/
type FpgaInfo struct {
	Name            string
	Parent          string
	Description     string
	MfgName         string
	FirmwareVersion string
	EntityInfo      PhysicalEntityInfo
	ResetCauses     []string
	ResetCount      string
}

/*SWCompInfo structure read from State DB*/
type SWCompInfo struct {
	Name            string
	SoftwareVersion string
	Parent          string
	OperStatus      string
	StorageSide     string
	Type            string
	BootLoaderType  string
}

/*HwSecurityModuleInfo structure read from State DB*/
type HwSecurityModuleInfo struct {
	Name                 string
	FirmwareVersion      string
	SerialNo             string
	SecPayloadEnforced   string
	PayloadVersion       string
	PayloadSignatureType string
	EntityInfo           PhysicalEntityInfo
}

type PowerSupplyInfo struct {
	Type                string
	Parent              string
	CommandedFrequency  string
	Frequency           string
	ManufacturerStatus  string
	StatusGpio          string
	Location            string
	Temperature         string
	TemperatureMax      string
	TemperatureInterval string
	Direction           string
	StatusVout          string
	CommendedVoltage    string
	Energy              string
	Current             string
	Voltage             string
	Power               string
	PeakPower           string
	PeakPowerInterval   string
	PeakVoltage         string
	PeakCurrent         string
	RailTemperature     string
}

type ControllerCardInfo struct {
	Model_Name        string
	Name              string
	Parent            string
	Part_Number       string
	Serial_Number     string
	Removable         string
	Mfg_Date          string
	Hardware_Version  string
	Install_Position  string
	Install_Component string
	OperStatus        string
	Empty             string
}

type LineCardInfo struct {
	Parent            string
	Serial_Number     string
	Part_Number       string
	Model_Name        string
	Removable         string
	Mfg_Date          string
	Hardware_Version  string
	Install_Position  string
	Install_Component string
	OperStatus        string
	Empty             string
	Type              string
}

type PinsOcsPortCounters struct {
	CorrectableErrorCount string
	TotalErrorCount       string
}

type PinsOcsPortConnection struct {
	Peer      string
	Connected string
	Counters  PinsOcsPortCounters
}

type PinsOcsPortInfo struct {
	// Component fields
	OperStatus       string
	InstallComponent string
	InstallPosition  string
	Parent           string
	Removable        string

	// OcsPortInfo config fields
	ConfigEnabled string
	ConfigAlias   string

	// OcsPortInfo state fields
	Enabled    string
	Alias      string
	Direction  string
	Index      string
	Status     string
	Connection PinsOcsPortConnection
	Counters   PinsOcsPortCounters
}

type PathType int

const (
	/* Represents all paths under /components/component */
	AllPaths PathType = iota
	/* Represents all paths under /components/component/config */
	ConfigPaths
	/* Represents all paths under /components/component/state */
	StatePaths
	/* Represents all paths under a component subtree, e.g.
	 * /components/component/port or /components/component/fan */
	AllCompPaths
	/* Represents some set of paths (or one leaf) under a component subtree, e.g.
	 * /components/component/port or /components/component/fan */
	CompSubtreeOrSinglePath
	/* Represents all config paths under a component subtree, e.g.
	 * /components/component/pins-ocs-port/config */
	CompConfigPaths
	/* Represents all state paths under a component subtree, e.g.
	 * /components/component/pins-ocs-port/state */
	CompStatePaths
	/* Represents a path to a specific leaf */
	SingularPath
	/* Represents all paths under /components/component/power-supply/rails/rail */
	RailPaths
)

type LeafType int

const (
	/* Represents any leaf within /components/component/state */
	StateLeaf LeafType = iota
	/* Represents any leaf within /components/component/config */
	ConfigLeaf
	/* Represents any leaf within /components/component/<subtree>/state, e.g.
	 * /components/component/pins-ocs-port/state  */
	CompStateLeaf
	/* Represents any leaf within /components/component/<subtree>/config, e.g.
	 * /components/component/pins-ocs-port/config */
	CompConfigLeaf
)

func (pt PathType) String() string {
	switch pt {
	case AllPaths:
		return "AllPaths"
	case AllCompPaths:
		return "AllComponentPaths"
	case ConfigPaths:
		return "ConfigPaths"
	case StatePaths:
		return "StatePaths"
	case CompSubtreeOrSinglePath:
		return "CompSubtreeOrSinglePath"
	case CompConfigPaths:
		return "CompConfigPaths"
	case CompStatePaths:
		return "CompStatePaths"
	case SingularPath:
		return "SingularPath"
	case RailPaths:
		return "RailPaths"
	}
	return fmt.Sprintf("%s", pt)
}

const (
	LaneIndex0 uint16 = iota
	LaneIndex1
	LaneIndex2
	LaneIndex3
	LaneIndex4
	LaneIndex5
	LaneIndex6
	LaneIndex7
)

var dbToYangEthPmdMap = map[string]ocbinds.E_OpenconfigTransportTypes_ETHERNET_PMD_TYPE{
	"10G_LRM":     ocbinds.OpenconfigTransportTypes_ETHERNET_PMD_TYPE_ETH_10GBASE_LRM,
	"10G_LR":      ocbinds.OpenconfigTransportTypes_ETHERNET_PMD_TYPE_ETH_10GBASE_LR,
	"10G_ZR":      ocbinds.OpenconfigTransportTypes_ETHERNET_PMD_TYPE_ETH_10GBASE_ZR,
	"10G_ER":      ocbinds.OpenconfigTransportTypes_ETHERNET_PMD_TYPE_ETH_10GBASE_ER,
	"10G_SR":      ocbinds.OpenconfigTransportTypes_ETHERNET_PMD_TYPE_ETH_10GBASE_SR,
	"40G_CR4":     ocbinds.OpenconfigTransportTypes_ETHERNET_PMD_TYPE_ETH_40GBASE_CR4,
	"40G_SR4":     ocbinds.OpenconfigTransportTypes_ETHERNET_PMD_TYPE_ETH_40GBASE_SR4,
	"40G_LR4":     ocbinds.OpenconfigTransportTypes_ETHERNET_PMD_TYPE_ETH_40GBASE_LR4,
	"40G_ER4":     ocbinds.OpenconfigTransportTypes_ETHERNET_PMD_TYPE_ETH_40GBASE_ER4,
	"40G_PSM4":    ocbinds.OpenconfigTransportTypes_ETHERNET_PMD_TYPE_ETH_40GBASE_PSM4,
	"4X10G_LR":    ocbinds.OpenconfigTransportTypes_ETHERNET_PMD_TYPE_ETH_4X10GBASE_LR,
	"4X10G_SR":    ocbinds.OpenconfigTransportTypes_ETHERNET_PMD_TYPE_ETH_4X10GBASE_SR,
	"100G_AOC":    ocbinds.OpenconfigTransportTypes_ETHERNET_PMD_TYPE_ETH_100G_AOC,
	"100G_ACC":    ocbinds.OpenconfigTransportTypes_ETHERNET_PMD_TYPE_ETH_100G_ACC,
	"100G_SR10":   ocbinds.OpenconfigTransportTypes_ETHERNET_PMD_TYPE_ETH_100GBASE_SR10,
	"100G_SR4":    ocbinds.OpenconfigTransportTypes_ETHERNET_PMD_TYPE_ETH_100GBASE_SR4,
	"100G_LR4":    ocbinds.OpenconfigTransportTypes_ETHERNET_PMD_TYPE_ETH_100GBASE_LR4,
	"100G_ER4":    ocbinds.OpenconfigTransportTypes_ETHERNET_PMD_TYPE_ETH_100GBASE_ER4,
	"100G_CWDM4":  ocbinds.OpenconfigTransportTypes_ETHERNET_PMD_TYPE_ETH_100GBASE_CWDM4,
	"100G_CLR4":   ocbinds.OpenconfigTransportTypes_ETHERNET_PMD_TYPE_ETH_100GBASE_CLR4,
	"100G_PSM4":   ocbinds.OpenconfigTransportTypes_ETHERNET_PMD_TYPE_ETH_100GBASE_PSM4,
	"100G_CR4":    ocbinds.OpenconfigTransportTypes_ETHERNET_PMD_TYPE_ETH_100GBASE_CR4,
	"100G_FR":     ocbinds.OpenconfigTransportTypes_ETHERNET_PMD_TYPE_ETH_100GBASE_FR,
	"400G_ZR":     ocbinds.OpenconfigTransportTypes_ETHERNET_PMD_TYPE_ETH_400GBASE_ZR,
	"400G_LR4":    ocbinds.OpenconfigTransportTypes_ETHERNET_PMD_TYPE_ETH_400GBASE_LR4,
	"400G_FR4":    ocbinds.OpenconfigTransportTypes_ETHERNET_PMD_TYPE_ETH_400GBASE_FR4,
	"400G_LR8":    ocbinds.OpenconfigTransportTypes_ETHERNET_PMD_TYPE_ETH_400GBASE_LR8,
	"400G_DR4":    ocbinds.OpenconfigTransportTypes_ETHERNET_PMD_TYPE_ETH_400GBASE_DR4,
	"400G_DR4+":   ocbinds.OpenconfigTransportTypes_ETHERNET_PMD_TYPE_ETH_400GBASE_DR4,
	"400G_PSM4":   ocbinds.OpenconfigTransportTypes_ETHERNET_PMD_TYPE_ETH_400GMSA_PSM4,
	"PMD_UNKNOWN": ocbinds.OpenconfigTransportTypes_ETHERNET_PMD_TYPE_ETH_UNDEFINED,
}

var dbToYangBootLoaderTypeMap = map[string]ocbinds.E_OpenconfigPlatformBootLoader_BOOT_LOADER_BASE{
	"GRUB":         ocbinds.OpenconfigPlatformBootLoader_BOOT_LOADER_BASE_BOOT_LOADER_GRUB,
	"ONIE":         ocbinds.OpenconfigPlatformBootLoader_BOOT_LOADER_BASE_BOOT_LOADER_ONIE,
	"UBOOT":        ocbinds.OpenconfigPlatformBootLoader_BOOT_LOADER_BASE_BOOT_LOADER_UBOOT,
	"SYSTEMD_BOOT": ocbinds.OpenconfigPlatformBootLoader_BOOT_LOADER_BASE_BOOT_LOADER_SYSTEMD_BOOT,
	"LINUXBOOT":    ocbinds.OpenconfigPlatformBootLoader_BOOT_LOADER_BASE_BOOT_LOADER_LINUXBOOT,
}

func operStatusFromString(status string) (ocbinds.E_OpenconfigPlatformTypes_COMPONENT_OPER_STATUS, error) {
	switch strings.ToLower(status) {
	case "active":
		return ocbinds.OpenconfigPlatformTypes_COMPONENT_OPER_STATUS_ACTIVE, nil
	case "inactive":
		return ocbinds.OpenconfigPlatformTypes_COMPONENT_OPER_STATUS_INACTIVE, nil
	case "disabled":
		return ocbinds.OpenconfigPlatformTypes_COMPONENT_OPER_STATUS_DISABLED, nil
	default:
		return ocbinds.OpenconfigPlatformTypes_COMPONENT_OPER_STATUS_DISABLED, fmt.Errorf("unknown oper-status: %s", status)
	}
}

func formFactorTypeFromString(ft string) ocbinds.E_OpenconfigTransportTypes_TRANSCEIVER_FORM_FACTOR_TYPE {
	switch {
	case ft == "N/A" || ft == "":
		return ocbinds.OpenconfigTransportTypes_TRANSCEIVER_FORM_FACTOR_TYPE_UNSET
	case strings.HasPrefix(ft, "Unknown"):
		return ocbinds.OpenconfigTransportTypes_TRANSCEIVER_FORM_FACTOR_TYPE_OTHER
	case strings.HasPrefix(ft, "SFP"):
		return ocbinds.OpenconfigTransportTypes_TRANSCEIVER_FORM_FACTOR_TYPE_SFP
	case ft == "XFP":
		return ocbinds.OpenconfigTransportTypes_TRANSCEIVER_FORM_FACTOR_TYPE_XFP
	case ft == "X2":
		return ocbinds.OpenconfigTransportTypes_TRANSCEIVER_FORM_FACTOR_TYPE_X2
	case ft == "QSFP":
		return ocbinds.OpenconfigTransportTypes_TRANSCEIVER_FORM_FACTOR_TYPE_QSFP
	case strings.HasPrefix(ft, "QSFP+"):
		return ocbinds.OpenconfigTransportTypes_TRANSCEIVER_FORM_FACTOR_TYPE_QSFP_PLUS
	case strings.HasPrefix(ft, "QSFP28"):
		return ocbinds.OpenconfigTransportTypes_TRANSCEIVER_FORM_FACTOR_TYPE_QSFP28
	case strings.HasPrefix(ft, "OSFP") || strings.HasPrefix(ft, "QSFP-DD"):
		return ocbinds.OpenconfigTransportTypes_TRANSCEIVER_FORM_FACTOR_TYPE_OSFP
	default:
		return ocbinds.OpenconfigTransportTypes_TRANSCEIVER_FORM_FACTOR_TYPE_UNSET
	}
}

func ethPmdTypeFromString(dbVal string) ocbinds.E_OpenconfigTransportTypes_ETHERNET_PMD_TYPE {
	ep := ocbinds.OpenconfigTransportTypes_ETHERNET_PMD_TYPE_ETH_UNDEFINED
	if dbVal == "" {
		log.V(lvl.ERROR).Info("transceiver ethernet-pmd not present in State DB")
		return ep
	}
	val, ok := dbToYangEthPmdMap[dbVal]
	if !ok {
		log.V(lvl.ERROR).Info("transceiver ethernet-pmd invalid field value in State DB: " + dbVal)
		return ep
	}
	return val
}

func String2Bool(b *string, dflt bool) *bool {
	v, err := strconv.ParseBool(*b)
	if err != nil {
		log.V(lvl.ERROR).Info(err)
		return &dflt
	}
	return &v
}

func String2Uint(b string, dflt uint64) *uint64 {
	v, err := strconv.ParseUint(b, 10, 64)
	if err != nil {
		if b != "" {
			log.V(lvl.DEBUG).Infof("string conversion failure for %v; err = %v", b, err)
		}
		return &dflt
	}
	return &v
}

func String2Uint32(b string, dflt uint32) *uint32 {
	v, err := strconv.ParseUint(b, 10, 32)
	if err != nil {
		if b != "" {
			log.V(lvl.DEBUG).Infof("string conversion failure for %v; err = %v", b, err)
		}
		return &dflt
	}
	vv := uint32(v)
	return &vv
}

func String2Float(b string, dflt float64) *float64 {
	v, err := strconv.ParseFloat(b, 64)
	if err != nil {
		if b != "" {
			log.V(lvl.DEBUG).Infof("string conversion failure for %v; err = %v", b, err)
		}
		return &dflt
	}
	return &v
}

func FirstNonEmptyOrNil(vals ...string) *string {
	for _, val := range vals {
		if val != "" {
			return &val
		}
	}
	return nil
}
func StringWithDefault(val *string, dval string) *string {
	if *val != "" {
		return val
	}
	return &dval
}
func NilIfStringEmpty(val *string) *string {
	if *val != "" {
		return val
	}
	return nil
}

func float32StrTo4Bytes(s string) ([]byte, error) {
	var data []byte
	float64val, err := strconv.ParseFloat(s, 32)
	if err != nil {
		log.V(lvl.DEBUG).Info("Error converting string to float32")
		return data, err
	}
	data = make([]byte, 4)
	/* Using Big Endian (network-order) to pack and unpack data
	 * IMPORTANT: REST server will do a b64 encode before sending the output
	 * The example of encode and decode can be found: https://go.dev/play/p/pr79oRzffY0"
	 * Json test result needs to be modified to b64 decoded result
	 */
	binary.BigEndian.PutUint32(data, math.Float32bits(float32(float64val)))
	return data, err
}

func hexaNumberToUint64(hexaString string) (uint64, error) {
	// replace 0x or 0X with empty String
	numberStr := strings.Replace(hexaString, "0x", "", -1)
	numberStr = strings.Replace(numberStr, "0X", "", -1)
	hexVal, err := strconv.ParseUint(numberStr, 16, 64)
	return hexVal, err
}

func uint64To8Bytes(hexa uint64) []byte {
	data := make([]byte, 8)
	binary.BigEndian.PutUint64(data, hexa)
	return data
}

func hexaNumberToUint32(hexaString string) (uint32, error) {
	// replace 0x or 0X with empty String
	numberStr := strings.Replace(hexaString, "0x", "", -1)
	numberStr = strings.Replace(numberStr, "0X", "", -1)
	hexVal, err := strconv.ParseUint(numberStr, 16, 32)
	if err != nil {
		return uint32(0), err
	}
	return uint32(hexVal), nil
}

func uint32To4Bytes(hexa uint32) []byte {
	var data = make([]byte, 4)
	binary.BigEndian.PutUint32(data, hexa)
	return data
}

func convertUTF8EndcodedString(s string) string {
	if !utf8.ValidString(s) {
		v := make([]rune, 0, len(s))
		for i, r := range s {
			if r == utf8.RuneError {
				_, size := utf8.DecodeRuneInString(s[i:])
				if size == 1 {
					continue
				}
			}
			v = append(v, r)
		}
		return string(v)
	}
	return s
}

/* This function converts the timestamp stored in the dB to the pattern accepted
 * by the IETF timestamp pattern specified in the YANG model
 */
func convertToIetfTime(time string) string {
	time = time[:8] + "T" + time[9:]
	time = time[:4] + "-" + time[4:6] + "-" + time[6:8] + time[8:] + "Z"
	return time
}

// reformatMfgDate reformats a date from "MM-DD-YYYY..." to "YYYY-MM-DD".
// It returns an empty string if the input string is not long enough.
func reformatMfgDate(date string) string {
	if len(date) <= 10 {
		return ""
	}
	return date[6:10] + "-" + date[0:2] + "-" + date[3:5]
}

func test_if_available(s string) bool {
	return ((s != "") && (s != "N/A") && (s != "n/a"))
}

func keyInDbTable(tableName, key string, d *db.DB) bool {
	if d == nil {
		return false
	}
	log.V(lvl.DEBUG).Infof("keyInDbTable: tableName=%s, key=%s, db=%s", tableName, key, d.Name())
	keys, err := d.GetKeysPattern(&db.TableSpec{Name: tableName}, db.Key{Comp: []string{key}})
	if err != nil {
		return false
	}
	return len(keys) > 0
}

/* Get a list of all table entries available */
func getAllTableEntries(d *db.DB, tblName string, key string) ([]string, error) {
	if tblName == "" || key == "" {
		return nil, errors.New("getAllTableEntries: empty table name or key.")
	}
	keyList, err := d.GetKeysPattern(&(db.TableSpec{Name: tblName}), db.Key{Comp: []string{key}})
	if err != nil {
		return nil, err
	}
	var ret []string
	for _, v := range keyList {
		if len(v.Comp) == 0 {
			continue
		}
		ret = append(ret, strings.Join(v.Comp, d.Opts.KeySeparator))
	}
	return ret, nil
}

type componentType int64

const (
	CompTypeInvalid componentType = iota
	CompTypePsu
	CompTypeFan
	CompTypeFanTray
	CompTypeStorage
	CompTypeDiskCarrier
	CompTypeXcvr
	CompTypeTemp
	CompTypeIC
	CompTypeChassis
	CompTypeNWStack
	CompTypeOS
	CompTypeBootLoader
	CompTypePort
	CompTypePcie
	CompTypePowerSupplyVR
	CompTypePowerSupplyPB
	CompTypePowerSupplyPH
	CompTypePowerSupplyPSEQ
	CompTypeControllerCard
	CompTypeLineCard
	CompTypeSystemEeprom
)

func (ct componentType) String() string {
	switch ct {
	case CompTypeInvalid:
		return "CompTypeInvalid"
	case CompTypePsu:
		return "CompTypePsu"
	case CompTypeFan:
		return "CompTypeFan"
	case CompTypeFanTray:
		return "CompTypeFanTray"
	case CompTypeStorage:
		return "CompTypeStorage"
	case CompTypeDiskCarrier:
		return "CompTypeDiskCarrier"
	case CompTypeXcvr:
		return "CompTypeXcvr"
	case CompTypeTemp:
		return "CompTypeTemp"
	case CompTypeIC:
		return "CompTypeIC"
	case CompTypeChassis:
		return "CompTypeChassis"
	case CompTypeNWStack:
		return "CompTypeNWStack"
	case CompTypeOS:
		return "CompTypeOS"
	case CompTypeBootLoader:
		return "CompTypeBootLoader"
	case CompTypePort:
		return "CompTypePort"
	case CompTypePcie:
		return "CompTypePcie"
	case CompTypePowerSupplyVR:
		return "CompTypePowerSupplyVR"
	case CompTypePowerSupplyPB:
		return "CompTypePowerSupplyPB"
	case CompTypePowerSupplyPH:
		return "CompTypePowerSupplyPH"
	case CompTypePowerSupplyPSEQ:
		return "CompTypePowerSupplyPSEQ"
	case CompTypeControllerCard:
		return "CompTypeControllerCard"
	case CompTypeLineCard:
		return "CompTypeLineCard"
	case CompTypeSystemEeprom:
		return "CompTypeSystemEeprom"
	}
	return fmt.Sprintf("%s", ct)
}

var compTblMap = map[componentType][]string{
	CompTypePsu:             {PSU_TBL, "PSU*"},
	CompTypeFan:             {FAN_TBL, "*"},
	CompTypeFanTray:         {FAN_TRAY_TBL, "*"},
	CompTypeStorage:         {STORAGE_INFO, STORAGE_NAME_PREFIX},
	CompTypeDiskCarrier:     {DISK_CARRIER_TBL, "*"},
	CompTypeXcvr:            {TRANSCEIVER_STATUS, XCVR_KEY_PREFIX + "*"},
	CompTypeTemp:            {TEMP_TBL, "*"},
	CompTypeIC:              {NODE_CFG_TBL, IC_NAME_PREFIX + "*"},
	CompTypeChassis:         {CHASSIS_TBL, CHASSIS_PREFIX + "*"},
	CompTypeNWStack:         {SW_COMP_TBL, NW_STACK_PREFIX + "*"},
	CompTypeOS:              {SW_COMP_TBL, OS_PREFIX + "*"},
	CompTypeBootLoader:      {SW_COMP_TBL, BOOTL_PREFIX + "*"},
	CompTypePort:            {PORT_BREAKOUT, ""},
	CompTypePcie:            {PCIE_TBL, "*"},
	CompTypePowerSupplyVR:   {POWER_INFO_TBL, VR_PREFIX + "*"},
	CompTypePowerSupplyPB:   {POWER_INFO_TBL, PB_PREFIX + "*"},
	CompTypePowerSupplyPH:   {POWER_INFO_TBL, PH_PREFIX + "*"},
	CompTypePowerSupplyPSEQ: {POWER_INFO_TBL, PSEQ_PREFIX + "*"},
	CompTypeControllerCard:  {CONTROLLER_CARD_TBL, "*"},
	CompTypeLineCard:        {LINECARD_TBL, "*"},
}

func getCompType(name string, d *db.DB) componentType {
	compType, err := getCompTypeByName(name)
	if err == nil {
		return compType
	}
	// For some types, derivation doesn't rely on a prefix check, but a table scan.
	componentTableMap := map[componentType]string{
		CompTypeStorage:        STORAGE_INFO,
		CompTypeDiskCarrier:    DISK_CARRIER_TBL,
		CompTypePcie:           PCIE_TBL,
		CompTypeControllerCard: CONTROLLER_CARD_TBL,
		CompTypeLineCard:       LINECARD_TBL,
		CompTypeTemp:           TEMP_TBL,
	}
	for compType, tblName := range componentTableMap {
		if keyInDbTable(tblName, name, d) {
			return compType
		}
	}
	return CompTypeInvalid
}

func getCompTypeByName(compName string) (componentType, error) {
	switch {
	case compName == SYS_EEPROM_NAME:
		return CompTypeSystemEeprom, nil
	case validPsuName(&compName):
		return CompTypePsu, nil
	case validFanName(&compName):
		return CompTypeFan, nil
	case validFanTrayName(compName):
		return CompTypeFanTray, nil
	case validXcvrName(&compName):
		return CompTypeXcvr, nil
	case validTempSensorName(&compName) || validCpuTempSensorName(compName):
		return CompTypeTemp, nil
	case validICName(&compName):
		return CompTypeIC, nil
	case strings.HasPrefix(compName, CHASSIS_PREFIX):
		return CompTypeChassis, nil
	case validSWCompName(&compName, NW_STACK_PREFIX):
		return CompTypeNWStack, nil
	case validSWCompName(&compName, OS_PREFIX):
		return CompTypeOS, nil
	case strings.HasPrefix(compName, BOOTL_PREFIX):
		return CompTypeBootLoader, nil
	case len(platform.InterfaceNameFromPort(compName)) > 0:
		return CompTypePort, nil
	case validPSCompName(&compName, VR_PREFIX):
		return CompTypePowerSupplyVR, nil
	case validPSCompName(&compName, PB_PREFIX):
		return CompTypePowerSupplyPB, nil
	case validPSCompName(&compName, PH_PREFIX):
		return CompTypePowerSupplyPH, nil
	case validPSCompName(&compName, PSEQ_PREFIX):
		return CompTypePowerSupplyPSEQ, nil
	default:
		return CompTypeInvalid, fmt.Errorf("component name %s did not match with supported types.", compName)
	}
}

func init() {
	XlateFuncBind("DbToYangPath_pfm_components_path_xfmr", DbToYangPath_pfm_components_path_xfmr)
	XlateFuncBind("Subscribe_pfm_components_xfmr", Subscribe_pfm_components_xfmr)
	XlateFuncBind("YangToDb_pfm_components_xfmr", YangToDb_pfm_components_xfmr)
	XlateFuncBind("DbToYang_pfm_components_xfmr", DbToYang_pfm_components_xfmr)
}

var DbToYangPath_pfm_components_path_xfmr PathXfmrDbToYangFunc = func(inParams XfmrDbToYgPathParams) error {
	rootPath := COMP

	log.V(lvl.DEBUG).Infof("DbToYangPath_pfm_path_xfmr: inParams: %#v", inParams)

	if len(inParams.tblKeyComp) == 0 {
		return fmt.Errorf("Invalid tblKeyCom for pfm path xmfr:%v", inParams.tblKeyComp)
	}

	tblKey := inParams.tblKeyComp[0]
	var subKey string
	if len(inParams.tblKeyComp) == 2 {
		subKey = inParams.tblKeyComp[1]
	}
	switch inParams.tblName {
	case PORT_BREAKOUT:
		// PORT_BREAKOUT uses the same db and db key as CompTypeXcvr, but with different yang key
		var portName string
		if inParams.db != nil {
			// Try to get port name from DB first.
			if entry, err := inParams.db.GetEntry(&db.TableSpec{Name: PORT_BREAKOUT}, db.Key{Comp: inParams.tblKeyComp}); err == nil {
				portName = entry.Get("name")
			}
		}
		if portName == "" {
			log.V(lvl.DEBUG).Info("No port name found for port breakout:", tblKey)
			// Try to get port name from platform.json.
			var err error
			if portName, err = platform.PortNameFromInterface(tblKey); err != nil {
				log.V(lvl.WARNING).Info("No port name found for port breakout:", tblKey, err)
				return err
			}
		}
		inParams.ygPathKeys[rootPath+"/name"] = portName

	case BREAKOUT_TBL:
		portName, err := platform.PortNameFromInterface(tblKey)
		if err != nil {
			log.V(lvl.WARNING).Infof("No port name found for interface \"%s\", error %v", tblKey, err)
			return err
		}
		inParams.ygPathKeys[rootPath+"/name"] = portName
	case POWER_INFO_TBL:
		inParams.ygPathKeys[rootPath+"/name"] = tblKey
	case "BREAKOUT_PORT_XCVR_CFG":
		inParams.ygPathKeys[rootPath+"/name"] = tblKey
		if subKey != "" {
			inParams.ygPathKeys[rootPath+"/subcomponents/subcomponent/name"] = subKey
		}
	case FAULT_INFO_TBL:
		inParams.ygPathKeys[rootPath+"/name"] = tblKey
		if subKey != "" {
			inParams.ygPathKeys[rootPath+"/openconfig-platform-healthz:healthz/openconfig-platform-healthz-fault:faults/fault/symptom"] = subKey
		}
	case "REMEDIATION_INFO":
		if subKey != "" {
			inParams.ygPathKeys[rootPath+"/openconfig-platform-healthz:healthz/openconfig-platform-healthz-fault:faults/fault/remediations/remediation/index"] = subKey
		}
		if inParams.db == nil {
			return fmt.Errorf("inParams.db is nil in pfm path xmfr!")
		}
		var compName, repair_strategy string
		// Get target name from REMEDIATION_INFO.
		if entry, err := inParams.db.GetEntry(&db.TableSpec{Name: "REMEDIATION_INFO"}, db.Key{Comp: inParams.tblKeyComp}); err == nil {
			compName = entry.Get("target")
		}
		inParams.ygPathKeys[rootPath+"/name"] = compName
		// Get symptom from FAULT_INFO.
		if keys, err := inParams.db.GetKeysPattern(&db.TableSpec{Name: FAULT_INFO_TBL}, db.Key{Comp: []string{compName, "*"}}); err == nil {
			for _, key := range keys {
				if key.Len() != 2 {
					log.V(lvl.DEBUG).Info("Not a valid FAULT_INFO table")
					continue
				}
				if entry, err := inParams.db.GetEntry(&db.TableSpec{Name: FAULT_INFO_TBL}, key); err == nil {
					repair_strategy = entry.Get("repair_strategy")
					if repair_strategy == tblKey {
						inParams.ygPathKeys[rootPath+"/openconfig-platform-healthz:healthz/openconfig-platform-healthz-fault:faults/fault/symptom"] = key.Comp[1]
					}
				}
			}
		}
	default:
		inParams.ygPathKeys[rootPath+"/name"] = tblKey
	}

	log.V(lvl.DEBUG).Info("DbToYangPath_pfm_path_xfmr:- params.ygPathKeys: ", inParams.ygPathKeys)

	return nil
}

var Subscribe_pfm_components_xfmr SubTreeXfmrSubscribe = func(inParams XfmrSubscInParams) (XfmrSubscOutParams, error) {
	var result XfmrSubscOutParams
	key := NewPathInfo(inParams.uri).Var("name")

	log.V(lvl.DEBUG).Infof("+++ Subscribe_pfm_components_xfmr uri (%v) key(%s) mode(%v) +++", inParams.uri, key, inParams.subscProc)
	log.V(lvl.DEBUG).Infof("+++ Subscribe_pfm_components_xfmr requestUri (%v) +++", inParams.requestURI)

	pathInfo := NewPathInfo(inParams.requestURI)
	targetUriPath, err := getYangPathFromUri(pathInfo.Path)
	if err != nil {
		return result, err
	}

	if key == "" || strings.Contains(key, "_sensor") || key == SYS_EEPROM_NAME {
		/* No need to verify dB data if we are requesting ALL components, a
		   sensor, or System Eeprom.  The EEPROM_INFO table stores each attribute
		   as its own row (keyed by index) with no "System Eeprom" key, so mark it
		   as a virtual table to tell the framework to skip DB key validation and
		   go straight to the subtree transformer. */
		result.isVirtualTbl = true
		return result, err
	}

	if inParams.subscProc == TRANSLATE_EXISTS {
		return translateExists(inParams, key)
	}
	if inParams.subscProc == TRANSLATE_SUBSCRIBE {
		return translateSubscribe(inParams, key, targetUriPath)
	}

	return result, err
}

/* Given a URI for a subscription, return a list of component types which apply
 * to it.  For example a URI of "/components/component/port" would return
 * [CompTypePort] while a URI of "/components/component/state/software-version"
 * would return a list of all component types which report software version. */
func compTypesForSubscriptionUri(uri string) []componentType {
	cTypes := []componentType{}
	if strings.HasPrefix(uri, "/openconfig-platform:components/component/chassis") {
		cTypes = []componentType{CompTypeChassis}
	} else if strings.HasPrefix(uri, "/openconfig-platform:components/component/port") {
		cTypes = []componentType{CompTypePort}
	} else if strings.HasPrefix(uri, "/openconfig-platform:components/component/power-supply") {
		cTypes = []componentType{CompTypePsu, CompTypePowerSupplyVR, CompTypePowerSupplyPB, CompTypePowerSupplyPH, CompTypePowerSupplyPSEQ}
	} else if strings.HasPrefix(uri, "/openconfig-platform:components/component/fan") {
		cTypes = []componentType{CompTypeFan}
	} else if strings.HasPrefix(uri, "/openconfig-platform:components/component/storage") {
		cTypes = []componentType{CompTypeStorage}
	} else if strings.HasPrefix(uri, "/openconfig-platform:components/component/integrated-circuit") {
		cTypes = []componentType{CompTypeIC}
	} else if strings.HasPrefix(uri, "/openconfig-platform:components/component/software-module") {
		cTypes = []componentType{CompTypeNWStack, CompTypeOS}
	} else if strings.HasPrefix(uri, "/openconfig-platform:components/component/boot-loader") {
		cTypes = []componentType{CompTypeBootLoader}
	} else if strings.HasPrefix(uri, "/openconfig-platform:components/component/state/oper-status") {
		cTypes = []componentType{CompTypeFan, CompTypeNWStack, CompTypeOS}
	} else if strings.HasPrefix(uri, "/openconfig-platform:components/component/oc-transceiver:transceiver") {
		cTypes = []componentType{CompTypeXcvr}
	} else if strings.HasPrefix(uri, COMP_STATE_PARENT) || strings.HasPrefix(uri, COMP_STATE_INSTALL_POSITION) || strings.HasPrefix(uri, COMP_STATE_INSTALL_COMPONENT) || strings.HasPrefix(uri, COMP_STATE_REMOVABLE) {
		/* Everything except chassis */
		cTypes = []componentType{CompTypePsu, CompTypeFan, CompTypeFanTray, CompTypeStorage, CompTypeXcvr, CompTypeTemp, CompTypeIC, CompTypeNWStack, CompTypeOS, CompTypeBootLoader, CompTypePort, CompTypePcie, CompTypePowerSupplyVR, CompTypePowerSupplyPB, CompTypePowerSupplyPH, CompTypePowerSupplyPSEQ, CompTypeControllerCard, CompTypeLineCard}
	} else if strings.HasPrefix(uri, "/openconfig-platform:components/component/state/software-version") {
		cTypes = []componentType{CompTypeNWStack, CompTypeOS, CompTypeBootLoader}
	}
	return cTypes
}

/* Helper for the main subscribe transformer handling the TRANSLATE_EXISTS case. */
func translateExists(inParams XfmrSubscInParams, key string) (XfmrSubscOutParams, error) {
	var result XfmrSubscOutParams
	dbNum := db.StateDB
	d := inParams.dbs[dbNum]
	if d == nil {
		dbNum := db.ConfigDB
		d = inParams.dbs[dbNum]
	}
	if d == nil {
		return result, fmt.Errorf("translateExists: No usable DB client in inParams (checked %v and %v)", db.StateDB.Name(), db.ConfigDB.Name())
	}
	compType := getCompType(key, d)
	if compType == CompTypeInvalid {
		return result, nil
	}
	tblInfo, ok := compTblMap[compType]
	if !ok {
		return result, errors.New("table not found.")
	}
	tblName := tblInfo[0]
	tblKey := key
	switch compType {
	case CompTypeIC:
		dbNum = db.ConfigDB
	case CompTypePort:
		// Convert the port-name (e.g. 1/1) to interface name (Ethernet1/1/1) since that is the DB key
		if ifName := platform.InterfaceNameFromPort(key); len(ifName) > 0 {
			tblKey = ifName
		}
		dbNum = db.ConfigDB
		// Should we just update compTblMap for CompTypePort to use this table???
		tblName = BREAKOUT_TBL
	case CompTypePowerSupplyVR, CompTypePowerSupplyPB, CompTypePowerSupplyPH, CompTypePowerSupplyPSEQ:
		// In the case of a power component which only has DB entries for rails and none for
		// the top level component (e.g. POWER_INFO|HotSwap_1|railA, POWER_INFO|HotSwap_1|railB,
		// but no entry for POWER_INFO|HotSwap) we do not want the framework to skip the key because
		// POWER_INFO|HotSwap doesn't exist.  Mark it as virtual to tell the framework to skip
		// that validation.
		result.isVirtualTbl = true
	}
	result.dbDataMap = RedisDbSubscribeMap{dbNum: {tblName: {tblKey: {}}}}
	log.V(lvl.DEBUG).Infof("+++ Subscribe_pfm_components_xfmr result: %v %v %v +++", dbNum, tblName, tblKey)
	return result, nil
}

/* Helper for the main subscribe transformer handling the TRANSLATE_SUBSCRIBE case. */
func translateSubscribe(inParams XfmrSubscInParams, key, targetUriPath string) (XfmrSubscOutParams, error) {
	var result XfmrSubscOutParams
	result.dbDataMap = make(RedisDbSubscribeMap)
	/* Handle TRANSLATE_SUBSCRIBE by expanding the wildcard yang key to a set of
	 * DB tables and keys.  If the key is not a wildcard then identify the set
	 * of DB tables and keys which apply to it. */
	result.isVirtualTbl = false
	result.needCache = true
	result.onChange = OnchangeEnable
	result.nOpts = &notificationOpts{mInterval: 0, pType: OnChange}

	/* Use the requested path to create a positive filter of component types to
	 * process.  Note that a completely empty filter means no filtering is
	 * required. */
	compTypeFilter := []componentType{}
	if key == "*" {
		compTypeFilter = compTypesForSubscriptionUri(targetUriPath)
	} else {
		cType := getCompType(key, inParams.dbs[db.StateDB])
		if cType == CompTypeInvalid {
			return result, nil
		}
		compTypeFilter = []componentType{cType}
	}

	for cType, tblNames := range compTblMap {
		/* An empty filter means no filtering is required. */
		if len(compTypeFilter) > 0 {
			/* Filtering is required, skip all component types not present in the
			 * filter. */
			filteredOut := true
			for _, ct := range compTypeFilter {
				if ct == cType {
					filteredOut = false
					break
				}
			}
			if filteredOut {
				continue
			}
		}

		tblName := tblNames[0]
		tblKey := tblNames[1]
		tblDb := db.StateDB
		if key != "*" {
			/* Generally the DB key will be the component name (yang key), for
			 * the cases where it is not (e.g. Port components) there will be
			 * special casing just below to handle it. */
			tblKey = key
		}

		var subCompTblName, subCompTblKey string
		var subCompDb db.DBNum
		if cType == CompTypeIC {
			/* The integrated-circuit subtree is backed by multiple DB tables but
			 * our compTblMap only captures one.  The special handling here is to
			 * cover all tables.
			 * For wildcard expansion we use the NODE_CFG_TBL in ConfigDB but for
			 * sample cases on the specific counter paths we need the counter
			 * tables. */
			tblDb = db.ConfigDB
			if strings.HasPrefix(targetUriPath, COMP_IC_MEM) {
				result.onChange = OnchangeDisable
			} else if strings.HasPrefix(targetUriPath, COMP_IC_PLC) {
				result.onChange = OnchangeDisable
			}
		} else if cType == CompTypePort {
			/* DB keys here are interface names but the path transformer
			 * will map them back to port names. */
			if strings.HasPrefix(targetUriPath, "/openconfig-platform:components/component/state") ||
				strings.HasPrefix(targetUriPath, "/openconfig-platform:components/component/port/state") {
				tblDb = db.StateDB
				tblName = PORT_BREAKOUT
			} else if strings.HasPrefix(targetUriPath, "/openconfig-platform:components/component/subcomponents") {
				/* Port components are the only component where we support the
				 * subcomponents subtree and populate a transceiver name as the
				 * subcomponent.  Handle that case here if needed. */
				subCompDb = db.ConfigDB
				subCompTblName = "BREAKOUT_PORT_XCVR_CFG"
				subCompKey := NewPathInfo(inParams.uri).Var("name#2")
				if key == "*" {
					subCompTblKey = "*"
				} else if subCompKey != "" && subCompKey != "*" {
					subCompTblKey = key + "|" + subCompKey
				} else {
					subCompTblKey = key + "|" + "*"
				}
			} else {
				tblDb = db.ConfigDB
				tblName = BREAKOUT_TBL
			}

			if key == "*" {
				tblKey = "*"
			} else {
				tblKey = platform.InterfaceNameFromPort(key)
			}
		}

		if result.dbDataMap[tblDb] == nil {
			result.dbDataMap[tblDb] = make(map[string]map[string]map[string]string)
		}
		if result.dbDataMap[tblDb][tblName] == nil {
			result.dbDataMap[tblDb][tblName] = make(map[string]map[string]string)
		}
		if subCompTblName != "" {
			if result.dbDataMap[subCompDb] == nil {
				result.dbDataMap[subCompDb] = make(map[string]map[string]map[string]string)
			}
			if result.dbDataMap[subCompDb][subCompTblName] == nil {
				result.dbDataMap[subCompDb][subCompTblName] = make(map[string]map[string]string)
			}
			result.dbDataMap[subCompDb][subCompTblName][subCompTblKey] = map[string]string{}
		}

		/* Add the DB table and key to the result.  Power components are a
		 * bit tricky as they have multi-level keys so we'll do a bit of
		 * special handling for that case now if needed.  Otherwise just add
		 * the table key to the result. */
		if tblName == POWER_INFO_TBL {
			/* Power components have multi-level keys, e.g. POWER_INFO|<hotswap>
			 * and POWER_INFO|<hotswap>|<rail> which will lead to duplicate
			 * processing and updates if we let the framework expand a simple
			 * wildcard key.  It would expand the wildcard to both the hotswap
			 * component and the hotswap's rail(s) component. */
			keyList, err := inParams.dbs[db.StateDB].GetKeysPattern(&(db.TableSpec{Name: tblName}), db.Key{Comp: []string{tblKey}})
			if err != nil {
				return result, err
			}
			/* First add all components with a single key. */
			for _, k := range keyList {
				if len(k.Comp) != 1 {
					continue
				}
				result.dbDataMap[tblDb][tblName][k.Comp[0]] = map[string]string{}
			}
			/* Check components with two keys for any that were not covered
			 * above and add them exactly once.  This covers the case of a
			 * component that has only rails. */
			added := map[string]bool{}
			for _, k := range keyList {
				if len(k.Comp) != 2 {
					continue
				}
				powerKey := k.Comp[0] + "|" + k.Comp[1]
				if _, ok := result.dbDataMap[tblDb][tblName][k.Comp[0]]; ok {
					continue
				}
				if _, ok := added[k.Comp[0]]; ok {
					continue
				}
				added[k.Comp[0]] = true
				result.dbDataMap[tblDb][tblName][powerKey] = map[string]string{}
			}
		} else {
			if result.dbDataMap[tblDb][tblName][tblKey] == nil {
				result.dbDataMap[tblDb][tblName][tblKey] = map[string]string{}
			}
		}
	}
	if log.V(lvl.DEBUG) {
		for db, _ := range result.dbDataMap {
			for tbl, _ := range result.dbDataMap[db] {
				for k, v := range result.dbDataMap[db][tbl] {
					log.Infof("+++ Subscribe_pfm_components_xfmr result: DB=%d, Table=%s, Key=%s, Flds=%v +++", db, tbl, k, v)
				}
			}
		}
	}
	return result, nil
}

var YangToDb_pfm_components_xfmr SubTreeXfmrYangToDb = func(inParams XfmrParams) (map[string]map[string]db.Value, error) {
	pathInfo := NewPathInfo(inParams.uri)
	key := pathInfo.Var("name")
	if key == "" {
		return nil, nil
	}

	log.V(lvl.DEBUG).Infof("YangToDb_pfm_components_xfmr: uri %s, name %s, requestURI %s, op %v", inParams.uri, key, inParams.requestUri, inParams.oper)
	pfmObj := getPfmRootObject(inParams.ygRoot)
	if pfmObj == nil || pfmObj.Component == nil || len(pfmObj.Component) < 1 {
		return nil, tlerr.NotSupported("YangToDb_pfm_components_xfmr: Empty component.")
	}

	comp, ok := pfmObj.Component[key]
	if !ok || comp == nil {
		return nil, fmt.Errorf("YangToDb_pfm_components_xfmr: Invalid component name: %s", key)
	}

	inParams.key = key
	var tblName, portName string
	cType := CompTypeInvalid
	if validICName(&key) {
		tblName = NODE_CFG_TBL
		cType = CompTypeIC
	} else if validXcvrName(&key) {
		tblName = TRANSCEIVER_CONTROL
		cType = CompTypeXcvr
	} else if strings.HasPrefix(key, CHASSIS_PREFIX) {
		tblName = CHASSIS_CFG
		cType = CompTypeChassis
	} else if ifName := platform.InterfaceNameFromPort(key); len(ifName) > 0 {
		tblName = BREAKOUT_TBL
		portName = key
		key = ifName
		cType = CompTypePort
	} else {
		return nil, tlerr.NotSupported("YangToDb_pfm_components_xfmr: SET not supported for component: %s", key)
	}
	inParams.table = tblName

	memMap := make(map[string]map[string]db.Value)
	if inParams.oper == DELETE {
		switch cType {
		case CompTypeIC:
			/* We only support deletion on the following path:
			 * /components/component/integrated-circuit/config/node-id */
			memMap[NODE_CFG_TBL] = map[string]db.Value{key: db.Value{Field: map[string]string{"node-id": ""}}}
		case CompTypePort:
			memMap[BREAKOUT_TBL] = map[string]db.Value{key: db.Value{Field: map[string]string{"port-id": ""}}}
		case CompTypeXcvr:
			return nil, tlerr.NotSupported("YangToDb_pfm_components_xfmr: DELETE Operation Not Supported for Xcvr config.")
		}
	} else {
		if comp.Config != nil {
			fields := db.Value{Field: make(map[string]string)}
			if comp.Config.Name != nil {
				if inParams.key != *comp.Config.Name {
					return nil, fmt.Errorf("Mismatch between component name key: (%s) and name to be configured: (%s)", inParams.key, *comp.Config.Name)
				}
				fields.Set("name", *comp.Config.Name)
			}
			memMap[tblName] = map[string]db.Value{key: fields}
		}
		if comp.Subcomponents != nil && comp.Subcomponents.Subcomponent != nil {
			for subCompKey := range comp.Subcomponents.Subcomponent {
				if comp.Subcomponents.Subcomponent[subCompKey].Config != nil && comp.Subcomponents.Subcomponent[subCompKey].Config.Name != nil {
					if portName != "" {
						portKey := portName + "|" + *comp.Subcomponents.Subcomponent[subCompKey].Config.Name
						memMap["BREAKOUT_PORT_XCVR_CFG"] = map[string]db.Value{portKey: db.Value{Field: map[string]string{"NULL": "NULL"}}}
						log.V(lvl.INFO).Infof("YangToDb_pfm_components_xfmr: memMap: %v\n", memMap)
					}
				}
			}
		}
	}

	if tblName == CHASSIS_CFG {
		subOpMap := make(map[db.DBNum]map[string]map[string]db.Value)
		subOpMap[db.ConfigDB] = memMap
		updateSubOpDataMap(subOpMap, UPDATE, inParams)
		return nil, nil
	}
	if len(memMap) == 0 {
		return nil, tlerr.NotSupported("YangToDb_pfm_components_xfmr: SET not supported for the targeted path on component: %s", inParams.key)
	}
	log.V(lvl.DEBUG).Infof("YangToDb_pfm_components_xfmr: result %v", memMap)
	return memMap, nil
}

var DbToYang_pfm_components_xfmr SubTreeXfmrDbToYang = func(inParams XfmrParams) error {
	pathInfo := NewPathInfo(inParams.uri)
	log.V(lvl.DEBUG).Infof("DbToYang_pfm_components_xfmr: %s, path: %s, vars: %v",
		pathInfo.Template, pathInfo.Path, pathInfo.Vars)

	if !strings.Contains(inParams.requestUri, "/openconfig-platform:components") {
		return errors.New("Component not supported")
	}
	log.V(lvl.DEBUG).Info("inParams.Uri:", inParams.requestUri)
	targetUriPath, err := getYangPathFromUri(pathInfo.Path)
	if err != nil {
		return err
	}

	/* Extract the component name (key), it may be empty ("") if the get is for
	 * the entire list/container (/components/component) */
	compName := pathInfo.Var("name")
	/* Rails and subcomponents may have a second level key */
	subKey := pathInfo.Var("name#2")
	return getSysComponents(getPfmRootObject(inParams.ygRoot), targetUriPath, inParams, compName, subKey)
}

func getPfmRootObject(s *ygot.GoStruct) *ocbinds.OpenconfigPlatform_Components {
	if s == nil {
		return nil
	}
	deviceObj := (*s).(*ocbinds.Device)
	return deviceObj.Components
}

/* Main workhorse of the DbToYang transformer.  The get is either for the entire
 * component list (/components/component) or for a specific component name; no
 * wildcards are handled here. */
func getSysComponents(pf_cpts *ocbinds.OpenconfigPlatform_Components, targetUriPath string, inParams XfmrParams, compName, subKey string) error {

	log.V(lvl.DEBUG).Infof("Preparing dB for system components")

	uri := inParams.uri
	dbs := inParams.dbs
	ygRoot := inParams.ygRoot

	var err error
	d := dbs[db.StateDB]
	cfgdb := dbs[db.ConfigDB]
	log.V(lvl.DEBUG).Infof("getSysComponents: compName: %s targetUriPath: %s", compName, targetUriPath)
	switch targetUriPath {
	case COMP:
		log.V(lvl.DEBUG).Infof("compName: %v", compName)
		subCompName := "" /* Get all subcomponents */
		if compName == "" {
			/* Handle all component types except for ports, they will be handled just below. */
			for cType, tbl := range compTblMap {
				if cType == CompTypePort || len(tbl) < 2 {
					continue
				}
				tblName := tbl[0]
				createCompAndFuncCall(pf_cpts, targetUriPath, cType, inParams, tblName, tbl[1])
			}

			ports := platform.PortNames()
			for _, port := range ports {
				pf_comp, err := pf_cpts.NewComponent(port)
				if err != nil {
					log.V(lvl.DEBUG).Infof("Component creation failed with NewComponent for comp %v; err = %v", port, err)
					continue
				}
				log.V(lvl.DEBUG).Info("DPB Adding ", port)
				ygot.BuildEmptyTree(pf_comp)
				if err = fillPortInfo(pf_comp, port, subCompName, AllPaths, targetUriPath, d, cfgdb); err != nil {
					log.V(lvl.DEBUG).Info(err)
				}
			}

			/* System Eeprom is assembled from the EEPROM_INFO table, which stores
			 * each attribute as its own Name/Value row instead of one row per
			 * component. It therefore does not fit the compTblMap model and is
			 * created explicitly here (previously handled by pfm_app.go). */
			sysEeprom, err := pf_cpts.NewComponent(SYS_EEPROM_NAME)
			if err != nil {
				log.V(lvl.DEBUG).Infof("Component creation failed with NewComponent for comp %v; err = %v", SYS_EEPROM_NAME, err)
			} else {
				ygot.BuildEmptyTree(sysEeprom)
				if err = fillSysEepromInfo(sysEeprom, SYS_EEPROM_NAME, AllPaths, targetUriPath, d); err != nil {
					log.V(lvl.DEBUG).Info(err)
				}
				// Healthz should be populated for all comp types
				fillHealthzInfo(SYS_EEPROM_NAME, "", targetUriPath, d, sysEeprom)
			}
		} else {
			compType := getCompType(compName, d)
			if compType == CompTypeInvalid {
				return nil
			}
			pf_comp, ok := pf_cpts.Component[compName]
			if !ok || pf_comp == nil {
				return fmt.Errorf("invalid input component name: %s", compName)
			}
			ygot.BuildEmptyTree(pf_comp)
			if err = compTypeToFuncCall(compType, compName, subCompName, pf_comp, targetUriPath, dbs, AllPaths, ygRoot); err != nil {
				log.V(lvl.DEBUG).Info(err)
			}
			// Healthz should be populated for all comp types
			fillHealthzInfo(compName, "", targetUriPath, d, pf_comp)
		}
	case COMP_ST:
		compType := getCompType(compName, d)
		if compType == CompTypeInvalid {
			return nil
		}
		pf_comp, ok := pf_cpts.Component[compName]
		if !ok || pf_comp == nil {
			return fmt.Errorf("invalid input component name for state path: %s", compName)
		}
		ygot.BuildEmptyTree(pf_comp)
		if compType == CompTypeTemp || compType == CompTypeXcvr || compType == CompTypePowerSupplyPB || compType == CompTypePowerSupplyVR || compType == CompTypePowerSupplyPH || compType == CompTypePowerSupplyPSEQ {
			ygot.BuildEmptyTree(pf_comp.State)
			ygot.BuildEmptyTree(pf_comp.State.Temperature)
		}
		if compType == CompTypePcie {
			ygot.BuildEmptyTree(pf_comp.State)
			ygot.BuildEmptyTree(pf_comp.State.Pcie)
		}
		if err = compTypeToFuncCall(compType, compName, subKey, pf_comp, targetUriPath, dbs, StatePaths, ygRoot); err != nil {
			log.V(lvl.DEBUG).Info(err)
		}

	case COMP_CFG:
		compType := getCompType(compName, d)
		if compType == CompTypeInvalid {
			return nil
		}
		if compType == CompTypeIC || compType == CompTypeChassis || compType == CompTypeXcvr || compType == CompTypePort || compType == CompTypePowerSupplyPH {
			pf_comp, ok := pf_cpts.Component[compName]
			if !ok || pf_comp == nil {
				return fmt.Errorf("Invalid component name: %s", compName)
			}
			ygot.BuildEmptyTree(pf_comp)
			if err = compTypeToFuncCall(compType, compName, subKey, pf_comp, targetUriPath, dbs, ConfigPaths, ygRoot); err != nil {
				log.V(lvl.DEBUG).Info(err)
			}
			break
		}
	case COMP_HEALTHZ:
		fallthrough
	case COMP_FAULTS:
		pf_comp, ok := pf_cpts.Component[compName]
		if !ok || pf_comp == nil {
			return fmt.Errorf("invalid input component name: %s", compName)
		}
		ygot.BuildEmptyTree(pf_comp)
		// Healthz should be populated for all comp types
		fillHealthzInfo(compName, "", targetUriPath, d, pf_comp)
	default:
		/* The following cases are handled above:
		 *   /components/component
		 *   /components/component[name=<component_name>]
		 *   /components/component[name=<component_name>]/config
		 *   /components/component[name=<component_name>]/state
		 *   /components/component[name=<component_name>]/healthz
		 *   /components/component[name=<component_name>]/healthz/faults
		 * so the request must be for a specific component's leaf or subtree,
		 * e.g. /components/component[name=integrated_circuit]/integrated-circuit */
		// TODO - Can we de-dup this code with compTypeToFuncCall?  No good way to set pathType...
		compType := getCompType(compName, d)
		if compType == CompTypeInvalid {
			return nil
		}
		pf_comp, ok := pf_cpts.Component[compName]
		if !ok || pf_comp == nil {
			return fmt.Errorf("invalid input component name: %s", compName)
		}
		ygot.BuildEmptyTree(pf_comp)
		switch compType {
		case CompTypePsu:
			ygot.BuildEmptyTree(pf_comp.PowerSupply)
			ygot.BuildEmptyTree(pf_comp.PowerSupply.State)
			switch targetUriPath {
			case COMP_PS:
				fallthrough
			case COMP_PS_STATE:
				fillPsuInfo(pf_comp, compName, true, true, targetUriPath, d)
			default:
				fillPsuInfo(pf_comp, compName, false, true, targetUriPath, d)
			}
		case CompTypeFan:
			fillFanInfo(pf_comp, compName, targetUriPath, d, CompTypeFan)
		case CompTypeFanTray:
			fillFanInfo(pf_comp, compName, targetUriPath, d, CompTypeFan)
		case CompTypeStorage:
			ygot.BuildEmptyTree(pf_comp.Storage)
			ygot.BuildEmptyTree(pf_comp.Storage.State)
			switch targetUriPath {
			case COMP_STORAGE:
				return fillStorageInfo(pf_comp, compName, AllCompPaths, targetUriPath, d)
			case COMP_STORAGE_ST:
				return fillStorageInfo(pf_comp, compName, StatePaths, targetUriPath, d)
			default:
				return fillStorageInfo(pf_comp, compName, SingularPath, targetUriPath, d)
			}
		case CompTypeXcvr:
			ygot.BuildEmptyTree(pf_comp.Transceiver)
			ygot.BuildEmptyTree(pf_comp.Transceiver.State)
			ygot.BuildEmptyTree(pf_comp.Transceiver.Config)

			laneIdx := NewPathInfo(uri).Var("index")
			switch targetUriPath {
			case XCVR_BASE_PREFIX /*XCVR_BASE_CONFIG,*/, XCVR_BASE_STATE, XCVR_BASE_PC, XCVR_BASE_CHANNEL:
				return fillXcvrInfo(pf_comp, compName, true, laneIdx, targetUriPath, dbs)
			default:
				/* For individual components*/
				return fillXcvrInfo(pf_comp, compName, false, laneIdx, targetUriPath, dbs)
			}
		case CompTypeTemp:
			switch targetUriPath {
			case COMP, COMP_ST, TEMP_COMP:
				return fillTempSensorInfo(pf_comp, compName, AllCompPaths, targetUriPath, d)
			default:
				return fillTempSensorInfo(pf_comp, compName, SingularPath, targetUriPath, d)
			}
		case CompTypeIC:
			return fillICInfo(pf_comp, compName, targetUriPath, inParams.dbs, inParams.ygRoot)
		case CompTypeChassis:
			ygot.BuildEmptyTree(pf_comp.Chassis)
			ygot.BuildEmptyTree(pf_comp.Chassis.State)
			ygot.BuildEmptyTree(pf_comp.Chassis.Config)
			switch targetUriPath {
			case FIRMWARE_CHASSIS, FIRMWARE_CHASSIS_ALARMS, FIRMWARE_CHASSIS_ALARMS_STATE, FIRMWARE_CHASSIS_STATE:
				return fillFirmwareInfo(pf_comp, compName, AllCompPaths, targetUriPath, d, cfgdb)
			case FIRMWARE_CHASSIS_CONFIG:
				return fillFirmwareInfo(pf_comp, compName, ConfigPaths, targetUriPath, d, cfgdb)
			default:
				return fillFirmwareInfo(pf_comp, compName, SingularPath, targetUriPath, d, cfgdb)
			}
		case CompTypeNWStack:
			fallthrough
		case CompTypeOS:
			fallthrough
		case CompTypeBootLoader:
			ygot.BuildEmptyTree(pf_comp.SoftwareModule)
			ygot.BuildEmptyTree(pf_comp.SoftwareModule.State)
			switch targetUriPath {
			case COMP_SW_MOD:
				fallthrough
			case COMP_SW_MOD_ST:
				return fillSWCompInfo(pf_comp, compName, AllCompPaths, targetUriPath, d, compType)
			default:
				return fillSWCompInfo(pf_comp, compName, SingularPath, targetUriPath, d, compType)
			}
		case CompTypePort:
			ygot.BuildEmptyTree(pf_comp.Port)
			ygot.BuildEmptyTree(pf_comp.Port.State)
			ygot.BuildEmptyTree(pf_comp.Port.Config)
			switch targetUriPath {
			case COMP_PORT:
				fallthrough
			case COMP_PORT_ST:
				fallthrough
			case COMP_PORT_CFG:
				return fillPortInfo(pf_comp, compName, subKey, AllCompPaths, targetUriPath, d, cfgdb)
			default:
				return fillPortInfo(pf_comp, compName, subKey, SingularPath, targetUriPath, d, cfgdb)
			}
		case CompTypePcie:
			ygot.BuildEmptyTree(pf_comp.State)
			ygot.BuildEmptyTree(pf_comp.State.Pcie)
			return fillPcieInfo(pf_comp, compName, AllPaths, targetUriPath, d)
		case CompTypePowerSupplyPB, CompTypePowerSupplyPH, CompTypePowerSupplyVR, CompTypePowerSupplyPSEQ:
			railKey := NewPathInfo(uri).Var("name#2")
			ygot.BuildEmptyTree(pf_comp.PowerSupply)
			ygot.BuildEmptyTree(pf_comp.PowerSupply.State)

			switch targetUriPath {
			case COMP_PS:
				fillPowerSupplyInfo(pf_comp, compName, railKey, AllCompPaths, targetUriPath, d)
			case COMP_PS_STATE:
				fillPowerSupplyInfo(pf_comp, compName, railKey, StatePaths, targetUriPath, d)
			default:
				fillPowerSupplyInfo(pf_comp, compName, railKey, SingularPath, targetUriPath, d)
			}
		case CompTypeControllerCard:
			return fillControllerCardInfo(pf_comp, compName, SingularPath, targetUriPath, d)
		case CompTypeLineCard:
			return fillLineCardInfo(pf_comp, compName, SingularPath, targetUriPath, d)
		case CompTypeDiskCarrier:
			return fillDiskCarrierInfo(pf_comp, compName, SingularPath, targetUriPath, d)
		case CompTypeSystemEeprom:
			return fillSysEepromInfo(pf_comp, compName, SingularPath, targetUriPath, d)
		default:
			return fmt.Errorf("Unhandled Component: %s", compName)
		}
	}

	return err
}

func createCompAndFuncCall(pfCpts *ocbinds.OpenconfigPlatform_Components, targetUriPath string, compType componentType, inParams XfmrParams, tblName string, tblKey string) {
	var compNames []string
	var err error
	dbs := inParams.dbs
	d := dbs[db.StateDB]
	cfgdb := dbs[db.ConfigDB]
	switch compType {
	case CompTypeIC:
		compNames, err = getAllTableEntries(cfgdb, tblName, tblKey)
	default:
		compNames, err = getAllTableEntries(d, tblName, tblKey)
	}
	if err != nil {
		log.V(lvl.DEBUG).Info(err)
	}

	for _, compAndKey := range compNames {
		compKeys := strings.Split(compAndKey, d.Opts.KeySeparator)
		if len(compKeys) == 0 {
			continue
		}
		comp := compKeys[0]
		pfComp := pfCpts.Component[comp]
		if pfComp == nil {
			pfComp, err = pfCpts.NewComponent(comp)
			if err != nil {
				log.V(lvl.DEBUG).Infof("Component creation failed with NewComponent for comp %v; err = %v", comp, err)
				continue
			}
			ygot.BuildEmptyTree(pfComp)
		}

		if err = compTypeToFuncCall(compType, comp, "", pfComp, targetUriPath, dbs, AllPaths, inParams.ygRoot); err != nil {
			log.V(lvl.DEBUG).Info(err)
		}
		// Healthz should be populated for all comp types
		fillHealthzInfo(comp, "", targetUriPath, d, pfComp)
	}
}

/* Helper to go from a component type to the type specific helper which reads
 * the DB data and populates the ocbinds structs.
 * createCompAndFuncCall - when fetching /components/component
 * getSysComponents - when fetching the following paths:
 *   /components/component[name=<compName>]
 *   /components/component[name=<compName>]/config
 *   /components/component[name=<compName>]/state
 */
func compTypeToFuncCall(cType componentType, compName, subKey string, pfComp *ocbinds.OpenconfigPlatform_Components_Component, targetUriPath string, dbs [db.MaxDB]*db.DB, pType PathType, ygRoot *ygot.GoStruct) error {
	log.V(lvl.DEBUG).Infof("compTypeToFuncCall with name=%s type=%v pType=%v", compName, cType, pType)
	d := dbs[db.StateDB]
	cfgdb := dbs[db.ConfigDB]
	ygot.BuildEmptyTree(pfComp)
	switch cType {
	case CompTypeStorage:
		return fillStorageInfo(pfComp, compName, pType, targetUriPath, d)
	case CompTypePsu:
		if pType == SingularPath {
			return fillPsuInfo(pfComp, compName, false, false, targetUriPath, d)
		} else if pType == AllPaths {
			if err := fillPsuInfo(pfComp, compName, true, true, targetUriPath, d); err != nil {
				return err
			}
		}
		return fillPsuInfo(pfComp, compName, true, false, targetUriPath, d)
	case CompTypeFan:
		return fillFanInfo(pfComp, compName, targetUriPath, d, cType)
	case CompTypeFanTray:
		return fillFanInfo(pfComp, compName, targetUriPath, d, cType)
	case CompTypeXcvr:
		return fillXcvrInfo(pfComp, compName, pType != SingularPath, "", targetUriPath, dbs)
	case CompTypeTemp:
		return fillTempSensorInfo(pfComp, compName, pType, targetUriPath, d)
	case CompTypeIC:
		return fillICInfo(pfComp, compName, targetUriPath, dbs, ygRoot)
	case CompTypeChassis:
		return fillFirmwareInfo(pfComp, compName, pType, targetUriPath, d, cfgdb)
	case CompTypeNWStack, CompTypeOS, CompTypeBootLoader:
		return fillSWCompInfo(pfComp, compName, pType, targetUriPath, d, cType)
	case CompTypePort:
		return fillPortInfo(pfComp, compName, subKey, pType, targetUriPath, d, cfgdb)
	case CompTypePcie:
		return fillPcieInfo(pfComp, compName, pType, targetUriPath, d)
	case CompTypePowerSupplyVR, CompTypePowerSupplyPB, CompTypePowerSupplyPH, CompTypePowerSupplyPSEQ:
		return fillPowerSupplyInfo(pfComp, compName, "", pType, targetUriPath, d)
	case CompTypeControllerCard:
		return fillControllerCardInfo(pfComp, compName, pType, targetUriPath, d)
	case CompTypeLineCard:
		return fillLineCardInfo(pfComp, compName, pType, targetUriPath, d)
	case CompTypeDiskCarrier:
		return fillDiskCarrierInfo(pfComp, compName, pType, targetUriPath, d)
	case CompTypeSystemEeprom:
		return fillSysEepromInfo(pfComp, compName, pType, targetUriPath, d)
	}
	return errors.New("Invalid component type")
}

func shouldExportLeaf(pType PathType, targetUriPath string, leafType LeafType, leafUriPath string) bool {
	switch pType {
	case AllPaths:
		return true
	case ConfigPaths:
		return leafType == ConfigLeaf
	case StatePaths:
		return leafType == StateLeaf
	case AllCompPaths:
		return leafType == CompStateLeaf || leafType == CompConfigLeaf
	case CompConfigPaths:
		return leafType == CompConfigLeaf
	case CompStatePaths:
		return leafType == CompStateLeaf
	default:
		// Check that the request target is a prefix of the leaf. This can be done simply since both
		// targetUriPath and dataUriPath are schema paths (without any path variables).
		// targetUriPath is a schema path (from getYangPathFromUri in xlate_utils.go).
		// dataUriPath is a schema path (from the constants at the top of this file).
		return strings.HasPrefix(leafUriPath, targetUriPath)
	}
}

func entityInfo(name string, d *db.DB) PhysicalEntityInfo {
	e, err := d.GetEntry(&db.TableSpec{Name: PHYS_ENTITY_TBL}, db.Key{Comp: []string{name}})
	if err != nil {
		return PhysicalEntityInfo{}
	}
	return PhysicalEntityInfo{
		Parent:           e.Get("parent_name"),
		PositionInParent: e.Get("position_in_parent"),
		InstallComponent: e.Get("install_component"),
	}
}

func entityParent(name, default_parent string, d *db.DB) string {
	entityEntry, err := d.GetEntry(&db.TableSpec{Name: PHYS_ENTITY_TBL}, db.Key{Comp: []string{name}})
	if err != nil {
		return default_parent
	}
	if parent := entityEntry.Get("parent_name"); parent != "" {
		return parent
	}
	return default_parent
}

func entityLocation(name, default_location string, d *db.DB) string {
	entityEntry, err := d.GetEntry(&db.TableSpec{Name: PHYS_ENTITY_TBL}, db.Key{Comp: []string{name}})
	if err != nil {
		return default_location
	}
	if location := entityEntry.Get("position_in_parent"); location != "" {
		return location
	}
	return default_location
}

func validPsuName(name *string) bool {
	if name == nil || *name == "" {
		return false
	}
	valid, _ := regexp.MatchString("PSU ?[1-9][0-9]*$", *name)
	return valid
}

func getPsuInfoFromDb(name string, d *db.DB) (PSUInfo, error) {
	var psuInfo PSUInfo
	var err error

	psuEntry, err := d.GetEntry(&db.TableSpec{Name: PSU_TBL}, db.Key{Comp: []string{name}})
	if err != nil {
		log.V(lvl.DEBUG).Info("Can't get entry: ", name, "; Error: ", err)
	}

	psuInfo.Enabled = false
	if psuEntry.Get("status") == "true" {
		psuInfo.Enabled = true
	}

	psuInfo.Volt_Type = psuEntry.Get("type")

	psuInfo.Install_Position = psuEntry.Get("install-position")
	psuInfo.Install_Component = psuEntry.Get("install-component")
	psuInfo.Removable = psuEntry.Get("removable") == "true"

	psuInfo.Presence = false
	if psuEntry.Get("presence") == "true" {
		psuInfo.Presence = true
	}

	psuInfo.Status = false
	if psuEntry.Get("status") == "true" {
		psuInfo.Status = true
	}

	psuInfo.Parent = entityParent(name, psuEntry.Get("parent"), d)
	psuInfo.Location = entityLocation(name, psuEntry.Get("location"), d)
	psuInfo.Model_Name = convertUTF8EndcodedString(psuEntry.Get("model"))
	psuInfo.Manufacturer = convertUTF8EndcodedString(psuEntry.Get("mfr_id"))
	psuInfo.Part_Number = convertUTF8EndcodedString(psuEntry.Get("part-no"))
	psuInfo.Serial_Number = convertUTF8EndcodedString(psuEntry.Get("serial"))
	psuInfo.Input_Current = extractFloat32Str("input_current", &psuEntry)
	psuInfo.Input_Voltage = extractFloat32Str("input_voltage", &psuEntry)
	psuInfo.Output_Voltage = extractFloat32Str("output_voltage", &psuEntry)
	psuInfo.Output_Current = extractFloat32Str("output_current", &psuEntry)
	psuInfo.Output_Power = extractFloat32Str("output_power", &psuEntry)
	return psuInfo, err
}

func fillPsuInfo(psuCom *ocbinds.OpenconfigPlatform_Components_Component,
	name string, all bool, getPowerStats bool, targetUriPath string, d *db.DB) error {
	log.V(lvl.DEBUG).Infof("fillPsuInfo: name=%s all=%v getPowerStats=%v targetUriPath=%s", name, all, getPowerStats, targetUriPath)
	var err error
	psuInfo, err := getPsuInfoFromDb(name, d)
	if err != nil {
		log.V(lvl.DEBUG).Info("Error Getting PSU info from dB")
		return err
	}

	empty := !psuInfo.Presence
	psuEepromState := psuCom.State
	defaultParentVal := CHASSIS_PREFIX
	if all {
		psuCom.Config.Name = &name
		if getPowerStats {
			if err != nil {
				log.V(lvl.DEBUG).Info("float data error")
				return err
			}
			return err
		}
		psuEepromState.Empty = &empty
		psuEepromState.Name = &name

		psuEepromState.Parent = &defaultParentVal
		if psuInfo.Parent != "" && psuInfo.Parent != "N/A" {
			psuEepromState.Parent = &psuInfo.Parent
		}
		psuEepromState.Location = StringWithDefault(&psuInfo.Location, "")

		psuEepromState.Type, _ = psuEepromState.To_OpenconfigPlatform_Components_Component_State_Type_Union(
			ocbinds.OpenconfigPlatformTypes_OPENCONFIG_HARDWARE_COMPONENT_POWER_SUPPLY)
		psuEepromState.OperStatus = ocbinds.OpenconfigPlatformTypes_COMPONENT_OPER_STATUS_DISABLED
		if psuInfo.Presence {
			if psuInfo.Status {
				psuEepromState.OperStatus = ocbinds.OpenconfigPlatformTypes_COMPONENT_OPER_STATUS_ACTIVE
			} else {
				psuEepromState.OperStatus = ocbinds.OpenconfigPlatformTypes_COMPONENT_OPER_STATUS_INACTIVE
			}
		}

		if psuInfo.Model_Name != "" {
			psuEepromState.ModelName = &psuInfo.Model_Name
			psuEepromState.Description = &psuInfo.Model_Name
			psuEepromState.PartNo = &psuInfo.Model_Name
		}
		if psuInfo.Manufacturer != "" {
			psuEepromState.MfgName = &psuInfo.Manufacturer
		}
		if psuInfo.Serial_Number != "" {
			psuEepromState.SerialNo = &psuInfo.Serial_Number
		}
		if psuInfo.Part_Number != "" {
			psuEepromState.PartNo = &psuInfo.Part_Number
		}
		if psuInfo.Install_Position != "" {
			psuEepromState.InstallPosition = &psuInfo.Install_Position
		}
		if psuInfo.Install_Component != "" {
			psuEepromState.InstallComponent = &psuInfo.Install_Component
		}

		psuEepromState.Removable = &psuInfo.Removable
		psuCom.PowerSupply.State.InputCurrent = psuInfo.Input_Current
		psuCom.PowerSupply.State.InputVoltage = psuInfo.Input_Voltage
		psuCom.PowerSupply.State.OutputCurrent = psuInfo.Output_Current
		psuCom.PowerSupply.State.OutputVoltage = psuInfo.Output_Voltage
		psuCom.PowerSupply.State.OutputPower = psuInfo.Output_Power

		return err
	}

	switch targetUriPath {
	case COMP_STATE_EMPTY:
		psuEepromState.Empty = &empty
	case COMP_STATE_OPER_STATUS:
		psuEepromState.OperStatus = ocbinds.OpenconfigPlatformTypes_COMPONENT_OPER_STATUS_INACTIVE
		if psuInfo.Status {
			psuEepromState.OperStatus = ocbinds.OpenconfigPlatformTypes_COMPONENT_OPER_STATUS_ACTIVE
		}
	case COMP_STATE_SERIAL_NO:
		if psuInfo.Serial_Number != "" {
			psuEepromState.SerialNo = &psuInfo.Serial_Number
		}
	case COMP_STATE_DESCR:
		if psuInfo.Model_Name != "" {
			psuEepromState.Description = &psuInfo.Model_Name
		}
	case COMP_STATE_MODEL_NAME:
		if psuInfo.Model_Name != "" {
			psuEepromState.ModelName = &psuInfo.Model_Name
		}
	case COMP_STATE_MFG_NAME:
		if psuInfo.Manufacturer != "" {
			psuEepromState.MfgName = &psuInfo.Manufacturer
		}
	case COMP_STATE_PART_NO:
		if psuInfo.Model_Name != "" {
			psuEepromState.PartNo = &psuInfo.Model_Name
		}
		if psuInfo.Part_Number != "" {
			psuEepromState.PartNo = &psuInfo.Part_Number
		}
	case COMP_STATE_NAME:
		psuEepromState.Name = &name
	case COMP_STATE_PARENT:
		psuEepromState.Parent = &defaultParentVal
		if psuInfo.Parent != "" && psuInfo.Parent != "N/A" {
			psuEepromState.Parent = &psuInfo.Parent
		}
	case COMP_STATE_LOCATION:
		psuEepromState.Location = StringWithDefault(&psuInfo.Location, "")
	case COMP_STATE_TYPE:
		psuEepromState.Type, _ = psuEepromState.To_OpenconfigPlatform_Components_Component_State_Type_Union(
			ocbinds.OpenconfigPlatformTypes_OPENCONFIG_HARDWARE_COMPONENT_POWER_SUPPLY)
	case COMP_STATE_INSTALL_POSITION:
		if psuInfo.Install_Position != "" {
			psuEepromState.InstallPosition = &psuInfo.Install_Position
		}
	case COMP_STATE_INSTALL_COMPONENT:
		if psuInfo.Install_Component != "" {
			psuEepromState.InstallComponent = &psuInfo.Install_Component
		}
	case COMP_STATE_REMOVABLE:
		psuEepromState.Removable = &psuInfo.Removable
	default:
		return fmt.Errorf("unsupported leaf path for PSU component: %v", targetUriPath)
	}

	return err
}

func validFanName(name *string) bool {
	if name == nil || *name == "" {
		return false
	}
	validFan, _ := regexp.MatchString("(?i)fan[1-9][0-9]*$", *name)
	return validFan
}

func validFanTrayName(name string) bool {
	if name == "" {
		return false
	}
	if strings.HasPrefix(name, FAN_TRAY_PREFIX) {
		return true
	} else if strings.HasSuffix(name, FAN_TRAY_SUFFIX) {
		return true
	} else {
		return false
	}
}

func getFanInfoFromDb(name string, d *db.DB) (FanInfo, error) {
	var fanInfo FanInfo

	index := strings.TrimPrefix(name, "fan")
	tblName := FAN_TBL
	if validFanTrayName(name) {
		tblName = FAN_TRAY_TBL
		index = strings.TrimSuffix(strings.TrimPrefix(name, FAN_TRAY_PREFIX), FAN_TRAY_SUFFIX)
	}
	dbEntry, err := d.GetEntry(&db.TableSpec{Name: tblName}, db.Key{Comp: []string{name}})
	if err != nil {
		log.V(lvl.DEBUG).Infof("Failed to get %s from DB, error %v", name, err)
		return fanInfo, err
	}

	fanInfo.hardwareRev = dbEntry.Get("hardware_revision")
	fanInfo.isReplaceable = false
	if removable := dbEntry.Get("is_replaceable"); removable == "true" || removable == "True" {
		fanInfo.isReplaceable = true
	}
	fanInfo.location = index
	fanInfo.mfgDate = dbEntry.Get("mfg_date")
	fanInfo.parent = entityParent(name, dbEntry.Get("drawer_name"), d)
	fanInfo.partNo = dbEntry.Get("part_no")
	fanInfo.model = dbEntry.Get("model")
	fanInfo.presence = false
	if presence := dbEntry.Get("presence"); presence == "true" || presence == "True" {
		fanInfo.presence = true
	}
	fanInfo.pwm = dbEntry.Get("pwm")
	fanInfo.serial = dbEntry.Get("serial")
	fanInfo.speed = dbEntry.Get("speed")
	fanInfo.status = false
	if status := dbEntry.Get("status"); status == "true" || status == "True" {
		fanInfo.status = true
	}
	fanInfo.installComponent = dbEntry.Get("install-component")
	fanInfo.installPosition = dbEntry.Get("install-position")

	return fanInfo, nil
}

func fillFanInfo(comp *ocbinds.OpenconfigPlatform_Components_Component,
	name string, targetUriPath string, d *db.DB, cType componentType) error {
	log.V(lvl.DEBUG).Infof("dbToYangFan: name %s uri %s type %v", name, targetUriPath, cType)

	fanInfo, err := getFanInfoFromDb(name, d)
	if err != nil {
		log.V(lvl.DEBUG).Info("Error Getting fan info from dB")
		return err
	}

	/* Supported fan/fan-tray leaves are:
	 *   - /components/component/state/empty
	 *   - /components/component/state/hardware-version
	 *   - /components/component/state/location
	 *   - /components/component/state/mfg-date
	 *   - /components/component/state/name
	 *   - /components/component/state/oper-status
	 *   - /components/component/state/parent
	 *   - /components/component/state/part-no
	 *   - /components/component/state/removable
	 *   - /components/component/state/serial-no
	 *   - /components/component/state/type
	 *   - /components/component/state/install-component
	 *   - /components/component/state/install-position
	 *   - /components/component/fan/state/speed
	 *   - /components/component/fan/state/speed-control-pct
	 */
	var allState, allFan bool
	if targetUriPath == COMP {
		allState = true
		allFan = true
	} else if targetUriPath == COMP_ST {
		allState = true
	} else if targetUriPath == COMP_FAN || targetUriPath == COMP_FAN_ST {
		allFan = true
	}

	if targetUriPath == COMP {
		ygot.BuildEmptyTree(comp.State)
		ygot.BuildEmptyTree(comp.Fan)
		ygot.BuildEmptyTree(comp.Fan.State)
	}
	if allState || strings.HasPrefix(targetUriPath, COMP_ST) {
		ygot.BuildEmptyTree(comp.State)
	}
	if allFan || strings.HasPrefix(targetUriPath, COMP_FAN) {
		ygot.BuildEmptyTree(comp.Fan)
		ygot.BuildEmptyTree(comp.Fan.State)
	}

	comp.Name = &name
	if allState || targetUriPath == COMP_STATE_EMPTY {
		empty := !fanInfo.presence
		comp.State.Empty = &empty
	}
	if allState || targetUriPath == COMP_STATE_HW_VER {
		if fanInfo.hardwareRev == "" {
			fanInfo.hardwareRev = "N/A"
		}
		comp.State.HardwareVersion = &fanInfo.hardwareRev
	}
	if allState || targetUriPath == COMP_STATE_LOCATION {
		comp.State.Location = &fanInfo.location
	}
	if allState || targetUriPath == COMP_STATE_MFG_DATE {
		if fanInfo.mfgDate != "" && fanInfo.mfgDate != "N/A" {
			if mfg_date := reformatMfgDate(fanInfo.mfgDate); mfg_date != "" {
				comp.State.MfgDate = &mfg_date
			} else {
				log.V(lvl.WARNING).Infof("fan %s mfg-date expected format: MM-DD-YYYY HH:MM:SS, got %v", name, fanInfo.mfgDate)
			}
		}
	}
	if allState || targetUriPath == COMP_STATE_NAME {
		comp.State.Name = &name
	}
	if allState || targetUriPath == COMP_STATE_OPER_STATUS {
		if fanInfo.presence && fanInfo.status {
			comp.State.OperStatus = ocbinds.OpenconfigPlatformTypes_COMPONENT_OPER_STATUS_ACTIVE
		} else if fanInfo.presence && !fanInfo.status {
			comp.State.OperStatus = ocbinds.OpenconfigPlatformTypes_COMPONENT_OPER_STATUS_INACTIVE
		} else {
			comp.State.OperStatus = ocbinds.OpenconfigPlatformTypes_COMPONENT_OPER_STATUS_DISABLED
		}
	}
	if allState || targetUriPath == COMP_STATE_PARENT {
		if fanInfo.parent != "" && fanInfo.parent != "N/A" {
			comp.State.Parent = &fanInfo.parent
		} else {
			parentChassis := CHASSIS_PREFIX
			comp.State.Parent = &parentChassis
		}
	}
	if allState || targetUriPath == COMP_STATE_PART_NO {
		if fanInfo.model != "" {
			comp.State.PartNo = &fanInfo.model
		}
		/* Let the partNo field from the DB override the model field from the DB
		 * if it is present. */
		if fanInfo.partNo != "" {
			comp.State.PartNo = &fanInfo.partNo
		}
	}
	if allState || targetUriPath == COMP_STATE_REMOVABLE {
		comp.State.Removable = &fanInfo.isReplaceable
	}
	if allState || targetUriPath == COMP_STATE_SERIAL_NO {
		if fanInfo.serial != "" {
			comp.State.SerialNo = &fanInfo.serial
		}
	}
	if allState || targetUriPath == COMP_STATE_TYPE {
		if cType == CompTypeFan {
			comp.State.Type, _ = comp.State.To_OpenconfigPlatform_Components_Component_State_Type_Union(
				ocbinds.OpenconfigPlatformTypes_OPENCONFIG_HARDWARE_COMPONENT_FAN)
		} else if cType == CompTypeFanTray {
			comp.State.Type, _ = comp.State.To_OpenconfigPlatform_Components_Component_State_Type_Union(
				ocbinds.OpenconfigPlatformTypes_OPENCONFIG_HARDWARE_COMPONENT_FAN_TRAY)
		}
	}
	if allState || targetUriPath == COMP_STATE_INSTALL_COMPONENT {
		if fanInfo.installComponent != "" {
			comp.State.InstallComponent = &fanInfo.installComponent
		}
	}
	if allState || targetUriPath == COMP_STATE_INSTALL_POSITION {
		if fanInfo.installPosition != "" {
			comp.State.InstallPosition = &fanInfo.installPosition
		}
	}
	if allState || targetUriPath == COMP_STATE_MODEL_NAME {
		if fanInfo.model != "" {
			comp.State.ModelName = &fanInfo.model
		}
	}
	if allFan || targetUriPath == COMP_FAN_SPEED {
		if fanInfo.speed != "" {
			speedU64, err := strconv.ParseUint(fanInfo.speed, 10, 32)
			if err != nil {
				log.V(lvl.DEBUG).Info("Fan %s, failed to convert speed \"%s\" to uint: %v", name, fanInfo.speed, err)
			} else {
				speedU32 := uint32(speedU64)
				comp.Fan.State.Speed = &speedU32
			}
		}
	}
	return nil
}

func getStorageFromDb(name string, d *db.DB, tblName string) (StorageInfo, error) {
	/* Backend is populating STORAGE_INFO in: sonic-platform-daemons/sonic-componentd/scripts/componentd */
	storageEntry, err := d.GetEntry(&db.TableSpec{Name: tblName}, db.Key{Comp: []string{name}})
	if err != nil {
		log.V(lvl.DEBUG).Info("Cant get entry: ", name, "; Error: ", err)
		return StorageInfo{}, err
	}
	storageInfo := StorageInfo{
		IOErrors:                       storageEntry.Get("io-errors"),
		PartNo:                         storageEntry.Get("part-no"),
		Parent:                         storageEntry.Get("parent"),
		SerialNo:                       storageEntry.Get("serial-no"),
		Removable:                      storageEntry.Get("removable"),
		OperStatus:                     storageEntry.Get("oper-status"),
		Empty:                          storageEntry.Get("empty"),
		WriteAmplificationFactor:       storageEntry.Get("write-amplification-factor"),
		RawReadErrorRate:               storageEntry.Get("raw-read-error-rate"),
		ThroughputPerformance:          storageEntry.Get("throughput-performance"),
		ReallocatedSectorCount:         storageEntry.Get("reallocated-sector-count"),
		PowerOnSeconds:                 storageEntry.Get("power-on-time"),
		SsdLifeLeft:                    storageEntry.Get("ssd-life-left"),
		PowerCycleCount:                storageEntry.Get("power-cycle-count"),
		UncorrectableSectorCountOnLine: storageEntry.Get("uncorrectable-sector-count"),
		NumPureSpare:                   storageEntry.Get("num-pure-spare"),
		NumInitialInvalidBlock:         storageEntry.Get("num-initial-invalid-block"),
		SlcTotalEraseCount:             storageEntry.Get("slc-total-erase-count"),
		SlcMaxEraseCount:               storageEntry.Get("slc-max-erase-count"),
		SlcMinEraseCount:               storageEntry.Get("slc-min-erase-count"),
		SlcAvgEraseCount:               storageEntry.Get("slc-avg-erase-count"),
		TlcTotalEraseCount:             storageEntry.Get("tlc-total-erase-count"),
		TlcMaxEraseCount:               storageEntry.Get("tlc-max-erase-count"),
		TlcMinEraseCount:               storageEntry.Get("tlc-min-erase-count"),
		TlcAvgEraseCount:               storageEntry.Get("tlc-avg-erase-count"),
		WearLevelingCount:              storageEntry.Get("wear-leveling-count"),
		ProgramFailCount:               storageEntry.Get("program-fail-count"),
		EraseFailCount:                 storageEntry.Get("erase-fail-count"),
		PowerOffRetractCount:           storageEntry.Get("power-off-retract-count"),
		Temperature:                    storageEntry.Get("temperature-celsius"),
		HardwareEccRecovered:           storageEntry.Get("hardware-ecc-recovered"),
		ReallocationEventCount:         storageEntry.Get("reallocation-event-count"),
		UdmaCrcErrorsCount:             storageEntry.Get("udma-crc-error"),
		AvailableReservedSpace:         storageEntry.Get("available-reserved-space"),
		WriteSectorCount:               storageEntry.Get("host-writes"),
		ReadSectorCount:                storageEntry.Get("read-sector-count"),
		FlashWriteCount:                storageEntry.Get("nand-writes"),
	}
	return storageInfo, nil
}

/* Filling in the state info for storage available in Redis DB */
func fillStorageInfo(comp *ocbinds.OpenconfigPlatform_Components_Component,
	name string, pType PathType, targetUriPath string, stdb *db.DB) error {
	statePresent := true
	storageInfo, err := getStorageFromDb(name, stdb, STORAGE_INFO)
	if err != nil {
		log.V(lvl.DEBUG).Info("Error Getting Storage info from State DB: ", err.Error())
		statePresent = false
		if pType == StatePaths {
			return err
		}
	}
	if (pType == AllPaths || pType == AllCompPaths) && !statePresent {
		return fmt.Errorf("State DB entries not found for all paths; Error: %w", err)
	}
	compState := comp.State
	ygot.BuildEmptyTree(compState)
	compTemp := compState.Temperature

	if targetUriPath == COMP || targetUriPath == COMP_CFG || targetUriPath == COMP_CONFIG_NAME {
		comp.Config.Name = &name
	}

	// Filling in state values: name, type and io-errors
	if ((pType == AllPaths || pType == AllCompPaths) && statePresent) || pType == StatePaths {
		compState.Type, _ = compState.To_OpenconfigPlatform_Components_Component_State_Type_Union(
			ocbinds.OpenconfigPlatformTypes_OPENCONFIG_HARDWARE_COMPONENT_STORAGE)
		compState.Name = &name
		if storageInfo.PartNo != "" {
			compState.PartNo = &storageInfo.PartNo
		}
		if storageInfo.SerialNo != "" {
			compState.SerialNo = &storageInfo.SerialNo
		}
		compState.Parent = StringWithDefault(&storageInfo.Parent, "Missing in State DB")
		compState.Removable = String2Bool(&storageInfo.Removable, false)
		if operStatus, err := operStatusFromString(storageInfo.OperStatus); err == nil {
			compState.OperStatus = operStatus
		}
		if storageInfo.Empty != "" {
			compState.Empty = String2Bool(&storageInfo.Empty, false)
		}

		compTemp.Instant = String2Float(storageInfo.Temperature, 0.0)
	}

	if pType != SingularPath {
		return nil
	}

	switch targetUriPath {
	case COMP_STATE_NAME:
		compState.Name = &name
	case COMP_STATE_PART_NO:
		if storageInfo.PartNo != "" {
			compState.PartNo = &storageInfo.PartNo
		}
	case COMP_STATE_SERIAL_NO:
		if storageInfo.SerialNo != "" {
			compState.SerialNo = &storageInfo.SerialNo
		}
	case COMP_STATE_TYPE:
		compState.Type, _ = compState.To_OpenconfigPlatform_Components_Component_State_Type_Union(
			ocbinds.OpenconfigPlatformTypes_OPENCONFIG_HARDWARE_COMPONENT_STORAGE)
	case COMP_STATE_PARENT:
		compState.Parent = StringWithDefault(&storageInfo.Parent, "Missing in State DB")
	case COMP_STATE_OPER_STATUS:
		if operStatus, err := operStatusFromString(storageInfo.OperStatus); err == nil {
			compState.OperStatus = operStatus
		}
	case COMP_STATE_EMPTY:
		if storageInfo.Empty != "" {
			compState.Empty = String2Bool(&storageInfo.Empty, false)
		}
	case COMP_STATE_REMOVABLE:
		compState.Removable = String2Bool(&storageInfo.Removable, false)
	case COMP_STATE_TEMP_CTR, COMP_STATE_TEMP:
		compTemp.Instant = String2Float(storageInfo.Temperature, 0.0)
	}
	return nil
}

func getDiskCarrierInfoFromDb(name string, d *db.DB) (DiskCarrierInfo, error) {
	diskCarrierEntry, err := d.GetEntry(&db.TableSpec{Name: DISK_CARRIER_TBL}, db.Key{Comp: []string{name}})
	if err != nil {
		log.V(lvl.DEBUG).Info("Cant get entry: ", name, "; Error: ", err)
		return DiskCarrierInfo{}, err
	}
	return DiskCarrierInfo{
		Model_Name:    diskCarrierEntry.Get("model"),
		Name:          diskCarrierEntry.Get("name"),
		Parent:        diskCarrierEntry.Get("parent"),
		Part_Number:   diskCarrierEntry.Get("part-no"),
		Serial_Number: diskCarrierEntry.Get("serial"),
	}, nil
}

/* Filling in the state info for disk carrier components available in Redis DB */
func fillDiskCarrierInfo(comp *ocbinds.OpenconfigPlatform_Components_Component,
	name string, pType PathType, targetUriPath string, stdb *db.DB) error {
	diskCarrierInfo, err := getDiskCarrierInfoFromDb(name, stdb)
	if err != nil {
		log.V(lvl.DEBUG).Info("Error Getting disk carrier info from State DB: ", err.Error())
		return err
	}
	compState := comp.State
	defaultVal := ""
	defaultParentVal := CHASSIS_PREFIX

	if shouldExportLeaf(pType, targetUriPath, StateLeaf, COMP_STATE_NAME) {
		compState.Name = StringWithDefault(&diskCarrierInfo.Name, name)
	}
	if shouldExportLeaf(pType, targetUriPath, StateLeaf, COMP_STATE_TYPE) {
		compState.Type, _ = compState.To_OpenconfigPlatform_Components_Component_State_Type_Union(
			ocbinds.OpenconfigPlatformTypes_OPENCONFIG_HARDWARE_COMPONENT_STORAGE)
	}
	if shouldExportLeaf(pType, targetUriPath, StateLeaf, COMP_STATE_DESCR) {
		compState.Description = StringWithDefault(&diskCarrierInfo.Model_Name, defaultVal)
	}
	if shouldExportLeaf(pType, targetUriPath, StateLeaf, COMP_STATE_PARENT) {
		compState.Parent = StringWithDefault(&diskCarrierInfo.Parent, defaultParentVal)
	}
	if shouldExportLeaf(pType, targetUriPath, StateLeaf, COMP_STATE_MODEL_NAME) {
		compState.ModelName = StringWithDefault(&diskCarrierInfo.Model_Name, defaultVal)
	}
	if shouldExportLeaf(pType, targetUriPath, StateLeaf, COMP_STATE_PART_NO) {
		compState.PartNo = StringWithDefault(&diskCarrierInfo.Part_Number, diskCarrierInfo.Model_Name)
	}
	if shouldExportLeaf(pType, targetUriPath, StateLeaf, COMP_STATE_SERIAL_NO) {
		compState.SerialNo = StringWithDefault(&diskCarrierInfo.Serial_Number, defaultVal)
	}

	return nil
}

func validXcvrName(name *string) bool {
	if name == nil || *name == "" {
		return false
	}

	/* Expect tranceiver name of form EthernetX, where X is an integer */
	if !strings.HasPrefix(*name, XCVR_KEY_PREFIX) {
		return false
	}

	sp := strings.SplitAfter(*name, "Ethernet")

	if _, err := strconv.Atoi(sp[1]); err != nil {
		return false
	}
	return true
}

func getXcvrStatusInfoFromDb(name string, d *db.DB) (XcvrInfo, error) {
	xcvrStatusEntry, err := d.GetEntry(&db.TableSpec{Name: TRANSCEIVER_STATUS}, db.Key{Comp: []string{name}})
	if err != nil {
		return XcvrInfo{}, err
	}

	var xcvrStatus XcvrInfo
	xcvrStatus.ForceLowPower = xcvrStatusEntry.Get(FORCE_LOW_POWER)

	status := xcvrStatusEntry.Get("status")
	switch status {
	case SFP_STATUS_INSERTED:
		xcvrStatus.Presence = true
	case SFP_STATUS_REMOVED, "":
		xcvrStatus.Presence = false
	default:
		return XcvrInfo{}, fmt.Errorf("Unknown status for transceiver %s: %s", name, status)
	}
	return xcvrStatus, nil
}

func getXcvrInfoFromDb(name string, d *db.DB) (XcvrInfo, error) {
	var xcvrInfo XcvrInfo
	var err error

	xcvrEntry, err := d.GetEntry(&db.TableSpec{Name: TRANSCEIVER_TBL}, db.Key{Comp: []string{name}})

	if err != nil {
		xcvrInfo.Presence = false
		return xcvrInfo, err
	}

	/* Existence of entry implies presence */
	xcvrInfo.Presence = true
	xcvrInfo.EthPmd = xcvrEntry.Get("ethernet-pmd")
	xcvrInfo.Parent = xcvrEntry.Get("parent")
	xcvrInfo.MfgName = xcvrEntry.Get("manufacturer")
	xcvrInfo.MfgDate = xcvrEntry.Get("vendor_date")
	xcvrInfo.PartNo = xcvrEntry.Get("model")
	xcvrInfo.SerialNo = xcvrEntry.Get("serial")
	xcvrInfo.HardwareRev = xcvrEntry.Get("hardware_rev")
	xcvrInfo.Type = xcvrEntry.Get("type")
	xcvrInfo.LatestFirmwareVer = xcvrEntry.Get("latest_fw_rev")

	xcvrDOMEntry, err := d.GetEntry(&db.TableSpec{Name: TRANSCEIVER_DOM}, db.Key{Comp: []string{name}})
	if err != nil {
		xcvrInfo.Presence = false
		return xcvrInfo, err
	}

	for i := 0; i < XCVR_LANE_LIMIT; i++ {
		xcvrInfo.Lanes[i].RxPowerLane = xcvrDOMEntry.Get(fmt.Sprintf("rx%dpower", i+1))
		xcvrInfo.Lanes[i].TxBiasLane = xcvrDOMEntry.Get(fmt.Sprintf("tx%dbias", i+1))
		xcvrInfo.Lanes[i].TxPowerLane = xcvrDOMEntry.Get(fmt.Sprintf("tx%dpower", i+1))
		xcvrInfo.Lanes[i].TxDisable = xcvrDOMEntry.Get(fmt.Sprintf("tx%ddisable", i+1))
	}

	xcvrInfo.Temperature = xcvrDOMEntry.Get("temperature")
	xcvrInfo.ModuleState = xcvrDOMEntry.Get("module_state")

	return xcvrInfo, err
}

// sfp_type_to_max_lanes_map mapping pulled from third_party/sonic-platform-daemons/sonic-xcvrd/xcvrd/xcvrd.py
var sfpTypeToMaxLanesMap = map[string]int{
	"SFP/SFP+/SFP28":                1,
	"QSFP":                          4,
	"QSFP+ or later":                4,
	"QSFP28 or later":               4,
	"OSFP 8X Pluggable Transceiver": 8,
	"QSFP-DD Double Density 8X Pluggable Transceiver": 8,
}

func convAndFillDBValues(rxpField, txpField, txbField, txdisableField string, channel *ocbinds.OpenconfigPlatform_Components_Component_Transceiver_PhysicalChannels_Channel) {
	if rxpField != "" {
		if rxPower, err := strconv.ParseFloat(rxpField, 64); err == nil {
			channel.State.InputPower.Instant = &rxPower
		} else {
			log.V(lvl.DEBUG).Infof("Error converting rxPower (\"%s\") from string to float64", rxpField)
		}
	}
	if txpField != "" {
		if txPower, err := strconv.ParseFloat(txpField, 64); err == nil {
			channel.State.OutputPower.Instant = &txPower
		} else {
			log.V(lvl.DEBUG).Infof("Error converting txPower (\"%s\") from string to float64", txpField)
		}
	}
	if txbField != "" {
		if txBias, err := strconv.ParseFloat(txbField, 64); err == nil {
			channel.State.LaserBiasCurrent.Instant = &txBias
		} else {
			log.V(lvl.DEBUG).Infof("Error converting txBias (\"%s\") from string to float64", txbField)
		}
	}
	txLaserEnable := false
	if txdisableField == "False" || txdisableField == "false" {
		txLaserEnable = true
	}
	channel.State.TxLaser = &txLaserEnable
}

func fetchAllPortsFromParentPortWithLanes(port string, appstdb *db.DB, maxLanes int) (map[uint16]string, error) {
	laneToPortMap := make(map[uint16]string)
	childPortList, err := platform.PrimaryIntfToChildIntfs(port)
	if err != nil {
		return nil, err
	}
	// Loop through all possible child ports and add to laneToPortMap only if
	// the Appl DB entry is present and has the "lanes" field.
	ts := db.TableSpec{Name: PORT_TBL}
	for _, intfName := range childPortList {
		key := db.Key{Comp: []string{intfName}}
		dbEntry, err := appstdb.GetEntry(&ts, key)
		if err != nil {
			continue
		}
		dbLanes := dbEntry.Get("lanes")
		if dbLanes == "" {
			continue
		}

		lanesList := strings.Split(dbLanes, ",")
		// Convert the lane set to physical channels list based on maxLanes per xcvr type:
		// E.g. ["9","10","11","12"] gets converted to ["0","1","2","3"]
		// For SFP+ ports, ["257"] / ["258"] gets converted to ["0"]
		for _, lane := range lanesList {
			val, err := strconv.ParseUint(lane, 10, 16)
			if err != nil {
				log.V(lvl.DEBUG).Info("error in strconv: " + err.Error())
				return nil, err
			}
			laneToPortMap[(uint16(val)-1)%uint16(maxLanes)] = intfName
		}
	}
	// len(laneToPortMap) should be atleast 1 to account for parent port itself.
	if len(laneToPortMap) == 0 {
		log.V(lvl.DEBUG).Info("no ports found in Appl DB for parent port " + port)
		return nil, tlerr.InvalidArgs("no ports found in Appl DB for parent port " + port)
	}
	return laneToPortMap, nil
}

func findAssociatedIntfForXcvrLane(xcvrName string, laneIdx uint16, maxLanes int, appstdb *db.DB) string {
	// Derive the parent port from transceiver name.
	parentIntfName := strings.Replace(xcvrName, "Ethernet", "Ethernet1/", 1)
	if maxLanes != 1 {
		parentIntfName += "/1"
	}
	// Fetch all existing child ports from parent port in Appl DB.
	var err error
	if appstdb == nil {
		appstdb, err = db.NewDB(getDBOptions(db.ApplDB))
		if err != nil {
			log.V(lvl.DEBUG).Info("could not create ApplDB instance: " + err.Error())
			return ""
		}
		defer appstdb.DeleteDB()
	}
	// laneToPortMap should return <laneIndex: interfaceName> mapping.
	laneToPortMap, err := fetchAllPortsFromParentPortWithLanes(parentIntfName, appstdb, maxLanes)
	if err != nil {
		return ""
	}
	// Lookup the associated interface using laneIdx.
	intf, ok := laneToPortMap[laneIdx]
	if !ok {
		log.V(lvl.DEBUG).Infof("invalid lane %v for parent port %v and xcvr %v", laneIdx, parentIntfName, xcvrName)
		return ""
	}
	return intf
}

func fillXcvrLaneInfo(xcvrCom *ocbinds.OpenconfigPlatform_Components_Component, laneIdx uint16, xcvrInfo XcvrInfo, name string, maxLanes int, d *db.DB) (err error) {
	channel, ok := xcvrCom.Transceiver.PhysicalChannels.Channel[laneIdx]
	if !ok || channel == nil {
		channel, err = xcvrCom.Transceiver.PhysicalChannels.NewChannel(laneIdx)
		if err != nil {
			return fmt.Errorf("cannot create channel object: %w", err)
		}
	}
	ygot.BuildEmptyTree(channel)
	ygot.BuildEmptyTree(channel.Config)
	ygot.BuildEmptyTree(channel.State)
	channel.Config.Index = &laneIdx
	channel.State.Index = &laneIdx

	if laneIdx < XCVR_LANE_LIMIT {
		lane := &xcvrInfo.Lanes[laneIdx]
		convAndFillDBValues(lane.RxPowerLane, lane.TxPowerLane, lane.TxBiasLane, lane.TxDisable, channel)
	} else {
		return errors.New("lane index is invalid.")
	}
	return nil
}

func fillXcvrInfo(xcvrCom *ocbinds.OpenconfigPlatform_Components_Component,
	name string, all bool, laneIdx string, targetUriPath string, dbs [db.MaxDB]*db.DB) error {
	var err error

	log.V(lvl.DEBUG).Infof("fillXcvrInfo: name %s, all %v laneIdx %s targetUriPath %s", name, all, laneIdx, targetUriPath)

	d := dbs[db.StateDB]
	if d == nil {
		d, err = db.NewDB(getDBOptions(db.StateDB))
		if err != nil {
			return tlerr.InvalidArgsError{Format: err.Error()}
		}
		defer d.DeleteDB()
	}
	cfgdb := dbs[db.ConfigDB]
	if cfgdb == nil {
		cfgdb, err = db.NewDB(getDBOptions(db.ConfigDB))
		if err != nil {
			return tlerr.InvalidArgsError{Format: err.Error()}
		}
		defer cfgdb.DeleteDB()
	}

	xcvrStatusState, err := getXcvrStatusInfoFromDb(name, d)
	if err != nil {
		log.V(lvl.DEBUG).Info("Error Getting transceiver status from dB")
		return err
	}
	nm := name
	xcvrEEPROMState := xcvrCom.State
	xcvrEEPROMState.Name = &nm
	var xcvrInfo XcvrInfo
	if !xcvrStatusState.Presence {
		p := !xcvrStatusState.Presence
		xcvrEEPROMState.Empty = &p
	} else {
		xcvrInfo, err = getXcvrInfoFromDb(name, d)
		if err != nil {
			log.V(lvl.DEBUG).Info("Error Getting transceiver info from dB")
			return err
		}
	}

	if xcvrInfo.Type != "" && laneIdx != "" {
		maxLanes, ok := sfpTypeToMaxLanesMap[xcvrInfo.Type]
		if !ok {
			return errors.New("could not find the max number of lanes for transceiver.")
		}
		idx, err := strconv.ParseUint(laneIdx, 10, 16)
		if err != nil {
			return err
		}
		if idx >= uint64(maxLanes) {
			return errors.New("lane index greater than the max number of lanes for transceiver.")
		}
		fillXcvrLaneInfo(xcvrCom, uint16(idx), xcvrInfo, name, maxLanes, dbs[db.ApplDB])
	}

	xcvrState := xcvrCom.Transceiver.State

	if all {

		/* Top level */
		xcvrEEPROMState.Name = &nm
		xcvrCom.Config.Name = &nm

		/* Present state */
		p := !xcvrInfo.Presence
		xcvrEEPROMState.Empty = &p

		q := true
		xcvrEEPROMState.Removable = &q

		xcvrEEPROMState.Type, _ = xcvrCom.State.To_OpenconfigPlatform_Components_Component_State_Type_Union(
			ocbinds.OpenconfigPlatformTypes_OPENCONFIG_HARDWARE_COMPONENT_TRANSCEIVER)

		if test_if_available(xcvrInfo.Parent) {
			xcvrEEPROMState.Parent = &xcvrInfo.Parent
		}

		/* Vendor info */
		if xcvrInfo.SerialNo != "" {
			xcvrEEPROMState.SerialNo = &xcvrInfo.SerialNo
		}
		if xcvrInfo.PartNo != "" {
			xcvrEEPROMState.PartNo = &xcvrInfo.PartNo
		}
		if xcvrInfo.MfgName != "" {
			xcvrEEPROMState.MfgName = &xcvrInfo.MfgName
		}
		if xcvrInfo.HardwareRev != "" {
			xcvrEEPROMState.HardwareVersion = &xcvrInfo.HardwareRev
			// Using the 'hardware_rev' field to also populate the firmware-version path.
			xcvrEEPROMState.FirmwareVersion = &xcvrInfo.HardwareRev
		}
		if xcvrInfo.MfgDate != "" {
			xcvrEEPROMState.MfgDate = &xcvrInfo.MfgDate
		}
		/* Not present */
		if xcvrInfo.Presence {
			xcvrEEPROMState.OperStatus = ocbinds.OpenconfigPlatformTypes_COMPONENT_OPER_STATUS_ACTIVE
		}

		if xcvrInfo.Temperature != "" {
			if float64val, err := strconv.ParseFloat(xcvrInfo.Temperature, 64); err == nil {
				xcvrEEPROMState.Temperature.Instant = &float64val
			}
		}

		/* Physical-Channels level */
		if xcvrInfo.Type != "" && laneIdx == "" {
			if maxLanes, ok := sfpTypeToMaxLanesMap[xcvrInfo.Type]; ok {
				for i := 0; i < maxLanes; i++ {
					fillXcvrLaneInfo(xcvrCom, uint16(i), xcvrInfo, name, maxLanes, dbs[db.ApplDB])
				}
			} else {
				log.V(lvl.DEBUG).Info("Could not find the max number of lanes for transceiver.")
			}
		}

		if xcvrInfo.Type != "" {
			xcvrState.FormFactor = formFactorTypeFromString(xcvrInfo.Type)
		}
		xcvrState.EthernetPmd = ethPmdTypeFromString(xcvrInfo.EthPmd)

		/*
		       Pending YANG updates
		   if (test_if_available(xcvrInfo.Module_Lane_Count)){
		       tmp, err := strconv.ParseUint(xcvrInfo.Module_Lane_Count, 10, 64)
		       if err == nil {
		           q := uint32(tmp)
		           xcvrState.ModuleLaneCount = &q
		       }
		   }
		   if (test_if_available(xcvrInfo.Lpmode)){
		       tmp, err := strconv.ParseBool(xcvrInfo.Lpmode)
		       if err == nil {
		           xcvrState.Lpmode = &tmp
		       }
		   }


		   if (test_if_available(xcvrInfo.Media_Interface)){
		       xcvrState.MediaInterface = &xcvrInfo.Media_Interface
		   }
		   if (test_if_available(xcvrInfo.Cable_Type)){
		       xcvrState.CableType = &xcvrInfo.Cable_Type
		   }

		   if (test_if_available(xcvrInfo.Qsa_Adapter_Type)){
		       xcvrState.QsaAdapterType = &xcvrInfo.Qsa_Adapter_Type
		   }
		*/
		return err
	}

	switch targetUriPath {
	case COMP_STATE_EMPTY:
		q := !xcvrInfo.Presence
		xcvrEEPROMState.Empty = &q
	case COMP_STATE_NAME:
		nm := name
		xcvrEEPROMState.Name = &nm
	case COMP_CONFIG_NAME:
		nm := name
		xcvrCom.Config.Name = &nm
	case COMP_STATE_TYPE:
		xcvrEEPROMState.Type, _ = xcvrCom.State.To_OpenconfigPlatform_Components_Component_State_Type_Union(
			ocbinds.OpenconfigPlatformTypes_OPENCONFIG_HARDWARE_COMPONENT_TRANSCEIVER)
	case COMP_STATE_PARENT:
		if test_if_available(xcvrInfo.Parent) {
			xcvrEEPROMState.Parent = &xcvrInfo.Parent
		}
	case COMP_STATE_SERIAL_NO:
		if xcvrInfo.SerialNo != "" {
			xcvrEEPROMState.SerialNo = &xcvrInfo.SerialNo
		}
	case COMP_STATE_PART_NO:
		if xcvrInfo.PartNo != "" {
			xcvrEEPROMState.PartNo = &xcvrInfo.PartNo
		}
	case COMP_STATE_MFG_NAME:
		if xcvrInfo.MfgName != "" {
			xcvrEEPROMState.MfgName = &xcvrInfo.MfgName
		}
	case COMP_STATE_HW_VER:
		if xcvrInfo.HardwareRev != "" {
			xcvrEEPROMState.HardwareVersion = &xcvrInfo.HardwareRev
		}
	// Using the 'hardware_rev' field to also populate the firmware-version path.
	case COMP_STATE_FIRM_VER:
		if xcvrInfo.HardwareRev != "" {
			xcvrEEPROMState.FirmwareVersion = &xcvrInfo.HardwareRev
		}
	case COMP_STATE_MFG_DATE:
		if xcvrInfo.MfgDate != "" {
			xcvrEEPROMState.MfgDate = &xcvrInfo.MfgDate
		}
	case COMP_STATE_OPER_STATUS:
		xcvrEEPROMState.OperStatus = ocbinds.OpenconfigPlatformTypes_COMPONENT_OPER_STATUS_ACTIVE
	case COMP_STATE_TEMP:
		if xcvrInfo.Temperature != "" {
			float64val, err := strconv.ParseFloat(xcvrInfo.Temperature, 64)
			if err != nil {
				return err
			}
			xcvrEEPROMState.Temperature.Instant = &float64val
		}

	case COMP_STATE_REMOVABLE:
		q := true
		xcvrEEPROMState.Removable = &q

	case XCVR_FORM_FACTOR:
		if xcvrInfo.Type != "" {
			xcvrState.FormFactor = formFactorTypeFromString(xcvrInfo.Type)
		}
	case XCVR_ETH_PMD:
		xcvrState.EthernetPmd = ethPmdTypeFromString(xcvrInfo.EthPmd)
	}
	return err
}

var tempSensorRe = regexp.MustCompile(`(exhaust|inlet|heatsink|dimm|sfp)_sensor(_[0-9\w]+)?$`)
var cpuTempSensorRe = regexp.MustCompile(`cpu_sensor_?[0-9\w]+$`)

func validTempSensorName(name *string) bool {
	if name == nil || *name == "" {
		return false
	}
	// For TEMPERATURE_INFO, SONiC supported keys are "exhaust_sensor_x", "inlet_sensor_x", "heatsink_sensor_x", "dimm_sensor_x",
	// "exhaust_sensor", "inlet_sensor", "heatsink_sensor", and "dimm_sensor"
	if tempSensorRe.MatchString(*name) {
		return true
	}

	return strings.HasPrefix(*name, "TEMP")
}

func validCpuTempSensorName(name string) bool {
	if name == "" {
		return false
	}
	// For CPU component, SONiC supported keys are "cpu_sensor_x", and "cpu_sensor"
	if valid := cpuTempSensorRe.MatchString(name); valid {
		return valid
	}
	return false
}

func getTempSensorInfoFromDb(name string, d *db.DB) (TempSensorInfo, error) {
	tempEntry, err := d.GetEntry(&db.TableSpec{Name: TEMP_TBL}, db.Key{Comp: []string{name}})
	if err != nil {
		return TempSensorInfo{}, err
	}

	tempInfo := TempSensorInfo{
		Current:             tempEntry.Get("temperature"),
		Name:                tempEntry.Get("name"),
		Description:         tempEntry.Get("description"),
		Crit_High_Threshold: tempEntry.Get("critical_high_threshold"),
		Crit_Low_Threshold:  tempEntry.Get("critical_low_threshold"),
		High_Threshold:      tempEntry.Get("high_threshold"),
		Low_Threshold:       tempEntry.Get("low_threshold"),
		Timestamp:           tempEntry.Get("timestamp"),
		Warning_Status:      tempEntry.Get("warning_status"),
		EntityInfo:          entityInfo(name, d),
	}

	return tempInfo, nil
}

func fillTempSensorInfo(temp *ocbinds.OpenconfigPlatform_Components_Component,
	name string, pType PathType, targetUriPath string, d *db.DB) error {
	isCpuSensor := validCpuTempSensorName(name)
	tempInfo, err := getTempSensorInfoFromDb(name, d)
	if err != nil {
		return err
	}

	tempCom := temp.State.Temperature
	tempState := temp.State
	defaultParentVal := CHASSIS_PREFIX

	if pType == AllPaths || pType == AllCompPaths || pType == StatePaths || targetUriPath == TEMP_COMP {
		tempState.Name = &name
		tempState.Location = &name
		temp.Config.Name = &name
		tempState.Description = NilIfStringEmpty(&tempInfo.Description)
		tempState.Parent = StringWithDefault(&tempInfo.EntityInfo.Parent, defaultParentVal)
		tempState.InstallPosition = NilIfStringEmpty(&tempInfo.EntityInfo.PositionInParent)
		tempState.InstallComponent = StringWithDefault(&tempInfo.EntityInfo.InstallComponent, *tempState.Parent)
		tempState.Type, _ = tempState.To_OpenconfigPlatform_Components_Component_State_Type_Union(
			ocbinds.OpenconfigPlatformTypes_OPENCONFIG_HARDWARE_COMPONENT_SENSOR)
		if isCpuSensor {
			tempState.Type, _ = tempState.To_OpenconfigPlatform_Components_Component_State_Type_Union(
				ocbinds.OpenconfigPlatformTypes_OPENCONFIG_HARDWARE_COMPONENT_CPU)
		}
		if tempInfo.Current != "" {
			cur, terr := strconv.ParseFloat(tempInfo.Current, 64)
			if terr != nil {
				log.V(lvl.DEBUG).Infof("Error in ParseFloat conversion for tempInfo.Current (%v): %v", tempInfo.Current, terr)
			} else {
				tempCom.Instant = &cur
			}
		}

		if pType == StatePaths {
			return nil
		}
		return nil
	}

	switch targetUriPath {
	case COMP_CONFIG_NAME:
		temp.Config.Name = &name
	case COMP_STATE_NAME:
		tempState.Name = &name
		if tempInfo.Name != "" {
			tempState.Name = &tempInfo.Name
		}
	case COMP_STATE_DESCR:
		tempState.Description = NilIfStringEmpty(&tempInfo.Description)
	case COMP_STATE_LOCATION:
		tempState.Location = &name
	case COMP_STATE_PARENT:
		tempState.Parent = StringWithDefault(&tempInfo.EntityInfo.Parent, defaultParentVal)
	case COMP_STATE_INSTALL_POSITION:
		tempState.InstallPosition = NilIfStringEmpty(&tempInfo.EntityInfo.PositionInParent)
	case COMP_STATE_INSTALL_COMPONENT:
		p := StringWithDefault(&tempInfo.EntityInfo.Parent, defaultParentVal)
		tempState.InstallComponent = StringWithDefault(&tempInfo.EntityInfo.InstallComponent, *p)
	case COMP_STATE_TYPE:
		tempState.Type, _ = tempState.To_OpenconfigPlatform_Components_Component_State_Type_Union(
			ocbinds.OpenconfigPlatformTypes_OPENCONFIG_HARDWARE_COMPONENT_SENSOR)
		if isCpuSensor {
			tempState.Type, _ = tempState.To_OpenconfigPlatform_Components_Component_State_Type_Union(
				ocbinds.OpenconfigPlatformTypes_OPENCONFIG_HARDWARE_COMPONENT_CPU)
		}
	case TEMP_INSTANT:
		if tempInfo.Current != "" {
			cur, terr := strconv.ParseFloat(tempInfo.Current, 64)
			if terr != nil {
				return terr
			}
			tempCom.Instant = &cur
		}
	default:
		return fmt.Errorf("unsupported leaf path for Sensor/CPU component: %v", targetUriPath)
	}

	return nil
}

func validICName(name *string) bool {
	if name == nil || *name == "" {
		return false
	}
	// Expect node name of form integrated-circuitX, where X is an integer
	if !strings.HasPrefix(*name, IC_NAME_PREFIX) {
		return false
	}

	sp := strings.SplitAfter(*name, IC_NAME_PREFIX)
	if len(sp) < 2 {
		return false
	}

	if _, err := strconv.Atoi(sp[1]); err != nil {
		return false
	}
	return true
}

func getICInfoFromDb(name string, d *db.DB, tblName string) ICInfo {
	var nodeInfo ICInfo

	switch tblName {
	case NODE_CFG_TBL:
		nodeEntry, err := d.GetEntry(&db.TableSpec{Name: tblName}, db.Key{Comp: []string{name}})
		if err != nil {
			log.V(lvl.DEBUG).Info("Cant get entry: ", name, "; Error: ", err)
			return nodeInfo
		}
		nodeInfo.Node_Id = nodeEntry.Get("node-id")
		nodeInfo.Name = nodeEntry.Get("name")
		nodeInfo.Parent = nodeEntry.Get("parent")
		nodeInfo.QualifiedName = nodeEntry.Get("fully-qualified-name")
	case SWITCH_EVENT:
		switchEvents, err := d.GetEntry(&db.TableSpec{Name: tblName}, db.Key{Comp: []string{"PARITY_ERROR"}})
		if err != nil {
			log.V(lvl.DEBUG).Info("Cant get parity errors from switch event table ", err)
			return nodeInfo
		}
		nodeInfo.UnrecoverableParityErrors = *String2Uint(switchEvents.Get("unrecoverable_parity_errors"), 0)
		nodeInfo.CorrectedParityErrors = *String2Uint(switchEvents.Get("recovered_parity_errors"), 0)
	}
	return nodeInfo
}

func getSwitchOidFromDb(asicDb *db.DB) (string, error) {
	keys, err := asicDb.GetKeysPattern(&db.TableSpec{Name: "ASIC_STATE"}, db.Key{Comp: []string{"SAI_OBJECT_TYPE_SWITCH*"}})
	if err != nil {
		return "", err
	}
	if len(keys) != 1 {
		return "", tlerr.NotFoundError{Format: "getALPMCounterTable: incorrect number of keys match the pattern"}
	}

	// Key should look like "ASIC_STATE:SAI_OBJECT_TYPE_SWITCH:<Switch OID>"
	key := keys[0]
	if key.Len() != 3 {
		return "", tlerr.NotFoundError{Format: "getALPMCounterTable: invalid key in ASIC DB"}
	}

	return key.Get(1) + ":" + key.Get(2), nil
}

func getAlpmCounterFromDb(ctrsDb, asicDb *db.DB) uint64 {
	var err error

	// Get the oid and field names of the counters
	oid, err := getSwitchOidFromDb(asicDb)
	if err != nil {
		log.V(lvl.DEBUG).Infof("Failed to get Switch Oid: %v", err)
		return 0
	}
	fieldNames, err := ctrsDb.GetEntry(&db.TableSpec{Name: "COUNTERS_DEBUG_NAME_SWITCH_STAT_MAP"}, db.Key{Comp: []string{}})
	if err != nil {
		log.V(tlerr.ErrorSeverity(err)).Infof("Failed to get entry: %v", err)
		return 0
	}
	field1 := fieldNames.Get("SAI_IN_DROP_REASON_LPM4_MISS")
	field2 := fieldNames.Get("SAI_IN_DROP_REASON_LPM6_MISS")

	entry, err := ctrsDb.GetEntry(&db.TableSpec{Name: "COUNTERS"}, db.Key{Comp: []string{oid}})
	if err != nil {
		log.V(tlerr.ErrorSeverity(err)).Infof("Failed to get entry: %v", err)
		return 0
	}

	ctr1, err := strconv.ParseUint(entry.Get(field1), 0, 64)
	if err != nil {
		log.V(lvl.DEBUG).Infof("Failed to convert string to uint: %v", err)
		ctr1 = 0
	}
	ctr2, err := strconv.ParseUint(entry.Get(field2), 0, 64)
	if err != nil {
		log.V(lvl.DEBUG).Infof("Failed to convert string to uint: %v", err)
		ctr2 = 0
	}

	return ctr1 + ctr2
}

/* Filling in the config and state info for integrated circuits available in Redis DB */
func fillICInfo(comp *ocbinds.OpenconfigPlatform_Components_Component,
	name string, targetUriPath string, dbs [db.MaxDB]*db.DB, ygRoot *ygot.GoStruct) error {
	/* Integrated-circuits have the following subtrees to populate:
	 *   ...component/config
	 *   ...component/state
	 *   ...component/integrated-circuit
	 *   ...component/integrated-circuit/config
	 *   ...component/integrated-circuit/state
	 *   ...component/integrated-circuit/state/blackhole
	 *   ...component/integrated-circuit/state/congestion
	 *   ...component/integrated-circuit/pipeline-counters
	 *   ...component/integrated-circuit/memory
	 * Decide now which subtrees to fill based on the request. */
	var all, allIc, compSt, compCfg, icCfg, icSt, icPlc, icMem bool
	if targetUriPath == COMP {
		all = true
	} else if strings.HasPrefix(targetUriPath, COMP_CFG) {
		compCfg = true
	} else if strings.HasPrefix(targetUriPath, COMP_ST) {
		compSt = true
	} else if strings.HasPrefix(targetUriPath, COMP_IC_CFG) {
		icCfg = true
	} else if strings.HasPrefix(targetUriPath, COMP_IC_ST) {
		icSt = true
	} else if strings.HasPrefix(targetUriPath, COMP_IC_PLC) {
		icPlc = true
	} else if strings.HasPrefix(targetUriPath, COMP_IC_MEM) {
		icMem = true
	} else if strings.HasPrefix(targetUriPath, COMP_IC) {
		allIc = true
	}
	log.V(lvl.DEBUG).Infof("dbToYangIC: name %s targetUriPath %s", name, targetUriPath)
	if !compSt && !compCfg {
		ygot.BuildEmptyTree(comp.IntegratedCircuit)
		if all || allIc || icCfg {
			ygot.BuildEmptyTree(comp.IntegratedCircuit.Config)
		}
		if all || allIc || icSt {
			ygot.BuildEmptyTree(comp.IntegratedCircuit.State)
		}
		if all || allIc || icPlc {
			ygot.BuildEmptyTree(comp.IntegratedCircuit.PipelineCounters)
			ygot.BuildEmptyTree(comp.IntegratedCircuit.PipelineCounters.Drop)
			ygot.BuildEmptyTree(comp.IntegratedCircuit.PipelineCounters.Drop.LookupBlock.State)
		}
		if all || allIc || icMem {
			ygot.BuildEmptyTree(comp.IntegratedCircuit.Memory)
			ygot.BuildEmptyTree(comp.IntegratedCircuit.Memory.State)
		}
	}

	cfgDb := dbs[db.ConfigDB]
	ctrsDb := dbs[db.CountersDB]
	asicDb := dbs[db.AsicDB]

	// Ignore error if unable to retrieve parity error counts from counters db
	nodeMem := getICInfoFromDb(name, ctrsDb, SWITCH_EVENT)
	nodeCfg := getICInfoFromDb(name, cfgDb, NODE_CFG_TBL)
	noRoute := getAlpmCounterFromDb(ctrsDb, asicDb)

	/* Handle component config paths.  Note that we always need "name" since
	 * is the list key. */
	comp.Config.Name = &name

	/* Handle component state paths: name, type, parent */
	if all || compSt {
		var stName, stType, stParent bool
		switch targetUriPath {
		case COMP_ST:
			stName, stType, stParent = true, true, true
		case COMP_STATE_NAME:
			stName = true
		case COMP_STATE_TYPE:
			stType = true
		case COMP_STATE_PARENT:
			stParent = true
		default:
			/* Unsupported path or /components/component */
		}
		if all || stName {
			comp.State.Name = &name
			if nodeCfg.Name != "" {
				comp.State.Name = &nodeCfg.Name
			}
		}
		if all || stType {
			comp.State.Type, _ = comp.State.To_OpenconfigPlatform_Components_Component_State_Type_Union(
				ocbinds.OpenconfigPlatformTypes_OPENCONFIG_HARDWARE_COMPONENT_INTEGRATED_CIRCUIT)
		}
		if all || stParent {
			parentChassis := CHASSIS_PREFIX
			comp.State.Parent = &parentChassis
			if nodeCfg.Parent != "" {
				comp.State.Parent = &nodeCfg.Parent
			}
		}
	}

	/* Handle component integrated-circuit pipeline counter paths (only the ALPM
	 * miss counter. */
	if all || allIc || icPlc {
		comp.IntegratedCircuit.PipelineCounters.Drop.LookupBlock.State.NoRoute = &noRoute
	}

	/* Handle component integrated-circuit memory paths */
	if all || allIc || icMem {
		totalParityErrors := nodeMem.CorrectedParityErrors + nodeMem.UnrecoverableParityErrors
		if targetUriPath != COMP_IC_MEM_TPE {
			comp.IntegratedCircuit.Memory.State.CorrectedParityErrors = &nodeMem.CorrectedParityErrors
		}
		if targetUriPath != COMP_IC_MEM_CPE {
			comp.IntegratedCircuit.Memory.State.TotalParityErrors = &totalParityErrors
		}
	}

	return nil
}

func getFirmwareInfoFromDb(name string, d *db.DB, tblName string) (FirmwareInfo, error) {
	firmwareEntry, err := d.GetEntry(&db.TableSpec{Name: tblName}, db.Key{Comp: []string{name}})
	if err != nil {
		log.V(lvl.DEBUG).Info("Cant get entry: ", name, "; Error: ", err)
		return FirmwareInfo{}, err
	}

	firmwareInfo := FirmwareInfo{AlarmStatus: false}
	if firmwareEntry.Get("alarm_status") == "true" {
		firmwareInfo.AlarmStatus = true
	}
	firmwareInfo.Description = firmwareEntry.Get("description")
	firmwareInfo.Name = firmwareEntry.Get("name")

	firmwareInfo.FirmwareVersion = firmwareEntry.Get("firmware-version")
	firmwareInfo.HardwareVersion = firmwareEntry.Get("hardware-version")
	firmwareInfo.MfgDate = firmwareEntry.Get("manufacture_date")
	firmwareInfo.OperStatus = firmwareEntry.Get("oper-status")
	firmwareInfo.PartNo = firmwareEntry.Get("part-no")
	firmwareInfo.SerialNo = firmwareEntry.Get("serial")
	firmwareInfo.QualifiedName = firmwareEntry.Get("fully-qualified-name")
	firmwareInfo.ModelName = firmwareEntry.Get("product_name")
	firmwareInfo.BaseMac = firmwareEntry.Get("base_mac_addr")
	firmwareInfo.MacPoolSize = firmwareEntry.Get("mac_addr_num")
	firmwareInfo.CpuType = firmwareEntry.Get("cpu-type")

	return firmwareInfo, nil
}

/* Filling in the config and state info for firmware chassis available in Redis DB */
/* EepromDb holds the subset of EEPROM_INFO attributes that are surfaced on the
 * "System Eeprom" component. The EEPROM_INFO table stores each attribute as its
 * own row (keyed by an index) with "Name"/"Value" fields, so it is flattened
 * into this struct before being mapped onto the OpenConfig state leaves.
 * This logic was ported from the standalone pfm_app.go so that this transformer
 * is the sole handler for /openconfig-platform:components. */
type EepromDb struct {
	Product_Name     string
	Part_Number      string
	Serial_Number    string
	Manufacture_Date string
	Label_Revision   string
	Platform_Name    string
	Manufacturer     string
	Vendor_Name      string
	Service_Tag      string
	Hardware_Version string
	Software_Version string
}

/* getSysEepromDbObj reads the EEPROM_INFO table and flattens its Name/Value rows
 * into a single EepromDb object. */
func getSysEepromDbObj(d *db.DB) EepromDb {
	var eepromDb EepromDb
	tbl, err := d.GetTable(&db.TableSpec{Name: EEPROM_TBL})
	if err != nil {
		log.V(lvl.DEBUG).Info("EEPROM_INFO table get failed: ", err.Error())
		return eepromDb
	}
	keys, _ := tbl.GetKeys()
	for _, key := range keys {
		e, kerr := tbl.GetEntry(key)
		if kerr != nil {
			continue
		}
		switch e.Get("Name") {
		case "Product Name":
			eepromDb.Product_Name = e.Get("Value")
		case "Part Number":
			eepromDb.Part_Number = e.Get("Value")
		case "Serial Number":
			eepromDb.Serial_Number = e.Get("Value")
		case "Manufacture Date":
			eepromDb.Manufacture_Date = e.Get("Value")
		case "Label Revision":
			eepromDb.Label_Revision = e.Get("Value")
		case "Platform Name":
			eepromDb.Platform_Name = e.Get("Value")
		case "Manufacturer":
			eepromDb.Manufacturer = e.Get("Value")
		case "Vendor Name":
			eepromDb.Vendor_Name = e.Get("Value")
		case "Service Tag":
			eepromDb.Service_Tag = e.Get("Value")
		case "Hardware Version":
			eepromDb.Hardware_Version = e.Get("Value")
		case "Software Version":
			eepromDb.Software_Version = e.Get("Value")
		}
	}
	return eepromDb
}

/* fillSysEepromInfo populates the synthetic "System Eeprom" component's state
 * from the EEPROM_INFO table. Ported from pfm_app.go's getSysEepromFromDb. */
func fillSysEepromInfo(comp *ocbinds.OpenconfigPlatform_Components_Component,
	name string, pType PathType, targetUriPath string, d *db.DB) error {
	eepromDb := getSysEepromDbObj(d)

	compState := comp.State
	empty := false
	removable := false
	sysName := SYS_EEPROM_NAME
	location := "Slot 1"

	if shouldExportLeaf(pType, targetUriPath, StateLeaf, COMP_STATE_NAME) {
		compState.Name = &sysName
	}
	if shouldExportLeaf(pType, targetUriPath, StateLeaf, COMP_STATE_LOCATION) {
		compState.Location = &location
	}
	if shouldExportLeaf(pType, targetUriPath, StateLeaf, COMP_STATE_EMPTY) {
		compState.Empty = &empty
	}
	if shouldExportLeaf(pType, targetUriPath, StateLeaf, COMP_STATE_REMOVABLE) {
		compState.Removable = &removable
	}
	if shouldExportLeaf(pType, targetUriPath, StateLeaf, COMP_STATE_OPER_STATUS) {
		compState.OperStatus = ocbinds.OpenconfigPlatformTypes_COMPONENT_OPER_STATUS_ACTIVE
	}
	if shouldExportLeaf(pType, targetUriPath, StateLeaf, COMP_STATE_ID) && eepromDb.Product_Name != "" {
		compState.Id = &eepromDb.Product_Name
	}
	if shouldExportLeaf(pType, targetUriPath, StateLeaf, COMP_STATE_PART_NO) && eepromDb.Part_Number != "" {
		compState.PartNo = &eepromDb.Part_Number
	}
	if shouldExportLeaf(pType, targetUriPath, StateLeaf, COMP_STATE_SERIAL_NO) {
		if eepromDb.Serial_Number != "" {
			compState.SerialNo = &eepromDb.Serial_Number
		} else if eepromDb.Service_Tag != "" {
			compState.SerialNo = &eepromDb.Service_Tag
		}
	}
	if shouldExportLeaf(pType, targetUriPath, StateLeaf, COMP_STATE_MFG_DATE) && eepromDb.Manufacture_Date != "" {
		compState.MfgDate = &eepromDb.Manufacture_Date
	}
	if shouldExportLeaf(pType, targetUriPath, StateLeaf, COMP_STATE_HW_VER) {
		if eepromDb.Hardware_Version != "" {
			compState.HardwareVersion = &eepromDb.Hardware_Version
		} else if eepromDb.Label_Revision != "" {
			compState.HardwareVersion = &eepromDb.Label_Revision
		}
	}
	if shouldExportLeaf(pType, targetUriPath, StateLeaf, COMP_STATE_DESCR) && eepromDb.Platform_Name != "" {
		compState.Description = &eepromDb.Platform_Name
	}
	if shouldExportLeaf(pType, targetUriPath, StateLeaf, COMP_STATE_MFG_NAME) {
		if eepromDb.Manufacturer != "" {
			compState.MfgName = &eepromDb.Manufacturer
		} else if eepromDb.Vendor_Name != "" {
			compState.MfgName = &eepromDb.Vendor_Name
		}
	}
	if shouldExportLeaf(pType, targetUriPath, StateLeaf, COMP_STATE_SW_VER) && eepromDb.Software_Version != "" {
		compState.SoftwareVersion = &eepromDb.Software_Version
	}
	return nil
}

func fillFirmwareInfo(comp *ocbinds.OpenconfigPlatform_Components_Component,
	name string, pType PathType, targetUriPath string, stdb *db.DB, cfgdb *db.DB) error {
	statePresent, cfgPresent := true, true
	firmwareInfo, err := getFirmwareInfoFromDb(name, stdb, CHASSIS_TBL)
	if err != nil {
		log.V(lvl.DEBUG).Info("Error Getting Chassis info from State DB: ", err.Error())
		statePresent = false
		if pType == StatePaths {
			return err
		}
	}
	firmwareCfg, err := getFirmwareInfoFromDb(name, cfgdb, CHASSIS_CFG)
	if err != nil {
		log.V(lvl.DEBUG).Info("Error Getting Chassis info from Config DB: ", err.Error())
		cfgPresent = false
		if pType == ConfigPaths {
			return err
		}
	}
	if (pType == AllPaths || pType == AllCompPaths) && !statePresent && !cfgPresent {
		return fmt.Errorf("Config and State DB entries not found for all paths; Error: %w", err)
	}

	compState := comp.State
	compCfg := comp.Config
	defaultVal := ""

	if ((pType == AllPaths || pType == AllCompPaths) && statePresent) || pType == StatePaths {
		// Filling in state values
		// State Type
		compState.Type, _ = compState.To_OpenconfigPlatform_Components_Component_State_Type_Union(
			ocbinds.OpenconfigPlatformTypes_OPENCONFIG_HARDWARE_COMPONENT_CHASSIS)
		// State Name
		compState.Name = &name
		if firmwareInfo.Name != "" {
			compState.Name = &firmwareInfo.Name
		}
		// State Description
		if firmwareInfo.Description != "" {
			compState.Description = &firmwareInfo.Description
		}
		// State Firmware Version
		compState.FirmwareVersion = &defaultVal
		if firmwareInfo.FirmwareVersion != "" {
			compState.FirmwareVersion = &firmwareInfo.FirmwareVersion
		}
		// State Hardware Version
		if firmwareInfo.HardwareVersion != "" {
			compState.HardwareVersion = &firmwareInfo.HardwareVersion
		}
		// State Mfg Date
		if mfg_date := reformatMfgDate(firmwareInfo.MfgDate); mfg_date != "" {
			compState.MfgDate = &mfg_date
		}
		// State Oper Status
		if firmwareInfo.OperStatus != "" {
			if operStatus, err := operStatusFromString(firmwareInfo.OperStatus); err == nil {
				compState.OperStatus = operStatus
			}
		}
		// State Part No
		if firmwareInfo.PartNo != "" {
			compState.PartNo = &firmwareInfo.PartNo
		}
		// State Serial No
		if firmwareInfo.SerialNo != "" {
			compState.SerialNo = &firmwareInfo.SerialNo
		}
		// State Model Name
		if firmwareInfo.ModelName != "" {
			compState.ModelName = &firmwareInfo.ModelName
		}
		if pType == StatePaths {
			return nil
		}
	}

	if ((pType == AllPaths || pType == AllCompPaths) && cfgPresent) || pType == ConfigPaths {
		// Filling in config values
		// Config Name
		compCfg.Name = &name
		if firmwareCfg.Name != "" {
			compCfg.Name = &firmwareCfg.Name
		}
	}

	if pType != SingularPath {
		return nil
	}

	switch targetUriPath {
	case COMP_STATE_DESCR:
		if firmwareInfo.Description == "" {
			return errors.New("description field not present in State DB")
		}
		compState.Description = &firmwareInfo.Description
	case COMP_STATE_NAME:
		compState.Name = &name
		if firmwareInfo.Name != "" {
			compState.Name = &firmwareInfo.Name
		}
	case COMP_STATE_TYPE:
		compState.Type, _ = compState.To_OpenconfigPlatform_Components_Component_State_Type_Union(
			ocbinds.OpenconfigPlatformTypes_OPENCONFIG_HARDWARE_COMPONENT_CHASSIS)
	case COMP_STATE_FIRM_VER:
		if firmwareInfo.FirmwareVersion == "" {
			return errors.New("firmware_version field not present in State DB")
		}
		compState.FirmwareVersion = &firmwareInfo.FirmwareVersion
	case COMP_STATE_HW_VER:
		if firmwareInfo.HardwareVersion == "" {
			return errors.New("hardware_version field not present in State DB")
		}
		compState.HardwareVersion = &firmwareInfo.HardwareVersion
	case COMP_STATE_MFG_DATE:
		mfg_date := reformatMfgDate(firmwareInfo.MfgDate)
		if mfg_date == "" {
			return fmt.Errorf("mfg_date field has invalid format in State DB: %s", firmwareInfo.MfgDate)
		}
		compState.MfgDate = &mfg_date
	case COMP_STATE_OPER_STATUS:
		if firmwareInfo.OperStatus == "" {
			return errors.New("oper_status field not present in State DB")
		}
		if operStatus, err := operStatusFromString(firmwareInfo.OperStatus); err == nil {
			compState.OperStatus = operStatus
		} else {
			return fmt.Errorf("oper_status invalid field value in State DB: %s", firmwareInfo.OperStatus)
		}
	case COMP_STATE_PART_NO:
		if firmwareInfo.PartNo == "" {
			return errors.New("part_no field not present in State DB")
		}
		compState.PartNo = &firmwareInfo.PartNo
	case COMP_STATE_SERIAL_NO:
		if firmwareInfo.SerialNo == "" {
			return errors.New("serial_no field not present in State DB")
		}
		compState.SerialNo = &firmwareInfo.SerialNo
	case COMP_STATE_MODEL_NAME:
		if firmwareInfo.ModelName == "" {
			return errors.New("component state model-name field not present in State DB")
		}
		compState.ModelName = &firmwareInfo.ModelName
	case COMP_CONFIG_NAME:
		compCfg.Name = &name
		if firmwareCfg.Name != "" {
			compCfg.Name = &firmwareCfg.Name
		}
	}
	return nil
}

func validSWCompName(name *string, prefix string) bool {
	if name == nil || *name == "" {
		return false
	}
	// Expect node name of form network_stackX or osX, where X is an integer (either 0 or 1)
	if !strings.HasPrefix(*name, prefix) {
		return false
	}

	sp := strings.SplitAfter(*name, prefix)
	if len(sp) < 2 {
		return false
	}

	if val, err := strconv.Atoi(sp[1]); err != nil || val > 1 {
		return false
	}
	return true
}

func getSWCompInfoFromDb(name string, d *db.DB, tblName string) (SWCompInfo, error) {
	swcEntry, err := d.GetEntry(&db.TableSpec{Name: tblName}, db.Key{Comp: []string{name}})
	if err != nil {
		log.V(lvl.DEBUG).Info("Cannot get entry: ", name, "; Error: ", err)
		return SWCompInfo{}, err
	}

	swcInfo := SWCompInfo{
		Name:            swcEntry.Get("name"),
		SoftwareVersion: swcEntry.Get("software-version"),
		Parent:          swcEntry.Get("parent"),
		OperStatus:      swcEntry.Get("oper-status"),
		StorageSide:     swcEntry.Get("storage-side"),
		Type:            swcEntry.Get("type"),
		BootLoaderType:  swcEntry.Get("boot-loader-type"),
	}

	return swcInfo, nil
}

func fillBootLoaderContainer(info SWCompInfo, comp *ocbinds.OpenconfigPlatform_Components_Component) {
	if info.BootLoaderType == "" {
		return
	}
	ygot.BuildEmptyTree(comp.BootLoader)
	ygot.BuildEmptyTree(comp.BootLoader.State)
	if et, ok := dbToYangBootLoaderTypeMap[info.BootLoaderType]; ok {
		comp.BootLoader.State.Type = et
	}
}

/* Filling in the state info for software components available in Redis DB */
func fillSWCompInfo(comp *ocbinds.OpenconfigPlatform_Components_Component,
	name string, pType PathType, targetUriPath string, stdb *db.DB, cType componentType) error {
	swcInfo, err := getSWCompInfoFromDb(name, stdb, SW_COMP_TBL)
	if err != nil {
		log.V(lvl.DEBUG).Info("Error Getting SW Comp info from State DB: ", err.Error())
		return err
	}
	compState := comp.State
	swModuleState := comp.SoftwareModule.State
	defaultVal := ""
	defaultParentVal := CHASSIS_PREFIX

	if pType == AllPaths || pType == AllCompPaths || pType == StatePaths {
		// Filling in state values
		// State Name
		compState.Name = &name
		// State Software Version
		compState.SoftwareVersion = &defaultVal
		if swcInfo.SoftwareVersion != "" {
			compState.SoftwareVersion = &swcInfo.SoftwareVersion
		}
		// State Parent
		compState.Parent = &defaultParentVal
		if swcInfo.Parent != "" {
			compState.Parent = &swcInfo.Parent
		}
		// State Type
		switch cType {
		case CompTypeOS:
			compState.Type, _ = compState.To_OpenconfigPlatform_Components_Component_State_Type_Union(
				ocbinds.OpenconfigPlatformTypes_OPENCONFIG_SOFTWARE_COMPONENT_OPERATING_SYSTEM)
		case CompTypeBootLoader:
			compState.Type, _ = compState.To_OpenconfigPlatform_Components_Component_State_Type_Union(
				ocbinds.OpenconfigPlatformTypes_OPENCONFIG_SOFTWARE_COMPONENT_BOOT_LOADER)
			fillBootLoaderContainer(swcInfo, comp)
			return nil
		case CompTypeNWStack:
			compState.Type, _ = compState.To_OpenconfigPlatform_Components_Component_State_Type_Union(
				ocbinds.OpenconfigPlatformTypes_OPENCONFIG_SOFTWARE_COMPONENT_SOFTWARE_MODULE)
		}
		// State Oper Status
		if operStatus, err := operStatusFromString(swcInfo.OperStatus); err == nil {
			compState.OperStatus = operStatus
		} else {
			compState.OperStatus = ocbinds.OpenconfigPlatformTypes_COMPONENT_OPER_STATUS_DISABLED
		}
		if pType == StatePaths {
			return nil
		}
		// SW Module State Module Type
		if cType == CompTypeNWStack {
			swModuleState.ModuleType = ocbinds.OpenconfigPlatformSoftware_SOFTWARE_MODULE_TYPE_USERSPACE_PACKAGE_BUNDLE
		}
		return nil
	}

	switch targetUriPath {
	case COMP_STATE_NAME:
		compState.Name = &name
	case COMP_STATE_SW_VER:
		if swcInfo.SoftwareVersion == "" {
			return errors.New("software_version field not present in State DB")
		}
		compState.SoftwareVersion = &swcInfo.SoftwareVersion
	case COMP_STATE_PARENT:
		compState.Parent = &defaultParentVal
		if swcInfo.Parent != "" {
			compState.Parent = &swcInfo.Parent
		}
	case COMP_STATE_TYPE:
		switch cType {
		case CompTypeOS:
			compState.Type, _ = compState.To_OpenconfigPlatform_Components_Component_State_Type_Union(
				ocbinds.OpenconfigPlatformTypes_OPENCONFIG_SOFTWARE_COMPONENT_OPERATING_SYSTEM)
		case CompTypeBootLoader:
			compState.Type, _ = compState.To_OpenconfigPlatform_Components_Component_State_Type_Union(
				ocbinds.OpenconfigPlatformTypes_OPENCONFIG_SOFTWARE_COMPONENT_BOOT_LOADER)
		case CompTypeNWStack:
			compState.Type, _ = compState.To_OpenconfigPlatform_Components_Component_State_Type_Union(
				ocbinds.OpenconfigPlatformTypes_OPENCONFIG_SOFTWARE_COMPONENT_SOFTWARE_MODULE)
		default:
			return errors.New("invalid component type for software component")
		}
	case COMP_STATE_OPER_STATUS:
		if cType == CompTypeBootLoader {
			return errors.New("invalid path for bootloader component.")
		}
		if operStatus, err := operStatusFromString(swcInfo.OperStatus); err == nil {
			compState.OperStatus = operStatus
		} else {
			return errors.New("oper_status is missing/invalid field value in State DB: " + swcInfo.OperStatus)
		}
	case SW_MODULE_STATE_MODULE_TYPE:
		if cType != CompTypeNWStack {
			return errors.New("invalid component for software-module/state/module-type path.")
		}
		swModuleState.ModuleType = ocbinds.OpenconfigPlatformSoftware_SOFTWARE_MODULE_TYPE_USERSPACE_PACKAGE_BUNDLE
	case SW_BOOT_LOADER_STATE_TYPE:
		fillBootLoaderContainer(swcInfo, comp)
	}
	return nil
}

func getPortInfoFromDb(name string, d *db.DB, tblName string) (PortInfo, error) {
	portEntry, err := d.GetEntry(&db.TableSpec{Name: tblName}, db.Key{Comp: []string{name}})
	if err != nil {
		log.V(lvl.DEBUG).Info("Cant get entry: ", name, "; Error: ", err)
		return PortInfo{}, err
	}

	portInfo := PortInfo{
		Name:            portEntry.Get("name"),
		Parent:          portEntry.Get("parent"),
		PortID:          portEntry.Get("port-id"),
		BreakoutMode:    portEntry.Get("brkout_mode"),
		FirmwareVersion: portEntry.Get("serdes-firmware-version"),
	}
	return portInfo, nil
}

func fillSubComponentForPort(subcomp *ocbinds.OpenconfigPlatform_Components_Component_Subcomponents_Subcomponent, d *db.DB, subCompKey, targetUriPath string) error {
	log.V(lvl.DEBUG).Infof("fillSubComponentForPort: subCompKey: %s, targetUriPath: %s\n", subCompKey, targetUriPath)
	if subCompKey == "" {
		return errors.New("empty key for subcomponent table.")
	}
	if _, err := d.GetEntry(&db.TableSpec{Name: "BREAKOUT_PORT_XCVR_CFG"}, db.Key{Comp: []string{subCompKey}}); err != nil {
		return fmt.Errorf("cannot get entry with key: %s; Err: %w", subCompKey, err)
	}

	ygot.BuildEmptyTree(subcomp)
	ygot.BuildEmptyTree(subcomp.Config)
	ygot.BuildEmptyTree(subcomp.State)

	if keyList := strings.Split(subCompKey, d.Opts.KeySeparator); len(keyList) == 2 {
		subCompName := keyList[1]
		subcomp.Config.Name = &subCompName
		subcomp.State.Name = &subCompName
	}
	return nil
}

func fillAllSubComponentsForPort(scomp *ocbinds.OpenconfigPlatform_Components_Component_Subcomponents, d *db.DB, compName, subKey, targetUriPath string) error {
	log.V(lvl.DEBUG).Infof("fillAllSubComponentsForPort: compName: %s, subKey: %s, targetUriPath: %s\n", compName, subKey, targetUriPath)
	key := subKey
	if subKey == "" {
		key = "*"
	}
	subCompKeys, err := d.GetKeysPattern(&(db.TableSpec{Name: "BREAKOUT_PORT_XCVR_CFG"}), db.Key{Comp: []string{compName, key}})
	if err != nil || len(subCompKeys) == 0 {
		return errors.New("failed to get keys from subcomponent table.")
	}
	for _, key := range subCompKeys {
		if key.Len() < 2 {
			continue
		}
		if subComp, err := scomp.NewSubcomponent(key.Get(1)); err == nil {
			// Ignore errors for non-leaf paths
			fillSubComponentForPort(subComp, d, key.Get(0)+d.Opts.KeySeparator+key.Get(1), targetUriPath)
		}
	}
	return nil
}

/* Filling in the state info for ports available in Redis DB */
func fillPortInfo(comp *ocbinds.OpenconfigPlatform_Components_Component, name, subKey string, pType PathType, targetUriPath string, stdb *db.DB, cfgdb *db.DB) error {
	ifName := platform.InterfaceNameFromPort(name)
	portInfo, err := getPortInfoFromDb(ifName, stdb, PORT_BREAKOUT)
	if err != nil {
		log.V(lvl.DEBUG).Info("Error Getting Port info from State DB: ", err.Error())
		if pType == StatePaths {
			return err
		}
	}
	portCfg, err := getPortInfoFromDb(ifName, cfgdb, BREAKOUT_TBL)
	if err != nil {
		log.V(lvl.DEBUG).Info("Error Getting Port info from Config DB: ", err.Error())
		if pType == ConfigPaths {
			return err
		}
	}
	compState := comp.State
	defaultParentVal := IC_NAME_PREFIX + "0"

	if pType != SingularPath {
		if pType == AllPaths || pType == AllCompPaths {
			// Subcomponents
			if targetUriPath == COMP || strings.HasPrefix(targetUriPath, COMP_SUB) {
				ygot.BuildEmptyTree(comp.Subcomponents)
				fillAllSubComponentsForPort(comp.Subcomponents, cfgdb, name, subKey, targetUriPath)
			}
		}
		if pType == AllPaths && portCfg.BreakoutMode != "" {
			// Port breakout
			if entry, err := cfgdb.GetEntry(&db.TableSpec{Name: BREAKOUT_TBL}, db.Key{Comp: []string{ifName}}); err == nil {
				if mode, err := sanitizeMode(entry.Get("brkout_mode")); err == nil {
					if chGrps, err := populateChannelGroups(ifName, mode); err == nil {
						// Populate for all indices.
						for idx, grp := range chGrps {
							if err := fillChGroup(idx, grp, comp.Port.BreakoutMode.Groups); err != nil {
								log.V(lvl.ERROR).Info(err)
							}
						}
					} else {
						log.V(lvl.DEBUG).Info("Cannot get channel groups: ", err)
					}
				} else {
					log.V(lvl.DEBUG).Info("Cannot get mode: ", err)
				}
			} else {
				log.V(lvl.DEBUG).Info("Cannot get entry: ", name, "; Error: ", err)
			}
		}
		// Filling in state values: name, parent, and type
		// State Type
		compState.Type, _ = compState.To_OpenconfigPlatform_Components_Component_State_Type_Union(
			ocbinds.OpenconfigPlatformTypes_OPENCONFIG_HARDWARE_COMPONENT_PORT)
		// State Name
		compState.Name = &name
		if portInfo.Name != "" {
			compState.Name = &portInfo.Name
		}
		// State Parent
		compState.Parent = &defaultParentVal
		if portInfo.Parent != "" {
			compState.Parent = &portInfo.Parent
		}
		// State Firmware Version
		if portInfo.FirmwareVersion != "" {
			compState.FirmwareVersion = &portInfo.FirmwareVersion
		}
		// Config Name
		comp.Config.Name = &name
		return nil
	}

	switch targetUriPath {
	case COMP_STATE_NAME:
		compState.Name = &name
		if portInfo.Name != "" {
			compState.Name = &portInfo.Name
		}
	case COMP_STATE_PARENT:
		compState.Parent = &defaultParentVal
		if portInfo.Parent != "" {
			compState.Parent = &portInfo.Parent
		}
	case COMP_STATE_FIRM_VER:
		if portInfo.FirmwareVersion != "" {
			compState.FirmwareVersion = &portInfo.FirmwareVersion
		}
	case COMP_STATE_TYPE:
		compState.Type, _ = compState.To_OpenconfigPlatform_Components_Component_State_Type_Union(
			ocbinds.OpenconfigPlatformTypes_OPENCONFIG_HARDWARE_COMPONENT_PORT)
	case COMP_CONFIG_NAME:
		comp.Config.Name = &name
	}

	if strings.HasPrefix(targetUriPath, COMP_SUB) {
		ygot.BuildEmptyTree(comp.Subcomponents)
		fillAllSubComponentsForPort(comp.Subcomponents, cfgdb, name, subKey, targetUriPath)
	}
	return nil
}

func fillPcieInfo(pcieComp *ocbinds.OpenconfigPlatform_Components_Component,
	name string, pType PathType, targetUriPath string, d *db.DB) error {
	pcieParent := CHASSIS_PREFIX
	pcieInfo, err := d.GetEntry(&db.TableSpec{Name: PCIE_TBL}, db.Key{Comp: []string{name}})
	if err != nil {
		log.V(lvl.DEBUG).Info("Cant get PCIE table info: ", err)
		return err
	}

	if targetUriPath == COMP || targetUriPath == COMP_ST || targetUriPath == COMP_STATE_PARENT {
		pcieComp.State.Parent = &pcieParent
	}

	// Updating the tree directly to avoid string case matches for each individual pcie path.
	// PCIE Fatal errors
	if pType == AllPaths || pType == AllCompPaths || pType == StatePaths ||
		targetUriPath == COMP_STATE_PCIE ||
		strings.Contains(targetUriPath, "fatal-errors") {
		ygot.BuildEmptyTree(pcieComp.State.Pcie.FatalErrors)
		pcieComp.State.Pcie.FatalErrors.TotalErrors = String2Uint(pcieInfo.Get("fatal|TOTAL_ERR_FATAL"), 0)
		pcieComp.State.Pcie.FatalErrors.UndefinedErrors = String2Uint(pcieInfo.Get("fatal|Undefined"), 0)
		pcieComp.State.Pcie.FatalErrors.DataLinkErrors = String2Uint(pcieInfo.Get("fatal|DLP"), 0)
		pcieComp.State.Pcie.FatalErrors.SurpriseDownErrors = String2Uint(pcieInfo.Get("fatal|SDES"), 0)
		pcieComp.State.Pcie.FatalErrors.PoisonedTlpErrors = String2Uint(pcieInfo.Get("fatal|TLP"), 0)
		pcieComp.State.Pcie.FatalErrors.FlowControlProtocolErrors = String2Uint(pcieInfo.Get("fatal|FCP"), 0)
		pcieComp.State.Pcie.FatalErrors.CompletionTimeoutErrors = String2Uint(pcieInfo.Get("fatal|CmpltTO"), 0)
		pcieComp.State.Pcie.FatalErrors.CompletionAbortErrors = String2Uint(pcieInfo.Get("fatal|CmpltAbrt"), 0)
		pcieComp.State.Pcie.FatalErrors.UnexpectedCompletionErrors = String2Uint(pcieInfo.Get("fatal|UnxCmplt"), 0)
		pcieComp.State.Pcie.FatalErrors.ReceiverOverflowErrors = String2Uint(pcieInfo.Get("fatal|RxOF"), 0)
		pcieComp.State.Pcie.FatalErrors.MalformedTlpErrors = String2Uint(pcieInfo.Get("fatal|MalfTLP"), 0)
		pcieComp.State.Pcie.FatalErrors.EcrcErrors = String2Uint(pcieInfo.Get("fatal|ECRC"), 0)
		pcieComp.State.Pcie.FatalErrors.UnsupportedRequestErrors = String2Uint(pcieInfo.Get("fatal|UnsupReq"), 0)
		pcieComp.State.Pcie.FatalErrors.AcsViolationErrors = String2Uint(pcieInfo.Get("fatal|ACSViol"), 0)
		pcieComp.State.Pcie.FatalErrors.InternalErrors = String2Uint(pcieInfo.Get("fatal|UncorrIntErr"), 0)
		pcieComp.State.Pcie.FatalErrors.BlockedTlpErrors = String2Uint(pcieInfo.Get("fatal|BlockedTLP"), 0)
		pcieComp.State.Pcie.FatalErrors.AtomicOpBlockedErrors = String2Uint(pcieInfo.Get("fatal|AtomicOpBlocked"), 0)
		pcieComp.State.Pcie.FatalErrors.TlpPrefixBlockedErrors = String2Uint(pcieInfo.Get("fatal|TLPBlockedErr"), 0)
	}

	// PCIE Non-fatal errors
	if pType == AllPaths || pType == AllCompPaths || pType == StatePaths ||
		targetUriPath == COMP_STATE_PCIE ||
		strings.Contains(targetUriPath, "non-fatal-errors") {
		ygot.BuildEmptyTree(pcieComp.State.Pcie.NonFatalErrors)
		pcieComp.State.Pcie.NonFatalErrors.TotalErrors = String2Uint(pcieInfo.Get("non_fatal|TOTAL_ERR_NONFATAL"), 0)
		pcieComp.State.Pcie.NonFatalErrors.UndefinedErrors = String2Uint(pcieInfo.Get("non_fatal|Undefined"), 0)
		pcieComp.State.Pcie.NonFatalErrors.DataLinkErrors = String2Uint(pcieInfo.Get("non_fatal|DLP"), 0)
		pcieComp.State.Pcie.NonFatalErrors.SurpriseDownErrors = String2Uint(pcieInfo.Get("non_fatal|SDES"), 0)
		pcieComp.State.Pcie.NonFatalErrors.PoisonedTlpErrors = String2Uint(pcieInfo.Get("non_fatal|TLP"), 0)
		pcieComp.State.Pcie.NonFatalErrors.FlowControlProtocolErrors = String2Uint(pcieInfo.Get("non_fatal|FCP"), 0)
		pcieComp.State.Pcie.NonFatalErrors.CompletionTimeoutErrors = String2Uint(pcieInfo.Get("non_fatal|CmpltTO"), 0)
		pcieComp.State.Pcie.NonFatalErrors.CompletionAbortErrors = String2Uint(pcieInfo.Get("non_fatal|CmpltAbrt"), 0)
		pcieComp.State.Pcie.NonFatalErrors.UnexpectedCompletionErrors = String2Uint(pcieInfo.Get("non_fatal|UnxCmplt"), 0)
		pcieComp.State.Pcie.NonFatalErrors.ReceiverOverflowErrors = String2Uint(pcieInfo.Get("non_fatal|RxOF"), 0)
		pcieComp.State.Pcie.NonFatalErrors.MalformedTlpErrors = String2Uint(pcieInfo.Get("non_fatal|MalfTLP"), 0)
		pcieComp.State.Pcie.NonFatalErrors.EcrcErrors = String2Uint(pcieInfo.Get("non_fatal|ECRC"), 0)
		pcieComp.State.Pcie.NonFatalErrors.UnsupportedRequestErrors = String2Uint(pcieInfo.Get("non_fatal|UnsupReq"), 0)
		pcieComp.State.Pcie.NonFatalErrors.AcsViolationErrors = String2Uint(pcieInfo.Get("non_fatal|ACSViol"), 0)
		pcieComp.State.Pcie.NonFatalErrors.InternalErrors = String2Uint(pcieInfo.Get("non_fatal|UncorrIntErr"), 0)
		pcieComp.State.Pcie.NonFatalErrors.BlockedTlpErrors = String2Uint(pcieInfo.Get("non_fatal|BlockedTLP"), 0)
		pcieComp.State.Pcie.NonFatalErrors.AtomicOpBlockedErrors = String2Uint(pcieInfo.Get("non_fatal|AtomicOpBlocked"), 0)
		pcieComp.State.Pcie.NonFatalErrors.TlpPrefixBlockedErrors = String2Uint(pcieInfo.Get("non_fatal|TLPBlockedErr"), 0)
	}

	// PCIE Correctable errors
	if pType == AllPaths || pType == AllCompPaths || pType == StatePaths ||
		targetUriPath == COMP_STATE_PCIE ||
		strings.Contains(targetUriPath, "correctable-errors") {
		ygot.BuildEmptyTree(pcieComp.State.Pcie.CorrectableErrors)
		pcieComp.State.Pcie.CorrectableErrors.TotalErrors = String2Uint(pcieInfo.Get("correctable|TOTAL_ERR_COR"), 0)
		pcieComp.State.Pcie.CorrectableErrors.ReceiverErrors = String2Uint(pcieInfo.Get("correctable|RxErr"), 0)
		pcieComp.State.Pcie.CorrectableErrors.BadTlpErrors = String2Uint(pcieInfo.Get("correctable|BadTLP"), 0)
		pcieComp.State.Pcie.CorrectableErrors.BadDllpErrors = String2Uint(pcieInfo.Get("correctable|BadDLLP"), 0)
		pcieComp.State.Pcie.CorrectableErrors.RelayRolloverErrors = String2Uint(pcieInfo.Get("correctable|Rollover"), 0)
		pcieComp.State.Pcie.CorrectableErrors.ReplayTimeoutErrors = String2Uint(pcieInfo.Get("correctable|Timeout"), 0)
		pcieComp.State.Pcie.CorrectableErrors.AdvisoryNonFatalErrors = String2Uint(pcieInfo.Get("correctable|NonFatalErr"), 0)
		pcieComp.State.Pcie.CorrectableErrors.InternalErrors = String2Uint(pcieInfo.Get("correctable|CorrIntErr"), 0)
		pcieComp.State.Pcie.CorrectableErrors.HdrLogOverflowErrors = String2Uint(pcieInfo.Get("correctable|HeaderOF"), 0)
	}

	return nil
}

func validPSCompName(name *string, prefix string) bool {
	if name == nil || *name == "" {
		return false
	}
	if !strings.HasPrefix(*name, prefix) {
		return false
	}
	return true
}

func getPowerSupplyFromDb(name string, railKey string, d *db.DB) (PowerSupplyInfo, error) {
	var psInfo PowerSupplyInfo
	var ps_comp_err, ps_rail_err error
	psEntry, ps_comp_err := d.GetEntry(&db.TableSpec{Name: POWER_INFO_TBL}, db.Key{Comp: []string{name}})
	if ps_comp_err == nil {
		psInfo = PowerSupplyInfo{
			Parent:              psEntry.Get("parent"),
			Temperature:         psEntry.Get("temperature"),
			TemperatureMax:      psEntry.Get("temperature_max"),
			TemperatureInterval: psEntry.Get("temperature_interval"),
			CommandedFrequency:  psEntry.Get("commanded-frequency"),
			Frequency:           psEntry.Get("frequency"),
			ManufacturerStatus:  psEntry.Get("manufacturer-status"),
			StatusGpio:          psEntry.Get("status-gpio"),
			Location:            psEntry.Get("location"),
		}
	} else {
		log.V(lvl.DEBUG).Info("Can't get comp entry: ", name, "; Error: ", ps_comp_err)
	}

	if railKey != "" {
		powerSupplyTBL := POWER_INFO_TBL + d.Opts.KeySeparator + name
		psRailEntry, ps_rail_err := d.GetEntry(&db.TableSpec{Name: powerSupplyTBL}, db.Key{Comp: []string{railKey}})
		if ps_rail_err == nil {
			psInfo.Direction = psRailEntry.Get("direction")
			psInfo.StatusVout = psRailEntry.Get("all-status-vout")
			psInfo.CommendedVoltage = psRailEntry.Get("commanded-voltage")
			psInfo.Energy = psRailEntry.Get("energy")
			psInfo.Current = psRailEntry.Get("current")
			psInfo.Voltage = psRailEntry.Get("voltage")
			psInfo.Power = psRailEntry.Get("power")
			psInfo.PeakPower = psRailEntry.Get("peak-power")
			psInfo.PeakPowerInterval = psRailEntry.Get("peak-power-interval")
			psInfo.PeakVoltage = psRailEntry.Get("peak-voltage")
			psInfo.PeakCurrent = psRailEntry.Get("peak-current")
			psInfo.RailTemperature = psRailEntry.Get("temperature")
		} else {
			log.V(lvl.DEBUG).Info("Can't get rail entry: ", name, " + ", railKey, "; Error: ", ps_rail_err)
		}
	}
	if ps_comp_err != nil && ps_rail_err != nil {
		return PowerSupplyInfo{}, fmt.Errorf("Can't get comp entry: %s; Comp name error: %w; Rail key error: %w", name, ps_comp_err, ps_rail_err)
	}

	return psInfo, nil
}

func fillPowerSupplyInfo(powerSupplyCom *ocbinds.OpenconfigPlatform_Components_Component, name string, railKey string, pType PathType, targetUriPath string, d *db.DB) error {
	log.V(lvl.DEBUG).Infof("fillPowerSupplyInfo name=%s railKey=%s pType=%v targetUriPath=%s", name, railKey, pType, targetUriPath)
	psi, err := getPowerSupplyFromDb(name, railKey, d)
	if err != nil {
		log.V(lvl.DEBUG).Info("Error Getting Power Supply info from State dB", err.Error())
		return err
	}

	psCompState := powerSupplyCom.State
	defaultParentVal := CHASSIS_PREFIX

	if pType == AllPaths || pType == AllCompPaths {
		powerSupplyCom.Config.Name = &name
	} else if pType == ConfigPaths {
		powerSupplyCom.Config.Name = &name
		return nil
	}

	if pType == AllPaths || pType == AllCompPaths || pType == StatePaths {
		psCompState.Name = &name
		psCompState.Location = &psi.Location
		psCompState.Type, _ = psCompState.To_OpenconfigPlatform_Components_Component_State_Type_Union(ocbinds.OpenconfigPlatformTypes_OPENCONFIG_HARDWARE_COMPONENT_POWER_SUPPLY)

		if psi.Temperature != "" {
			psCompState.Temperature.Instant = String2Float(psi.Temperature, 0.0)
		}
		if psi.TemperatureMax != "" {
			psCompState.Temperature.Max = String2Float(psi.TemperatureMax, 0.0)
		}
		if psi.TemperatureInterval != "" {
			psCompState.Temperature.Interval = String2Uint(psi.TemperatureInterval, 0)
		}

		psCompState.Parent = StringWithDefault(&psi.Parent, defaultParentVal)
		return nil
	}

	switch targetUriPath {
	case COMP_CONFIG_NAME:
		powerSupplyCom.Config.Name = &name
	case COMP_STATE_NAME:
		psCompState.Name = &name
	case COMP_STATE_LOCATION:
		psCompState.Location = &psi.Location
	case COMP_STATE_TYPE:
		psCompState.Type, _ = psCompState.To_OpenconfigPlatform_Components_Component_State_Type_Union(ocbinds.OpenconfigPlatformTypes_OPENCONFIG_HARDWARE_COMPONENT_POWER_SUPPLY)
	case COMP_STATE_TEMP:
		if psi.Temperature != "" {
			psCompState.Temperature.Instant = String2Float(psi.Temperature, 0.0)
		}
	case COMP_STATE_TEMP_MAX:
		if psi.TemperatureMax != "" {
			psCompState.Temperature.Max = String2Float(psi.TemperatureMax, 0.0)
		}
	case COMP_STATE_TEMP_INTV:
		if psi.TemperatureInterval != "" {
			psCompState.Temperature.Interval = String2Uint(psi.TemperatureInterval, 0)
		}
	case COMP_STATE_PARENT:
		psCompState.Parent = StringWithDefault(&psi.Parent, defaultParentVal)
	}

	return nil
}

func getControllerCardInfoFromDb(name string, d *db.DB) (ControllerCardInfo, error) {
	cardEntry, err := d.GetEntry(&db.TableSpec{Name: CONTROLLER_CARD_TBL}, db.Key{Comp: []string{name}})
	if err != nil {
		log.V(lvl.DEBUG).Info("Cant get entry: ", name, "; Error: ", err)
		return ControllerCardInfo{}, err
	}

	return ControllerCardInfo{
		Name:              cardEntry.Get("name"),
		Model_Name:        cardEntry.Get("model"),
		Parent:            cardEntry.Get("parent"),
		Part_Number:       cardEntry.Get("part-no"),
		Serial_Number:     cardEntry.Get("serial"),
		Removable:         cardEntry.Get("removable"),
		Mfg_Date:          cardEntry.Get("mfg-date"),
		Hardware_Version:  cardEntry.Get("hardware-version"),
		Install_Position:  cardEntry.Get("install-position"),
		Install_Component: cardEntry.Get("install-component"),
		OperStatus:        cardEntry.Get("oper-status"),
		Empty:             cardEntry.Get("empty"),
	}, nil
}

/* Filling in the state info for controller card components available in Redis DB */
func fillControllerCardInfo(comp *ocbinds.OpenconfigPlatform_Components_Component,
	name string, pType PathType, targetUriPath string, stdb *db.DB) error {
	cardInfo, err := getControllerCardInfoFromDb(name, stdb)
	if err != nil {
		log.V(lvl.DEBUG).Info("Error Getting controller card info from State DB: ", err.Error())
		return err
	}
	cardState := comp.State
	defaultVal := ""
	defaultParentVal := CHASSIS_PREFIX

	if shouldExportLeaf(pType, targetUriPath, StateLeaf, COMP_STATE_NAME) {
		cardState.Name = StringWithDefault(&cardInfo.Name, name)
	}
	if shouldExportLeaf(pType, targetUriPath, StateLeaf, COMP_STATE_TYPE) {
		cardState.Type, _ = cardState.To_OpenconfigPlatform_Components_Component_State_Type_Union(
			ocbinds.OpenconfigPlatformTypes_OPENCONFIG_HARDWARE_COMPONENT_CONTROLLER_CARD)
	}
	if shouldExportLeaf(pType, targetUriPath, StateLeaf, COMP_STATE_DESCR) {
		cardState.Description = StringWithDefault(&cardInfo.Model_Name, defaultVal)
	}
	if shouldExportLeaf(pType, targetUriPath, StateLeaf, COMP_STATE_PARENT) {
		cardState.Parent = StringWithDefault(&cardInfo.Parent, defaultParentVal)
	}
	if shouldExportLeaf(pType, targetUriPath, StateLeaf, COMP_STATE_MODEL_NAME) {
		cardState.ModelName = StringWithDefault(&cardInfo.Model_Name, defaultVal)
	}
	if shouldExportLeaf(pType, targetUriPath, StateLeaf, COMP_STATE_PART_NO) {
		cardState.PartNo = StringWithDefault(&cardInfo.Part_Number, cardInfo.Model_Name)
	}
	if shouldExportLeaf(pType, targetUriPath, StateLeaf, COMP_STATE_SERIAL_NO) {
		cardState.SerialNo = StringWithDefault(&cardInfo.Serial_Number, defaultVal)
	}
	if shouldExportLeaf(pType, targetUriPath, StateLeaf, COMP_STATE_REMOVABLE) {
		cardState.Removable = String2Bool(&cardInfo.Removable, false)
	}
	if shouldExportLeaf(pType, targetUriPath, StateLeaf, COMP_STATE_MFG_DATE) {
		if mfg_date := reformatMfgDate(cardInfo.Mfg_Date); mfg_date != "" {
			cardState.MfgDate = &mfg_date
		}
	}
	if shouldExportLeaf(pType, targetUriPath, StateLeaf, COMP_STATE_HW_VER) {
		cardState.HardwareVersion = StringWithDefault(&cardInfo.Hardware_Version, defaultVal)
	}
	if shouldExportLeaf(pType, targetUriPath, StateLeaf, COMP_STATE_INSTALL_POSITION) {
		cardState.InstallPosition = StringWithDefault(&cardInfo.Install_Position, defaultVal)
	}
	if shouldExportLeaf(pType, targetUriPath, StateLeaf, COMP_STATE_INSTALL_COMPONENT) {
		cardState.InstallComponent = FirstNonEmptyOrNil(cardInfo.Install_Component, cardInfo.Parent, defaultParentVal)
	}
	if shouldExportLeaf(pType, targetUriPath, StateLeaf, COMP_STATE_EMPTY) {
		// If DB has a controller card entry but no empty field, consider the controller card present.
		cardState.Empty = String2Bool(&cardInfo.Empty, false)
	}
	if shouldExportLeaf(pType, targetUriPath, StateLeaf, COMP_STATE_OPER_STATUS) {
		if operStatus, err := operStatusFromString(cardInfo.OperStatus); err == nil {
			cardState.OperStatus = operStatus
		} else {
			// If this code is running, the controller card is functioning, so report ACTIVE
			cardState.OperStatus = ocbinds.OpenconfigPlatformTypes_COMPONENT_OPER_STATUS_ACTIVE
		}
	}

	return nil
}

func getLineCardInfoFromDb(name string, d *db.DB) (LineCardInfo, error) {
	entry, err := d.GetEntry(&db.TableSpec{Name: LINECARD_TBL}, db.Key{Comp: []string{name}})
	if err != nil {
		log.V(lvl.DEBUG).Info("Cant get entry: ", name, "; Error: ", err)
		return LineCardInfo{}, err
	}

	return LineCardInfo{
		Parent:            entry.Get("parent"),
		Serial_Number:     entry.Get("serial"),
		Model_Name:        entry.Get("model"),
		Part_Number:       entry.Get("part-no"),
		Removable:         entry.Get("removable"),
		Mfg_Date:          entry.Get("mfg-date"),
		Hardware_Version:  entry.Get("hardware-version"),
		Install_Position:  entry.Get("install-position"),
		Install_Component: entry.Get("install-component"),
		OperStatus:        entry.Get("oper-status"),
		Empty:             entry.Get("empty"),
		Type:              entry.Get("type"),
	}, nil
}

/* Filling in the state info for line card components available in Redis DB */
func fillLineCardInfo(comp *ocbinds.OpenconfigPlatform_Components_Component,
	name string, pType PathType, targetUriPath string, stdb *db.DB) error {
	lineCardInfo, err := getLineCardInfoFromDb(name, stdb)
	if err != nil {
		log.V(lvl.DEBUG).Info("Error Getting line card info from State DB: ", err.Error())
		return err
	}

	ygot.BuildEmptyTree(comp.Linecard)
	ygot.BuildEmptyTree(comp.Linecard.State)

	cardState := comp.State
	defaultVal := ""
	defaultParentVal := CHASSIS_PREFIX

	if shouldExportLeaf(pType, targetUriPath, StateLeaf, COMP_STATE_NAME) {
		cardState.Name = &name
	}
	if shouldExportLeaf(pType, targetUriPath, StateLeaf, COMP_STATE_PARENT) {
		cardState.Parent = StringWithDefault(&lineCardInfo.Parent, defaultParentVal)
	}
	if shouldExportLeaf(pType, targetUriPath, StateLeaf, COMP_STATE_SERIAL_NO) {
		cardState.SerialNo = StringWithDefault(&lineCardInfo.Serial_Number, defaultVal)
	}
	if shouldExportLeaf(pType, targetUriPath, StateLeaf, COMP_STATE_MODEL_NAME) {
		cardState.ModelName = StringWithDefault(&lineCardInfo.Model_Name, defaultVal)
	}
	if shouldExportLeaf(pType, targetUriPath, StateLeaf, COMP_STATE_PART_NO) {
		cardState.PartNo = StringWithDefault(&lineCardInfo.Part_Number, defaultVal)
	}
	if shouldExportLeaf(pType, targetUriPath, StateLeaf, COMP_STATE_REMOVABLE) {
		cardState.Removable = String2Bool(&lineCardInfo.Removable, false)
	}
	if shouldExportLeaf(pType, targetUriPath, StateLeaf, COMP_STATE_MFG_DATE) {
		if mfg_date := reformatMfgDate(lineCardInfo.Mfg_Date); mfg_date != "" {
			cardState.MfgDate = &mfg_date
		}
	}
	if shouldExportLeaf(pType, targetUriPath, StateLeaf, COMP_STATE_HW_VER) {
		cardState.HardwareVersion = StringWithDefault(&lineCardInfo.Hardware_Version, defaultVal)
	}
	if shouldExportLeaf(pType, targetUriPath, StateLeaf, COMP_STATE_INSTALL_POSITION) {
		cardState.InstallPosition = StringWithDefault(&lineCardInfo.Install_Position, defaultVal)
	}
	if shouldExportLeaf(pType, targetUriPath, StateLeaf, COMP_STATE_INSTALL_COMPONENT) {
		cardState.InstallComponent = FirstNonEmptyOrNil(lineCardInfo.Install_Component, lineCardInfo.Parent, defaultParentVal)
	}
	if shouldExportLeaf(pType, targetUriPath, StateLeaf, COMP_STATE_TYPE) {
		cardState.Type, _ = cardState.To_OpenconfigPlatform_Components_Component_State_Type_Union(
			ocbinds.OpenconfigPlatformTypes_OPENCONFIG_HARDWARE_COMPONENT_LINECARD)
	}
	if shouldExportLeaf(pType, targetUriPath, StateLeaf, COMP_STATE_OPER_STATUS) {
		if operStatus, err := operStatusFromString(lineCardInfo.OperStatus); err == nil {
			cardState.OperStatus = operStatus
		}
	}
	if shouldExportLeaf(pType, targetUriPath, StateLeaf, COMP_STATE_EMPTY) {
		// If DB has a line card entry but no empty field, consider the line card present.
		cardState.Empty = String2Bool(&lineCardInfo.Empty, false)
	}

	return nil
}
