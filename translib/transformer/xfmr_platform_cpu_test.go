package transformer

import (
	"reflect"
	"sync"
	"testing"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	"github.com/Azure/sonic-mgmt-common/translib/ocbinds"
	"github.com/openconfig/ygot/ygot"
)

/* Replaces the cached cpu model for the duration of a test. */
func setCpuModel(t *testing.T, model string) {
	cpuModelOnce = sync.Once{}
	cpuModelOnce.Do(func() { cpuModel = model })
	t.Cleanup(func() {
		cpuModelOnce = sync.Once{}
		cpuModel = ""
	})
}

func TestParseCpuModel(t *testing.T) {
	cases := []struct {
		desc, in, want string
	}{
		{"x86", "processor\t: 0\nvendor_id\t: AuthenticAMD\nmodel name\t: AMD Ryzen Embedded V3C48 8-Core Processor\nprocessor\t: 1\nmodel name\t: other\n",
			"AMD Ryzen Embedded V3C48 8-Core Processor"},
		{"arm has no model name", "processor\t: 0\nBogoMIPS\t: 100.00\nCPU implementer\t: 0x41\nCPU part\t: 0xd08\n", ""},
		{"empty", "", ""},
	}
	for _, c := range cases {
		if got := parseCpuModel(c.in); got != c.want {
			t.Errorf("%s: parseCpuModel() == %q, want %q", c.desc, got, c.want)
		}
	}
}

func TestGetCompTypeByNameCpu(t *testing.T) {
	if got, err := getCompTypeByName(CPU_COMP_NAME); got != CompTypeCpu || err != nil {
		t.Errorf("getCompTypeByName(%q) == (%v, %v), want (%v, nil)", CPU_COMP_NAME, got, err, CompTypeCpu)
	}
}

func TestCompTypesForSubscriptionUriCpu(t *testing.T) {
	if got := compTypesForSubscriptionUri(COMP_CPU); !reflect.DeepEqual(got, []componentType{CompTypeCpu}) {
		t.Errorf("compTypesForSubscriptionUri(%q) == %v, want [%v]", COMP_CPU, got, CompTypeCpu)
	}
}

// A whole-component GET carries identity, description, utilization and the
// host properties.
func TestFillCpuInfoWholeComponent(t *testing.T) {
	const model = "Test CPU"
	setCpuModel(t, model)
	var dbs [db.MaxDB]*db.DB
	comp := &ocbinds.OpenconfigPlatform_Components_Component{}
	if err := compTypeToFuncCall(CompTypeCpu, CPU_COMP_NAME, "", comp, COMP, dbs, AllPaths, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	st := comp.State
	if st.Name == nil || *st.Name != CPU_COMP_NAME {
		t.Errorf("name == %v, want %q", st.Name, CPU_COMP_NAME)
	}
	if v, ok := st.Type.(*ocbinds.OpenconfigPlatform_Components_Component_State_Type_Union_E_OpenconfigPlatformTypes_OPENCONFIG_HARDWARE_COMPONENT); !ok ||
		v.E_OpenconfigPlatformTypes_OPENCONFIG_HARDWARE_COMPONENT != ocbinds.OpenconfigPlatformTypes_OPENCONFIG_HARDWARE_COMPONENT_CPU {
		t.Errorf("type == %v, want CPU", st.Type)
	}
	if st.Parent == nil || *st.Parent != CHASSIS_COMP_NAME {
		t.Errorf("parent == %v, want %q", st.Parent, CHASSIS_COMP_NAME)
	}
	if st.Description == nil || *st.Description != model {
		t.Errorf("description == %v, want %q", st.Description, model)
	}
	if st.OperStatus != ocbinds.OpenconfigPlatformTypes_COMPONENT_OPER_STATUS_ACTIVE {
		t.Errorf("oper-status == %v, want ACTIVE", st.OperStatus)
	}
	if comp.Cpu.Utilization.State.Instant == nil {
		t.Error("cpu utilization not set")
	}
	for name := range nonDbProps {
		if _, ok := comp.Properties.Property[name]; !ok {
			t.Errorf("property %q missing", name)
		}
	}
}

func TestFillCpuInfoLeaves(t *testing.T) {
	newComp := func() *ocbinds.OpenconfigPlatform_Components_Component {
		comp := &ocbinds.OpenconfigPlatform_Components_Component{}
		ygot.BuildEmptyTree(comp)
		return comp
	}

	t.Run("parent", func(t *testing.T) {
		setCpuModel(t, "")
		comp := newComp()
		if err := fillCpuInfo(comp, SingularPath, COMP_STATE_PARENT); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if comp.State.Parent == nil || *comp.State.Parent != CHASSIS_COMP_NAME {
			t.Errorf("parent == %v, want %q", comp.State.Parent, CHASSIS_COMP_NAME)
		}
	})

	t.Run("oper-status", func(t *testing.T) {
		setCpuModel(t, "")
		comp := newComp()
		if err := fillCpuInfo(comp, SingularPath, COMP_STATE_OPER_STATUS); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if comp.State.OperStatus != ocbinds.OpenconfigPlatformTypes_COMPONENT_OPER_STATUS_ACTIVE {
			t.Errorf("oper-status == %v, want ACTIVE", comp.State.OperStatus)
		}
	})

	t.Run("description", func(t *testing.T) {
		setCpuModel(t, "Test CPU")
		comp := newComp()
		if err := fillCpuInfo(comp, SingularPath, COMP_STATE_DESCR); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if comp.State.Description == nil || *comp.State.Description != "Test CPU" {
			t.Errorf("description == %v, want %q", comp.State.Description, "Test CPU")
		}
	})

	t.Run("no model is NotFound", func(t *testing.T) {
		setCpuModel(t, "")
		if err := fillCpuInfo(newComp(), SingularPath, COMP_STATE_DESCR); !isNotFound(err) {
			t.Fatalf("fillCpuInfo(description) returned %T (%v), want tlerr.NotFoundError", err, err)
		}
	})

	for _, uri := range []string{COMP_STATE_SERIAL_NO, COMP_STATE_PART_NO, COMP_STATE_MFG_NAME} {
		t.Run("targeted "+uri, func(t *testing.T) {
			setCpuModel(t, "Test CPU")
			if err := fillCpuInfo(newComp(), SingularPath, uri); !isUnsupportedLeaf(err) {
				t.Fatalf("fillCpuInfo(%s) returned %v, want an unsupported leaf path NotFoundError", uri, err)
			}
		})
	}
}

// The host properties are no longer served on the chassis.
func TestFillPropertiesChassisHasNoHostCpu(t *testing.T) {
	var dbs [db.MaxDB]*db.DB
	comp := &ocbinds.OpenconfigPlatform_Components_Component{}
	_ = fillProperties(comp, "chassis 1", CompTypeChassis, "", dbs)
	if comp.Properties == nil {
		return
	}
	for name := range nonDbProps {
		if _, ok := comp.Properties.Property[name]; ok {
			t.Errorf("property %q served on the chassis", name)
		}
	}
}

// The host CPU component created for a whole-tree get carries healthz.
func TestCreateCpuCompHealthz(t *testing.T) {
	setCpuModel(t, "")
	d, err := db.NewDB(getDBOptions(db.StateDB))
	if err != nil {
		t.Fatal("NewDB failed")
	}
	defer d.DeleteDB()

	device := &ocbinds.Device{}
	ygot.BuildEmptyTree(device)
	var inParams XfmrParams
	inParams.dbs[db.StateDB] = d
	createCpuComp(device.Components, COMP, inParams)

	comp := device.Components.Component[CPU_COMP_NAME]
	if comp == nil || comp.Healthz == nil || comp.Healthz.State == nil {
		t.Fatal("host-cpu healthz/state is unset")
	}
	if got := comp.Healthz.State.Status; got != ocbinds.OpenconfigPlatform_Components_Component_Healthz_State_Status_HEALTHY {
		t.Errorf("healthz/state/status = %v, want HEALTHY", got)
	}
}
