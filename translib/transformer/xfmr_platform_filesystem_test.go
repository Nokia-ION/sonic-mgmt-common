package transformer

import (
	"os"
	"reflect"
	"syscall"
	"testing"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	"github.com/Azure/sonic-mgmt-common/translib/ocbinds"
	"github.com/Azure/sonic-mgmt-common/translib/tlerr"
	"github.com/openconfig/ygot/ygot"
)

func TestGetCompTypeByNameFilesystem(t *testing.T) {
	cases := []struct {
		in   string
		want componentType
	}{
		{"/", CompTypeFilesystem},
		{"/var/log", CompTypeFilesystem},
		{"/sys/kernel/debug/tracing", CompTypeFilesystem},
		// A name with no leading slash must not be claimed as a filesystem.
		{"var/log", CompTypeInvalid},
		{"", CompTypeInvalid},
	}
	for _, c := range cases {
		got, err := getCompTypeByName(c.in)
		if got != c.want {
			t.Errorf("getCompTypeByName(%q) == %v, want %v", c.in, got, c.want)
		}
		if (err == nil) != (c.want != CompTypeInvalid) {
			t.Errorf("getCompTypeByName(%q) returned err %v, inconsistent with type %v", c.in, err, got)
		}
	}
}

// Pins the two derivations a mix-up would leave plausible-looking: used is total
// minus free, not total minus available, and byte counts scale by Frsize.
func TestFsPropFields(t *testing.T) {
	st := syscall.Statfs_t{
		Blocks: 1011640,
		Bfree:  915617,
		Bavail: 859093,
		Frsize: 4096,
		// Deliberately unequal to Frsize so a Bsize mix-up cannot pass.
		Bsize: 8192,
	}
	want := map[string]uint64{
		"total_blocks":     1011640,
		"free_blocks":      915617,
		"available_blocks": 859093,
		"total_bytes":      1011640 * 4096,
		"free_bytes":       915617 * 4096,
		"available_bytes":  859093 * 4096,
		"used_blocks":      96023,
		"used_bytes":       96023 * 4096,
	}
	if len(fsPropFields) != len(want) {
		t.Fatalf("fsPropFields has %d entries, want %d", len(fsPropFields), len(want))
	}
	for name, field := range fsPropFields {
		expected, ok := want[name]
		if !ok {
			t.Errorf("unexpected property %q in fsPropFields", name)
			continue
		}
		if got := field(&st); got != expected {
			t.Errorf("fsPropFields[%q] == %d, want %d", name, got, expected)
		}
	}
}

func TestHostPath(t *testing.T) {
	// The prefix is applied only when /mnt/host exists, so assert against
	// whichever branch this machine is in.  The last two cases must not escape.
	_, err := os.Stat("/mnt/host")
	cases := []struct {
		in       string
		bare     string
		prefixed string
	}{
		{"/", "/", "/mnt/host"},
		{"/var/log", "/var/log", "/mnt/host/var/log"},
		{"/dev/shm", "/dev/shm", "/mnt/host/dev/shm"},
		{"/../etc", "/etc", "/mnt/host/etc"},
		{"/var/log/../..", "/", "/mnt/host"},
	}
	for _, c := range cases {
		want := c.bare
		if err == nil {
			want = c.prefixed
		}
		if got := hostPath(c.in); got != want {
			t.Errorf("hostPath(%q) == %q, want %q", c.in, got, want)
		}
	}
}

// A filesystem name that does not resolve must yield no component at all, not a
// typed one with no properties.
func TestFillFilesystemInfoUnresolvedName(t *testing.T) {
	comp := &ocbinds.OpenconfigPlatform_Components_Component{}
	err := fillFilesystemInfo(comp, "/no/such/mount/point", "")
	if err == nil {
		t.Fatal("fillFilesystemInfo on a nonexistent path returned nil error")
	}
	if comp.State != nil || comp.Properties != nil {
		t.Errorf("fillFilesystemInfo populated the component despite failing: %v", comp)
	}
}

// The table holds exactly the listed mount points that resolve, replacing any
// earlier rows.
func TestWriteFilesystemTable(t *testing.T) {
	d, err := db.NewDB(getDBOptions(db.StateDB))
	if err != nil {
		t.Fatal("NewDB failed")
	}
	defer d.DeleteDB()
	ts := &db.TableSpec{Name: FILESYSTEM_TBL}
	defer d.DeleteKeys(ts, db.Key{Comp: []string{"*"}})

	saved := fsComponentNames
	defer func() { fsComponentNames = saved }()
	fsComponentNames = []string{"/", "/no/such/mount/point"}

	stale := db.Value{Field: map[string]string{"NULL": "NULL"}}
	if err := d.SetEntry(ts, db.Key{Comp: []string{"/stale"}}, stale); err != nil {
		t.Fatalf("seeding a stale row failed: %v", err)
	}
	if err := writeFilesystemTable(d); err != nil {
		t.Fatalf("writeFilesystemTable failed: %v", err)
	}

	keys, err := d.GetKeysPattern(ts, db.Key{Comp: []string{"*"}})
	if err != nil {
		t.Fatalf("reading %s failed: %v", FILESYSTEM_TBL, err)
	}
	if len(keys) != 1 || keys[0].Get(0) != "/" {
		t.Errorf("%s keys == %v, want only /", FILESYSTEM_TBL, keys)
	}
}

// The component list builds a filesystem component from each table row.
func TestCreateCompAndFuncCallFilesystem(t *testing.T) {
	d, err := db.NewDB(getDBOptions(db.StateDB))
	if err != nil {
		t.Fatal("NewDB failed")
	}
	defer d.DeleteDB()
	ts := &db.TableSpec{Name: FILESYSTEM_TBL}
	defer d.DeleteKeys(ts, db.Key{Comp: []string{"*"}})
	if err := d.SetEntry(ts, db.Key{Comp: []string{"/"}}, db.Value{Field: map[string]string{"NULL": "NULL"}}); err != nil {
		t.Fatalf("seeding %s failed: %v", FILESYSTEM_TBL, err)
	}

	device := &ocbinds.Device{}
	ygot.BuildEmptyTree(device)
	var inParams XfmrParams
	inParams.dbs[db.StateDB] = d
	tbl := compTblMap[CompTypeFilesystem]
	createCompAndFuncCall(device.Components, COMP, CompTypeFilesystem, inParams, tbl[0], tbl[1])

	comp, ok := device.Components.Component["/"]
	if !ok || comp.State == nil || comp.State.Name == nil || *comp.State.Name != "/" {
		t.Fatalf("component / missing or without state/name: %v", comp)
	}
	if got := len(comp.Properties.Property); got != len(fsPropFields) {
		t.Errorf("emitted %d properties, want %d", got, len(fsPropFields))
	}
}

// A wildcard subscribe watches the filesystem table without turning off
// on-change for the other component types.
func TestSubscribe_pfm_components_filesystem_wildcard(t *testing.T) {
	d, err := db.NewDB(getDBOptions(db.StateDB))
	if err != nil {
		t.Fatal("NewDB failed")
	}
	defer d.DeleteDB()

	uri := "/openconfig-platform:components/component[name=*]/state/type"
	in := XfmrSubscInParams{uri: uri, requestURI: uri, subscProc: TRANSLATE_SUBSCRIBE}
	in.dbs[db.StateDB] = d
	out, err := Subscribe_pfm_components_xfmr(in)
	if err != nil {
		t.Fatalf("Subscribe_pfm_components_xfmr failed: %v", err)
	}
	if _, ok := out.dbDataMap[db.StateDB][FILESYSTEM_TBL]["*"]; !ok {
		t.Errorf("%s|* not watched: %v", FILESYSTEM_TBL, out.dbDataMap)
	}
	if out.onChange != OnchangeEnable {
		t.Errorf("onChange == %v, want OnchangeEnable", out.onChange)
	}
}

// A property name that is not one of the eight must be reported as not found,
// not answered with an empty component the caller cannot interpret.
func TestFillFilesystemInfoUnknownProperty(t *testing.T) {
	comp := &ocbinds.OpenconfigPlatform_Components_Component{}
	err := fillFilesystemInfo(comp, "/", "bogus")
	if _, ok := err.(tlerr.NotFoundError); !ok {
		t.Fatalf("fillFilesystemInfo with an unknown property returned %T (%v), want tlerr.NotFoundError", err, err)
	}
	if comp.State != nil || comp.Properties != nil {
		t.Errorf("fillFilesystemInfo populated the component despite failing: %v", comp)
	}
}

func TestFillFilesystemInfo(t *testing.T) {
	// "/" resolves everywhere this test can run, including a build container.
	comp := &ocbinds.OpenconfigPlatform_Components_Component{}
	if err := fillFilesystemInfo(comp, "/", ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if comp.State == nil || comp.State.Name == nil || *comp.State.Name != "/" {
		t.Errorf("state/name not set to the component name: %v", comp.State)
	}
	// The union members are pointers to generated structs, so compare with
	// DeepEqual rather than ==, which would compare the pointers.
	wantType, err := comp.State.To_OpenconfigPlatform_Components_Component_State_Type_Union(
		ocbinds.OpenconfigPlatformTypes_OPENCONFIG_HARDWARE_COMPONENT_STORAGE)
	if err != nil {
		t.Fatalf("unexpected error building the expected type union: %v", err)
	}
	if !reflect.DeepEqual(comp.State.Type, wantType) {
		t.Errorf("state/type == %v, want %v", comp.State.Type, wantType)
	}

	if got := len(comp.Properties.Property); got != len(fsPropFields) {
		t.Errorf("emitted %d properties, want %d", got, len(fsPropFields))
	}
	for name := range fsPropFields {
		prop, ok := comp.Properties.Property[name]
		if !ok {
			t.Errorf("property %q missing", name)
			continue
		}
		if prop.State.Name == nil || *prop.State.Name != name {
			t.Errorf("property %q has state/name %v", name, prop.State.Name)
		}
		if prop.State.Value == nil {
			t.Errorf("property %q has no state/value", name)
		}
	}

	// Checks the value reaches the leaf unchanged.  The Frsize scaling itself is
	// pinned by TestFsPropFields, which uses an unequal Bsize; this cannot, since
	// the two are equal on any filesystem this test runs against.
	var stat syscall.Statfs_t
	if err := syscall.Statfs(hostPath("/"), &stat); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	totalBytes := comp.Properties.Property["total_bytes"].State
	wantBytes, err := totalBytes.
		To_OpenconfigPlatform_Components_Component_Properties_Property_State_Value_Union(stat.Blocks * uint64(stat.Frsize))
	if err != nil {
		t.Fatalf("unexpected error building the expected value union: %v", err)
	}
	if !reflect.DeepEqual(totalBytes.Value, wantBytes) {
		t.Errorf("total_bytes == %v, want %v", totalBytes.Value, wantBytes)
	}
}

// A property name in the request must select exactly that one property.
func TestFillFilesystemInfoSingleProperty(t *testing.T) {
	comp := &ocbinds.OpenconfigPlatform_Components_Component{}
	if err := fillFilesystemInfo(comp, "/", "used_bytes"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := len(comp.Properties.Property); got != 1 {
		t.Fatalf("emitted %d properties, want 1: %v", got, comp.Properties.Property)
	}
	if _, ok := comp.Properties.Property["used_bytes"]; !ok {
		t.Errorf("used_bytes missing: %v", comp.Properties.Property)
	}
}

// The framework pre-creates the list entries named in the request URI, so the
// fill must populate an existing property rather than skipping it.
func TestFillFilesystemInfoPreCreatedProperty(t *testing.T) {
	comp := &ocbinds.OpenconfigPlatform_Components_Component{}
	ygot.BuildEmptyTree(comp)
	if _, err := comp.Properties.NewProperty("free_bytes"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := fillFilesystemInfo(comp, "/", "free_bytes"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	prop := comp.Properties.Property["free_bytes"]
	if prop == nil || prop.State == nil || prop.State.Value == nil {
		t.Errorf("pre-created property was not populated: %v", prop)
	}
}

// A named filesystem subscribe watches its own row and must disable on-change,
// which needs an explicit assignment because the zero value is OnchangeDefault.
func TestSubscribe_pfm_components_filesystem(t *testing.T) {
	d, err := db.NewDB(getDBOptions(db.StateDB))
	if err != nil {
		t.Fatal("NewDB failed")
	}
	defer d.DeleteDB()

	for _, uri := range []string{
		"/openconfig-platform:components/component[name=/var/log]",
		"/openconfig-platform:components/component[name=/var/log]/properties/property[name=used_bytes]/state/value",
	} {
		for _, proc := range []SubscProcType{TRANSLATE_EXISTS, TRANSLATE_SUBSCRIBE} {
			t.Run(uri, func(t *testing.T) {
				in := XfmrSubscInParams{uri: uri, requestURI: uri, subscProc: proc}
				in.dbs[db.StateDB] = d
				out, err := Subscribe_pfm_components_xfmr(in)
				if err != nil {
					t.Fatalf("Subscribe_pfm_components_xfmr(%v) failed: %v", proc, err)
				}
				if out.isVirtualTbl {
					t.Errorf("isVirtualTbl is true for %v; the row must be validated", proc)
				}
				if _, ok := out.dbDataMap[db.StateDB][FILESYSTEM_TBL]["/var/log"]; !ok {
					t.Errorf("%s|/var/log not mapped for %v: %v", FILESYSTEM_TBL, proc, out.dbDataMap)
				}
				if proc == TRANSLATE_SUBSCRIBE && out.onChange != OnchangeDisable {
					t.Errorf("onChange == %v, want OnchangeDisable", out.onChange)
				}
			})
		}
	}
}
