package security

import "strings"

// The macOS sandbox detector's rules: which signals it scores, what each is
// worth, and the type each one names. They sit outside sandbox_darwin.go, which
// builds only on macOS, so that they can be run from any host;
// detectSandboxDarwin reads the machine and hands what it read to
// scoreDarwinSandbox.
//
// Hosting virtual machines does not make a Mac a guest, and nothing a Mac
// carries for hosting them is scored: VMware Fusion, VirtualBox, Parallels
// Desktop or UTM under /Applications, /Users/Shared/Parallels, a Homebrew
// colima, COLIMA_HOME, or vmware-vmx, qemu-system or colima running. Each of
// those is the host side of virtualization, and all but /Users/Shared/Parallels
// used to reach SandboxDetectedThreshold on its own, so secure mode refused to
// run on an examiner's Mac with a hypervisor installed (M26-TMP-012). What is
// left for virtualization is evidence from inside a guest: the guest tools that
// VirtualBox and Parallels install in the machines they run, and the CPU's
// hypervisor vendor.
const (
	darwinSandboxTypeAppSandbox = "macOS App Sandbox"
	darwinSandboxTypeVirtualBox = "VirtualBox"
	darwinSandboxTypeParallels  = "Parallels"
	darwinSandboxTypeVM         = "VM"

	darwinConfidenceAppSandboxEnv = 85
	darwinConfidenceDYLDEnv       = 35
	darwinConfidenceVMProcess     = 70
	darwinConfidenceCPUIDVendor   = 35

	darwinEnvAppSandboxContainer = "APP_SANDBOX_CONTAINER_ID"
	darwinEnvDYLDInsertLibraries = "DYLD_INSERT_LIBRARIES"

	darwinIndicatorEnvAppSandbox = "darwin:env:app_sandbox_container_id"
	darwinIndicatorEnvDYLDInsert = "darwin:env:dyld_insert_libraries"
	darwinIndicatorProcVBox      = "darwin:process:virtualbox_tools"
	darwinIndicatorProcParallels = "darwin:process:parallels_tools"
	darwinIndicatorCPUIDHypervis = "darwin:cpuid:hypervisor"
)

// scoreDarwinSandbox turns what the macOS detector read into a detection. isSet
// reports whether an environment variable holds anything but blanks, processes
// is what ps -axo comm printed (empty when it could not be run), and
// hypervisorVendor is the CPU's hypervisor vendor string.
func scoreDarwinSandbox(isSet func(name string) bool, processes string, hypervisorVendor string) sandboxDetection {
	typeScore := map[string]int{}
	indicators := make([]string, 0, 8)

	add := func(kind string, confidence int, indicator string) {
		if confidence <= 0 {
			return
		}
		typeScore[kind] += confidence
		indicators = append(indicators, indicator)
	}

	if isSet(darwinEnvAppSandboxContainer) {
		add(darwinSandboxTypeAppSandbox, darwinConfidenceAppSandboxEnv, darwinIndicatorEnvAppSandbox)
	}
	if isSet(darwinEnvDYLDInsertLibraries) {
		add(darwinSandboxTypeAppSandbox, darwinConfidenceDYLDEnv, darwinIndicatorEnvDYLDInsert)
	}

	procs := strings.ToLower(processes)
	if strings.Contains(procs, "vboxservice") || strings.Contains(procs, "vboxclient") {
		add(darwinSandboxTypeVirtualBox, darwinConfidenceVMProcess, darwinIndicatorProcVBox)
	}
	if strings.Contains(procs, "prl_tools") {
		add(darwinSandboxTypeParallels, darwinConfidenceVMProcess, darwinIndicatorProcParallels)
	}

	if hasAnyHypervisorVendor(hypervisorVendor) {
		add(darwinSandboxTypeVM, darwinConfidenceCPUIDVendor, darwinIndicatorCPUIDHypervis)
	}

	bestType := ""
	bestScore := 0
	for kind, score := range typeScore {
		if score > bestScore {
			bestType = kind
			bestScore = score
		}
	}

	if bestScore > maxConfidenceScore {
		bestScore = maxConfidenceScore
	}

	return sandboxDetection{
		Type:       bestType,
		Confidence: bestScore,
		Indicators: uniqueStrings(indicators),
	}
}
