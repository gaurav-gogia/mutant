package security

import (
	"strings"
	"testing"
)

// sandboxHostSoftwarePS renders commands the way ps -axo comm prints them: a
// header line, then one command per line.
func sandboxHostSoftwarePS(commands ...string) string {
	return "COMM\n" + strings.Join(commands, "\n") + "\n"
}

// sandboxHostSoftwareEnv answers scoreDarwinSandbox's isSet from a fixed list
// of variable names, so that no test here touches the process environment.
func sandboxHostSoftwareEnv(names ...string) func(string) bool {
	set := map[string]bool{}
	for _, name := range names {
		set[name] = true
	}
	return func(name string) bool { return set[name] }
}

// sandboxHostSoftwareMac is software a Mac that hosts virtual machines runs, in
// paths of the shape ps prints: VMware Fusion and its vmware-vmx, UTM, QEMU,
// VirtualBox, Parallels Desktop and Colima.
var sandboxHostSoftwareMac = []string{
	"/Applications/VMware Fusion.app/Contents/MacOS/VMware Fusion",
	"/Applications/VMware Fusion.app/Contents/Library/vmware-vmx",
	"/Applications/UTM.app/Contents/MacOS/UTM",
	"/opt/homebrew/bin/qemu-system-aarch64",
	"/Applications/VirtualBox.app/Contents/MacOS/VirtualBox",
	"/Applications/VirtualBox.app/Contents/MacOS/VBoxSVC",
	"/Applications/Parallels Desktop.app/Contents/MacOS/prl_client_app",
	"/opt/homebrew/bin/colima",
}

// TestDarwinSandboxRulesDoNotScoreHypervisorHostSoftware is M26-TMP-012. The
// macOS detector read the host side of virtualization as proof that the Mac was
// a guest: vmware-vmx, qemu-system or colima running scored 70 each and
// COLIMA_HOME 80, each at or over SandboxDetectedThreshold on its own, so secure
// mode refused to run on an examiner's Mac that merely had a hypervisor. The
// applications installed under /Applications scored 80 each as well; they are
// not an input any more, because the rules read no file.
//
// The applications are run here too, because they are what the guest-tool
// process rules that remain have to leave alone.
func TestDarwinSandboxRulesDoNotScoreHypervisorHostSoftware(t *testing.T) {
	cases := []struct {
		name      string
		env       []string
		processes []string
	}{
		{"VMware Fusion running a guest", nil, sandboxHostSoftwareMac[0:2]},
		{"UTM running", nil, sandboxHostSoftwareMac[2:3]},
		{"QEMU running a guest", nil, sandboxHostSoftwareMac[3:4]},
		{"VirtualBox running", nil, sandboxHostSoftwareMac[4:6]},
		{"Parallels Desktop running", nil, sandboxHostSoftwareMac[6:7]},
		{"Colima running", nil, sandboxHostSoftwareMac[7:8]},
		{"COLIMA_HOME set", []string{"COLIMA_HOME"}, nil},
		{"all of it at once", []string{"COLIMA_HOME"}, sandboxHostSoftwareMac},
	}
	for _, tc := range cases {
		detection := scoreDarwinSandbox(sandboxHostSoftwareEnv(tc.env...), sandboxHostSoftwarePS(tc.processes...), "")
		if detection.Type != "" || detection.Confidence != 0 || len(detection.Indicators) != 0 {
			t.Errorf("%s: hosting virtual machines does not make a Mac a guest, so this must score nothing; "+
				"got type=%q confidence=%d indicators=%v",
				tc.name, detection.Type, detection.Confidence, detection.Indicators)
		}
	}
}

// TestDarwinSandboxRulesStillScoreGuestEvidence is the other direction: what
// comes from inside a sandbox or a guest still counts, and the host software of
// the test above, run beside it, adds nothing to it.
func TestDarwinSandboxRulesStillScoreGuestEvidence(t *testing.T) {
	cases := []struct {
		name       string
		env        []string
		processes  []string
		vendor     string
		kind       string
		confidence int
		indicators []string
	}{
		{"VirtualBox Guest Additions service", nil,
			[]string{"/Library/Application Support/VirtualBox Guest Additions/VBoxService"}, "",
			"VirtualBox", 70, []string{"darwin:process:virtualbox_tools"}},
		{"VirtualBox Guest Additions client", nil, []string{"VBoxClient"}, "",
			"VirtualBox", 70, []string{"darwin:process:virtualbox_tools"}},
		{"Parallels Tools", nil, []string{"prl_tools_service"}, "",
			"Parallels", 70, []string{"darwin:process:parallels_tools"}},
		{"App Sandbox container", []string{"APP_SANDBOX_CONTAINER_ID"}, nil, "",
			"macOS App Sandbox", 85, []string{"darwin:env:app_sandbox_container_id"}},
		{"a hypervisor vendor in CPUID", nil, nil, "vmwarevmware",
			"VM", 35, []string{"darwin:cpuid:hypervisor"}},
		{"guest tools beside all of the host software", []string{"COLIMA_HOME"},
			append(append([]string(nil), sandboxHostSoftwareMac...),
				"/Library/Application Support/VirtualBox Guest Additions/VBoxService"), "",
			"VirtualBox", 70, []string{"darwin:process:virtualbox_tools"}},
	}
	for _, tc := range cases {
		detection := scoreDarwinSandbox(sandboxHostSoftwareEnv(tc.env...), sandboxHostSoftwarePS(tc.processes...), tc.vendor)
		if detection.Type != tc.kind || detection.Confidence != tc.confidence ||
			strings.Join(detection.Indicators, ",") != strings.Join(tc.indicators, ",") {
			t.Errorf("%s: want type=%q confidence=%d indicators=%v; got type=%q confidence=%d indicators=%v",
				tc.name, tc.kind, tc.confidence, tc.indicators,
				detection.Type, detection.Confidence, detection.Indicators)
		}
	}
}
