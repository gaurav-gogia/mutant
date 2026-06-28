//go:build darwin
// +build darwin

package security

import (
	"os"
	"os/exec"
	"strings"
)

func detectSandboxDarwin() (sandboxDetection, error) {
	var detection sandboxDetection

	typeScore := map[string]int{}
	indicators := make([]string, 0, 8)

	add := func(kind string, confidence int, indicator string) {
		if confidence <= 0 {
			return
		}
		typeScore[kind] += confidence
		indicators = append(indicators, indicator)
	}

	if envSet("APP_SANDBOX_CONTAINER_ID") {
		add("macOS App Sandbox", 85, "darwin:env:app_sandbox_container_id")
	}
	if envSet("DYLD_INSERT_LIBRARIES") {
		add("macOS App Sandbox", 35, "darwin:env:dyld_insert_libraries")
	}
	if envSet("COLIMA_HOME") {
		add("Colima", 80, "darwin:env:colima_home")
	}

	for _, path := range []struct {
		path  string
		kind  string
		score int
		mark  string
	}{
		{"/Applications/VMware Fusion.app", "VMware Fusion", 80, "darwin:file:vmware_fusion_app"},
		{"/Applications/VirtualBox.app", "VirtualBox", 80, "darwin:file:virtualbox_app"},
		{"/Applications/Parallels Desktop.app", "Parallels", 80, "darwin:file:parallels_app"},
		{"/Applications/UTM.app", "UTM", 80, "darwin:file:utm_app"},
		{"/Users/Shared/Parallels", "Parallels", 60, "darwin:file:parallels_shared"},
		{"/opt/homebrew/bin/colima", "Colima", 70, "darwin:file:colima_bin"},
	} {
		if _, err := os.Stat(path.path); err == nil {
			add(path.kind, path.score, path.mark)
		}
	}

	if out, err := exec.Command("ps", "-axo", "comm").CombinedOutput(); err == nil {
		procs := strings.ToLower(string(out))
		if strings.Contains(procs, "vmware-vmx") {
			add("VMware Fusion", 70, "darwin:process:vmware_vmx")
		}
		if strings.Contains(procs, "vboxservice") || strings.Contains(procs, "vboxclient") {
			add("VirtualBox", 70, "darwin:process:virtualbox_tools")
		}
		if strings.Contains(procs, "prl_tools") {
			add("Parallels", 70, "darwin:process:parallels_tools")
		}
		if strings.Contains(procs, "qemu-system") {
			add("QEMU", 70, "darwin:process:qemu_system")
		}
		if strings.Contains(procs, "colima") {
			add("Colima", 70, "darwin:process:colima")
		}
	}

	bestType := ""
	bestScore := 0
	for kind, score := range typeScore {
		if score > bestScore {
			bestType = kind
			bestScore = score
		}
	}

	if bestScore > 100 {
		bestScore = 100
	}

	detection.Type = bestType
	detection.Confidence = bestScore
	detection.Indicators = uniqueStrings(indicators)
	return detection, nil
}
