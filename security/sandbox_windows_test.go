//go:build windows
// +build windows

package security

import (
	"strings"
	"testing"
)

func TestDetectSandboxWindowsFromTasklistHyperV(t *testing.T) {
	detection := detectSandboxWindowsFromTasklist("vmicheartbeat.exe vmictimesync.exe")
	if detection.Type != "Hyper-V" {
		t.Fatalf("expected Hyper-V detection, got %q", detection.Type)
	}
	if detection.Confidence < 80 {
		t.Fatalf("expected Hyper-V confidence >= 80, got %d", detection.Confidence)
	}
	if len(detection.Indicators) == 0 {
		t.Fatalf("expected Hyper-V indicators")
	}
}

func TestDetectSandboxWindowsFromTasklistHostHyperVProcessesNoSignal(t *testing.T) {
	detection := detectSandboxWindowsFromTasklist("vmcompute.exe vmwp.exe vmms.exe")
	if detection.Type != "" || detection.Confidence != 0 {
		t.Fatalf("expected no sandbox signal from host Hyper-V processes, got type=%q confidence=%d", detection.Type, detection.Confidence)
	}
}

func TestDetectSandboxWindowsFromTasklistHostWSLProcessesNoSignal(t *testing.T) {
	detection := detectSandboxWindowsFromTasklist("wslhost.exe wslservice.exe vmmemwsl.exe")
	if detection.Type != "" || detection.Confidence != 0 {
		t.Fatalf("expected no sandbox signal from host WSL processes, got type=%q confidence=%d", detection.Type, detection.Confidence)
	}
}

func TestDetectSandboxWindowsFromEnvWSL(t *testing.T) {
	detection := detectSandboxWindowsFromEnv(map[string]string{"WSL_INTEROP": `/run/WSL/9_interop`})
	if detection.Type != "WSL" {
		t.Fatalf("expected WSL detection, got %q", detection.Type)
	}
	if detection.Confidence < 90 {
		t.Fatalf("expected WSL confidence >= 90, got %d", detection.Confidence)
	}
	if len(detection.Indicators) == 0 {
		t.Fatalf("expected WSL indicators")
	}
}

func TestDetectSandboxWindowsFromEnvWSLENV(t *testing.T) {
	detection := detectSandboxWindowsFromEnv(map[string]string{"WSLENV": "PATH/l"})
	if detection.Type != "WSL" {
		t.Fatalf("expected WSL detection, got %q", detection.Type)
	}
	if detection.Confidence < 90 {
		t.Fatalf("expected WSL confidence >= 90, got %d", detection.Confidence)
	}
}

func TestDetectSandboxWindowsFromCwdWSLUNC(t *testing.T) {
	detection := detectSandboxWindowsFromCwd(`\\wsl.localhost\Ubuntu\home\user\project`)
	if detection.Type != "WSL" {
		t.Fatalf("expected WSL detection from cwd, got %q", detection.Type)
	}
	if detection.Confidence < 90 {
		t.Fatalf("expected WSL cwd confidence >= 90, got %d", detection.Confidence)
	}
}

func TestDetectSandboxWindowsFromParentWSLHost(t *testing.T) {
	detection := detectSandboxWindowsFromParent("wslhost.exe")
	if detection.Type != "WSL" {
		t.Fatalf("expected WSL detection from parent process, got %q", detection.Type)
	}
	if detection.Confidence < 90 {
		t.Fatalf("expected WSL parent confidence >= 90, got %d", detection.Confidence)
	}
}

func TestParseTasklistImageName(t *testing.T) {
	image, err := parseTasklistImageName([]byte(`"wslhost.exe","4231","Console","1","10,240 K"`))
	if err != nil {
		t.Fatalf("expected no parse error, got %v", err)
	}
	if image != "wslhost.exe" {
		t.Fatalf("expected image name wslhost.exe, got %q", image)
	}
}

func TestDetectSandboxWindowsFromEnvWindowsSandbox(t *testing.T) {
	detection := detectSandboxWindowsFromEnv(map[string]string{"USERNAME": "WDAGUtilityAccount"})
	if detection.Type != "Windows Sandbox" {
		t.Fatalf("expected Windows Sandbox detection, got %q", detection.Type)
	}
	if detection.Confidence < 95 {
		t.Fatalf("expected Windows Sandbox confidence >= 95, got %d", detection.Confidence)
	}
	if len(detection.Indicators) == 0 {
		t.Fatalf("expected Windows Sandbox indicators")
	}
}

func detectSandboxWindowsFromTasklist(tasklist string) sandboxDetection {
	typeScore := map[string]int{}
	indicators := make([]string, 0, 8)

	add := func(kind string, confidence int, indicator string) {
		if confidence <= 0 {
			return
		}
		typeScore[kind] += confidence
		indicators = append(indicators, indicator)
	}

	addWindowsProcessIndicators(tasklist, add)
	return finalizeWindowsDetection(typeScore, indicators)
}

func TestDetectSandboxWindowsFromTasklistHostWindowsSandboxProcessesNoSignal(t *testing.T) {
	detection := detectSandboxWindowsFromTasklist("WindowsSandbox.exe SandboxClient.exe")
	if detection.Type != "" || detection.Confidence != 0 {
		t.Fatalf("expected no sandbox signal from host Windows Sandbox processes, got type=%q confidence=%d", detection.Type, detection.Confidence)
	}
}

func detectSandboxWindowsFromEnv(env map[string]string) sandboxDetection {
	typeScore := map[string]int{}
	indicators := make([]string, 0, 8)

	add := func(kind string, confidence int, indicator string) {
		if confidence <= 0 {
			return
		}
		typeScore[kind] += confidence
		indicators = append(indicators, indicator)
	}

	addWindowsEnvIndicators(func(name string) (string, bool) {
		value, ok := env[name]
		return value, ok
	}, add)
	if value, ok := env["USERNAME"]; ok && value == "WDAGUtilityAccount" {
		add("Windows Sandbox", 95, "windows:env:wdag_utility_account")
	}
	if value, ok := env["USERPROFILE"]; ok && value != "" {
		profile := strings.ToLower(value)
		if strings.Contains(profile, `\users\wdagutilityaccount`) {
			add("Windows Sandbox", 95, "windows:env:userprofile_wdag")
		}
	}

	return finalizeWindowsDetection(typeScore, indicators)
}

func detectSandboxWindowsFromCwd(cwd string) sandboxDetection {
	typeScore := map[string]int{}
	indicators := make([]string, 0, 8)

	add := func(kind string, confidence int, indicator string) {
		if confidence <= 0 {
			return
		}
		typeScore[kind] += confidence
		indicators = append(indicators, indicator)
	}

	addWindowsWSLCwdIndicators(cwd, add)
	return finalizeWindowsDetection(typeScore, indicators)
}

func detectSandboxWindowsFromParent(parentName string) sandboxDetection {
	typeScore := map[string]int{}
	indicators := make([]string, 0, 8)

	add := func(kind string, confidence int, indicator string) {
		if confidence <= 0 {
			return
		}
		typeScore[kind] += confidence
		indicators = append(indicators, indicator)
	}

	addWindowsWSLParentIndicators(parentName, add)
	return finalizeWindowsDetection(typeScore, indicators)
}
