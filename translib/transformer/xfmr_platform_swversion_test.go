package transformer

import "testing"

func TestParseSonicVersionYAML(t *testing.T) {
	data := []byte(`build_version: 'nokia-cpn.0-c7939e4bf'
sonic_os_version: '13'
debian_version: '13.6'
kernel_version: '6.1.0'
`)
	info := parseSonicVersionYAML(data)
	if info.buildVersion != "nokia-cpn.0-c7939e4bf" {
		t.Fatalf("buildVersion=%q", info.buildVersion)
	}
	if info.sonicOSVersion != "13" {
		t.Fatalf("sonicOSVersion=%q", info.sonicOSVersion)
	}
	if info.debianVersion != "13.6" {
		t.Fatalf("debianVersion=%q", info.debianVersion)
	}
}

func TestFormatSonicSoftwareVersion(t *testing.T) {
	cases := []struct {
		info sonicVersionYAML
		want string
	}{
		{
			info: sonicVersionYAML{
				buildVersion:   "nokia-cpn.0-c7939e4bf",
				sonicOSVersion: "13",
				debianVersion:  "13.6",
			},
			want: "SONiC.nokia-cpn.0-c7939e4bf (SONiC OS 13, Debian 13.6)",
		},
		{
			info: sonicVersionYAML{buildVersion: "master.487-a98cf221"},
			want: "SONiC.master.487-a98cf221",
		},
		{
			info: sonicVersionYAML{sonicOSVersion: "13", debianVersion: "13.6"},
			want: "SONiC OS 13, Debian 13.6",
		},
	}
	for _, c := range cases {
		if got := formatSonicSoftwareVersion(c.info); got != c.want {
			t.Errorf("formatSonicSoftwareVersion(%+v)=%q, want %q", c.info, got, c.want)
		}
	}
}

func TestResolveSoftwareVersion(t *testing.T) {
	swcInfo := SWCompInfo{
		SoftwareVersion: "legacy-version",
		OperStatus:      "active",
	}
	if got := resolveSoftwareVersion(swcInfo, CompTypeBootLoader); got != "legacy-version" {
		t.Fatalf("bootloader resolve=%q, want legacy-version", got)
	}

	swcInfo.OperStatus = "inactive"
	if got := resolveSoftwareVersion(swcInfo, CompTypeOS); got != "legacy-version" {
		t.Fatalf("inactive os resolve=%q, want legacy-version", got)
	}
}
