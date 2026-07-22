package transformer

import (
	"os"
	"regexp"
	"strings"
	"sync"
)

const sonicVersionYAMLPath = "/etc/sonic/sonic_version.yml"

type sonicVersionYAML struct {
	buildVersion   string
	sonicOSVersion string
	debianVersion  string
}

var (
	sonicVersionKeyRE = regexp.MustCompile(`^([a-z_]+):\s*['"]?([^'"]*)['"]?\s*$`)

	sonicVersionOnce sync.Once
	sonicVersionInfo sonicVersionYAML
	sonicVersionRead bool
)

func loadSonicVersionInfo() {
	data, err := os.ReadFile(sonicVersionYAMLPath)
	if err != nil {
		return
	}
	sonicVersionInfo = parseSonicVersionYAML(data)
	sonicVersionRead = true
}

func getSonicVersionInfo() (sonicVersionYAML, bool) {
	sonicVersionOnce.Do(loadSonicVersionInfo)
	return sonicVersionInfo, sonicVersionRead
}

func parseSonicVersionYAML(data []byte) sonicVersionYAML {
	var info sonicVersionYAML
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		m := sonicVersionKeyRE.FindStringSubmatch(line)
		if len(m) != 3 {
			continue
		}
		key, val := m[1], strings.TrimSpace(m[2])
		switch key {
		case "build_version":
			info.buildVersion = val
		case "sonic_os_version":
			info.sonicOSVersion = val
		case "debian_version":
			info.debianVersion = val
		}
	}
	return info
}

// formatSonicSoftwareVersion builds a software-version string aligned with
// "show version" output, e.g.:
// SONiC.nokia-cpn.0-c7939e4bf (SONiC OS 13, Debian 13.6)
func formatSonicSoftwareVersion(info sonicVersionYAML) string {
	buildVersion := strings.TrimSpace(info.buildVersion)
	sonicOSVersion := strings.TrimSpace(info.sonicOSVersion)
	debianVersion := strings.TrimSpace(info.debianVersion)

	var details []string
	if sonicOSVersion != "" {
		details = append(details, "SONiC OS "+sonicOSVersion)
	}
	if debianVersion != "" {
		details = append(details, "Debian "+debianVersion)
	}

	if buildVersion == "" {
		return strings.Join(details, ", ")
	}
	if len(details) == 0 {
		return "SONiC." + buildVersion
	}
	return "SONiC." + buildVersion + " (" + strings.Join(details, ", ") + ")"
}

func getFormattedSonicSoftwareVersion() string {
	info, ok := getSonicVersionInfo()
	if !ok {
		return ""
	}
	return formatSonicSoftwareVersion(info)
}

func resolveSoftwareVersion(swcInfo SWCompInfo, cType componentType) string {
	if cType == CompTypeOS && strings.EqualFold(strings.TrimSpace(swcInfo.OperStatus), "active") {
		if version := getFormattedSonicSoftwareVersion(); version != "" {
			return version
		}
	}
	return strings.TrimSpace(swcInfo.SoftwareVersion)
}
