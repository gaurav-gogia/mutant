package security

import (
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const AntiTamperProbeEnableEnv = "MUTANT_ENABLE_ANTITAMPER_PROBE"

type AntiTamperSignal struct {
	Name       string
	Detected   bool
	Confidence int
	Detail     string
}

func RunAntiTamperProbe(requested []string, stage string) ([]AntiTamperSignal, bool, error) {
	if strings.TrimSpace(stage) == "" {
		stage = "unknown"
	}

	if !isAntiTamperProbeEnabled() {
		return nil, false, nil
	}

	RecordProbeInvoked(stage)

	signals := runNativeProbe(requested)
	return signals, true, nil
}

func isAntiTamperProbeEnabled() bool {
	return strings.TrimSpace(strings.ToLower(os.Getenv(AntiTamperProbeEnableEnv))) == "1"
}

func runNativeProbe(requested []string) []AntiTamperSignal {
	if len(requested) == 0 {
		return nil
	}

	out := make([]AntiTamperSignal, 0, len(requested))
	for _, name := range requested {
		out = append(out, probeOne(strings.TrimSpace(name)))
	}
	return out
}

func probeOne(name string) AntiTamperSignal {
	switch name {
	case "hardware_breakpoint":
		return detectHardwareBreakpoint()
	case "timing":
		return detectTiming()
	case "syscall":
		return detectSyscall()
	case "frida_ptrace":
		return detectFridaPtrace()
	case "ld_preload":
		return detectLDPreload()
	case "cpuid_hypervisor":
		return detectCPUIDHypervisor()
	case "rdtsc_drift":
		return detectRDTSCDrift()
	case "acpi_pci":
		return makeSignal(name, false, 0, "not implemented yet")
	case "gpu_feature":
		return makeSignal(name, false, 0, "not implemented yet")
	case "iat_got":
		return makeSignal(name, false, 0, "not implemented yet")
	case "syscall_table":
		return makeSignal(name, false, 0, "not implemented yet")
	case "trampoline":
		return makeSignal(name, false, 0, "not implemented yet")
	case "":
		return makeSignal("", false, 0, "unknown probe")
	default:
		return makeSignal(name, false, 0, "unknown probe")
	}
}

func makeSignal(name string, detected bool, confidence int, detail string) AntiTamperSignal {
	return AntiTamperSignal{
		Name:       name,
		Detected:   detected,
		Confidence: confidence,
		Detail:     detail,
	}
}

func detectHardwareBreakpoint() AntiTamperSignal {
	detected, methods := DetectDebuggerDetails()
	if !detected {
		return makeSignal("hardware_breakpoint", false, 0, "no debugger method detected")
	}

	if len(methods) == 0 {
		return makeSignal("hardware_breakpoint", true, 65, "debugger detected without named method")
	}

	return makeSignal(
		"hardware_breakpoint",
		true,
		70,
		"debugger_methods="+strings.Join(methods, ","),
	)
}

func detectTiming() AntiTamperSignal {
	start := time.Now()
	var acc uint64
	for i := uint64(0); i < 200000; i++ {
		acc ^= i * 0x9E3779B9
	}
	elapsedUs := time.Since(start).Microseconds()
	suspicious := elapsedUs > 200000
	confidence := 5
	if suspicious {
		confidence = 40
	}

	return makeSignal(
		"timing",
		suspicious,
		confidence,
		"loop_us="+strconv.FormatInt(elapsedUs, 10)+";acc="+strconv.FormatUint(acc, 10),
	)
}

func detectSyscall() AntiTamperSignal {
	detected, methods := DetectDebuggerDetails()
	if !detected {
		return makeSignal("syscall", false, 0, "no debugger API signal detected")
	}

	detail := "debugger signal detected"
	if len(methods) > 0 {
		detail = "api_hits=" + strings.Join(methods, ",")
	}

	return makeSignal("syscall", true, 80, detail)
}

func detectFridaPtrace() AntiTamperSignal {
	fridaMarkers := []string{"FRIDA", "FRIDA_AGENT", "FRIDA_GADGET"}
	for _, marker := range fridaMarkers {
		if _, ok := os.LookupEnv(marker); ok {
			return makeSignal("frida_ptrace", true, 90, "env marker present: "+marker)
		}
	}

	if runtime.GOOS == "linux" {
		if tracer, ok := readLinuxTracerPID(); ok && tracer > 0 {
			return makeSignal("frida_ptrace", true, 75, "ptrace tracer pid: "+strconv.Itoa(tracer))
		}
	}

	if runtime.GOOS == "windows" {
		out, err := exec.Command("tasklist").Output()
		if err == nil {
			tasks := strings.ToLower(string(out))
			for _, marker := range []string{"frida", "frida-helper", "frida-server", "frida-agent"} {
				if strings.Contains(tasks, marker) {
					return makeSignal("frida_ptrace", true, 85, "tasklist marker: "+marker)
				}
			}
		}
	}

	return makeSignal("frida_ptrace", false, 0, "no frida/ptrace heuristic triggered")
}

func detectLDPreload() AntiTamperSignal {
	if runtime.GOOS == "windows" {
		markers := []string{"COR_ENABLE_PROFILING", "COR_PROFILER", "COR_PROFILER_PATH", "__COMPAT_LAYER"}
		present := make([]string, 0, len(markers))
		for _, name := range markers {
			if value, ok := os.LookupEnv(name); ok && strings.TrimSpace(value) != "" {
				present = append(present, name+"="+value)
			}
		}

		if len(present) == 0 {
			return makeSignal("ld_preload", false, 0, "no windows injection env markers")
		}

		return makeSignal("ld_preload", true, 55, "windows_env_markers="+strings.Join(present, ";"))
	}

	preload := strings.TrimSpace(os.Getenv("LD_PRELOAD"))
	if preload == "" {
		return makeSignal("ld_preload", false, 0, "LD_PRELOAD empty")
	}

	return makeSignal("ld_preload", true, 85, "LD_PRELOAD="+preload)
}

func detectCPUIDHypervisor() AntiTamperSignal {
	sandboxType, confidence, err := DetectSandboxType()
	if err == nil && sandboxType != "none" && confidence >= 70 {
		return makeSignal("cpuid_hypervisor", true, 70, "sandbox_type="+sandboxType)
	}

	return makeSignal("cpuid_hypervisor", false, 0, "no hypervisor signal")
}

func detectRDTSCDrift() AntiTamperSignal {
	start := time.Now()
	for i := 0; i < 3; i++ {
		time.Sleep(time.Millisecond)
	}
	elapsedMs := int(time.Since(start).Milliseconds())
	drift := elapsedMs - 3
	if drift < 0 {
		drift = -drift
	}

	suspicious := drift > 10
	confidence := 0
	if suspicious {
		confidence = 35
	}

	return makeSignal(
		"rdtsc_drift",
		suspicious,
		confidence,
		"sleep_ms="+strconv.Itoa(elapsedMs)+";drift_ms="+strconv.Itoa(drift),
	)
}

func detectACPIPCI() AntiTamperSignal {
	sandboxType, confidence, err := DetectSandboxType()
	if err != nil {
		return makeSignal("acpi_pci", false, 0, "sandbox detect error: "+err.Error())
	}

	indicators, indicatorsErr := GetSandboxIndicators()
	if indicatorsErr != nil {
		return makeSignal("acpi_pci", false, 0, "sandbox indicators error: "+indicatorsErr.Error())
	}

	detail := "no sandbox indicators"
	if len(indicators) > 0 {
		detail = "indicators=" + strings.Join(indicators, ";")
	}

	if sandboxType != "none" && confidence > 0 {
		score := confidence
		if score > 60 {
			score = 60
		}
		if score < 1 {
			score = 1
		}
		return makeSignal("acpi_pci", true, score, "sandbox_type="+sandboxType+";"+detail)
	}

	return makeSignal("acpi_pci", false, 0, detail)
}

func readLinuxTracerPID() (int, bool) {
	if runtime.GOOS != "linux" {
		return 0, false
	}

	status, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return 0, false
	}

	for _, line := range strings.Split(string(status), "\n") {
		if !strings.HasPrefix(line, "TracerPid:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return 0, false
		}
		pid, parseErr := strconv.Atoi(fields[1])
		if parseErr != nil {
			return 0, false
		}
		return pid, true
	}

	return 0, false
}
