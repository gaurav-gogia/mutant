//go:build windows
// +build windows

package security

import (
	"encoding/csv"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func detectSandboxWindows() (sandboxDetection, error) {
	typeScore := map[string]int{}
	indicators := make([]string, 0, 8)

	add := func(kind string, confidence int, indicator string) {
		if confidence <= 0 {
			return
		}
		typeScore[kind] += confidence
		indicators = append(indicators, indicator)
	}

	var detection sandboxDetection

	for _, path := range []struct {
		path  string
		kind  string
		score int
		mark  string
	}{
		{`C:\\windows\\system32\\drivers\\vmmouse.sys`, "VMware", 80, "windows:file:vmmouse.sys"},
		{`C:\\windows\\system32\\drivers\\vmhgfs.sys`, "VMware", 80, "windows:file:vmhgfs.sys"},
		{`C:\\windows\\system32\\drivers\\VBoxMouse.sys`, "VirtualBox", 80, "windows:file:vboxmouse.sys"},
		{`C:\\windows\\system32\\drivers\\VBoxGuest.sys`, "VirtualBox", 80, "windows:file:vboxguest.sys"},
		{`C:\\windows\\system32\\drivers\\xenbus.sys`, "Xen", 80, "windows:file:xenbus.sys"},
		{`C:\\windows\\system32\\SbieDll.dll`, "Sandboxie", 85, "windows:file:sbiedll.dll"},
	} {
		if _, err := os.Stat(path.path); err == nil {
			add(path.kind, path.score, path.mark)
		}
	}

	if envSet("SANDBOXIE") {
		add("Sandboxie", 85, "windows:env:sandboxie")
	}
	if envSet("CUCKOO") {
		add("Cuckoo", 85, "windows:env:cuckoo")
	}
	if envSet("VBOX_INSTALL_PATH") {
		add("VirtualBox", 70, "windows:env:vbox_install_path")
	}
	addWindowsEnvIndicators(os.LookupEnv, add)
	if wd, err := os.Getwd(); err == nil {
		addWindowsWSLCwdIndicators(wd, add)
	}
	if parent, err := getWindowsParentProcessName(os.Getppid); err == nil {
		addWindowsWSLParentIndicators(parent, add)
	}

	if user, ok := os.LookupEnv("USERNAME"); ok && strings.EqualFold(strings.TrimSpace(user), "WDAGUtilityAccount") {
		add("Windows Sandbox", 95, "windows:env:wdag_utility_account")
	}
	if profile, ok := os.LookupEnv("USERPROFILE"); ok {
		profile = strings.ToLower(strings.TrimSpace(profile))
		if strings.Contains(profile, `\users\wdagutilityaccount`) {
			add("Windows Sandbox", 95, "windows:env:userprofile_wdag")
		}
	}

	if out, err := exec.Command("tasklist").CombinedOutput(); err == nil {
		addWindowsProcessIndicators(strings.ToLower(string(out)), add)
	}

	detection = finalizeWindowsDetection(typeScore, indicators)
	return detection, nil
}

func addWindowsProcessIndicators(procs string, add func(kind string, confidence int, indicator string)) {
	procs = strings.ToLower(procs)

	if strings.Contains(procs, "vmtoolsd.exe") || strings.Contains(procs, "vmwaretray.exe") {
		add("VMware", 70, "windows:process:vmware_tools")
	}
	if strings.Contains(procs, "vboxservice.exe") || strings.Contains(procs, "vboxtray.exe") {
		add("VirtualBox", 70, "windows:process:virtualbox_tools")
	}
	if strings.Contains(procs, "xenservice.exe") {
		add("Xen", 70, "windows:process:xenservice")
	}
	if strings.Contains(procs, "qemu-ga.exe") {
		add("KVM/QEMU", 70, "windows:process:qemu_guest_agent")
	}
	if strings.Contains(procs, "sbiectrl.exe") || strings.Contains(procs, "sandboxiedcomlaunch.exe") {
		add("Sandboxie", 85, "windows:process:sandboxie")
	}

	if containsAny(procs, []string{"vmicheartbeat.exe", "vmicvss.exe", "vmicrdv.exe", "vmicshutdown.exe", "vmictimesync.exe", "vmicvmsession.exe"}) {
		add("Hyper-V", 80, "windows:process:hyperv_guest_integration")
	}
}

func addWindowsEnvIndicators(lookupEnv func(string) (string, bool), add func(kind string, confidence int, indicator string)) {
	for _, name := range []string{"WSL_INTEROP", "WSL_DISTRO_NAME", "WSLENV"} {
		if value, ok := lookupEnv(name); ok && strings.TrimSpace(value) != "" {
			add("WSL", 95, "windows:env:wsl_context")
			return
		}
	}
}

func addWindowsWSLCwdIndicators(cwd string, add func(kind string, confidence int, indicator string)) {
	path := strings.ToLower(strings.TrimSpace(cwd))
	if strings.HasPrefix(path, `\\wsl$\`) || strings.HasPrefix(path, `\\wsl.localhost\`) {
		add("WSL", 90, "windows:cwd:wsl_unc_path")
	}
}

func addWindowsWSLParentIndicators(parentName string, add func(kind string, confidence int, indicator string)) {
	parent := strings.ToLower(strings.TrimSpace(parentName))
	if containsAny(parent, []string{"wsl.exe", "wslhost.exe", "bash.exe"}) {
		add("WSL", 90, "windows:process_parent:wsl")
	}
}

func getWindowsParentProcessName(getppid func() int) (string, error) {
	ppid := getppid()
	if ppid <= 0 {
		return "", nil
	}
	out, err := exec.Command("tasklist", "/fo", "csv", "/nh", "/fi", fmt.Sprintf("PID eq %d", ppid)).CombinedOutput()
	if err != nil {
		return "", err
	}
	return parseTasklistImageName(out)
}

func parseTasklistImageName(out []byte) (string, error) {
	raw := strings.TrimSpace(string(out))
	if raw == "" {
		return "", nil
	}
	reader := csv.NewReader(strings.NewReader(raw))
	rec, err := reader.Read()
	if err != nil {
		return "", err
	}
	if len(rec) == 0 {
		return "", nil
	}
	imageName := strings.ToLower(strings.TrimSpace(rec[0]))
	if strings.HasPrefix(imageName, "info:") {
		return "", nil
	}
	return imageName, nil
}

func finalizeWindowsDetection(typeScore map[string]int, indicators []string) sandboxDetection {
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

	return sandboxDetection{
		Type:       bestType,
		Confidence: bestScore,
		Indicators: uniqueStrings(indicators),
	}
}
