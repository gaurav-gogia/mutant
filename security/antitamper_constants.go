package security

const (
	ProbeTiming            = "timing"
	ProbeFridaPtrace       = "frida_ptrace"
	ProbeSleepDrift        = "sleep_drift"
	ProbeIATGOT            = "iat_got"
	ProbeSyscallTable      = "syscall_table"
	ProbeTrampoline        = "trampoline"
	ProbeProcessInjection  = "process_injection"
	ProbeModuleIntegrity   = "module_integrity"
	ProbeMemoryPageAnomaly = "memory_page_anomaly"

	AntiTamperUnknownStage = "unknown"

	AntiTamperDetailUnknownProbe = "unknown probe"

	ConfidenceNone                      = 0
	ConfidenceTimingBaseline            = 5
	ConfidenceSleepDriftSuspicious      = 35
	ConfidenceTimingSuspicious          = 40
	ConfidenceProcessInjectionDetected  = 70
	ConfidenceTrampolineDetected        = 75
	ConfidenceFridaPtraceTracerDetected = 75
	ConfidenceSyscallTableDetected      = 82
	ConfidenceFridaTasklistDetected     = 85
	ConfidenceModuleIntegrityDetected   = 85
	ConfidenceIATGOTDetected            = 90
	ConfidenceFridaEnvMarkerDetected    = 90
	ConfidenceTrampolineMultiHit        = 90
	ConfidenceMemoryPageAnomalyDetected = 92

	// TimingLoopIterations is the length of the timing probe's loop: enough
	// work to be timed, little enough to cost nothing.
	//
	//mutant:limit iterations
	TimingLoopIterations  uint64 = 200000
	TimingLoopXORConstant uint64 = 0x9E3779B9
	// TimingSuspiciousThresholdUs is how long the loop may take before the
	// timing probe reports it. The loop runs in well under a millisecond on an
	// unhindered machine -- the Windows clock reads it as 0 or 1 ms -- so two
	// hundred is a thread stopped part way through, single-stepped or
	// suspended, and not a slow one.
	//
	//mutant:limit microseconds
	TimingSuspiciousThresholdUs = 200000

	// SleepDriftNaps is how many sleepDriftNap naps the sleep_drift probe
	// times.
	//
	//mutant:limit iterations
	SleepDriftNaps = 3
	// SleepDriftThresholdMs is how far the naps' total may run past what they
	// asked for before the probe reports it. Three 1 ms naps take four to six
	// milliseconds on an idle Windows machine; ten more than asked for is a
	// thread that was stopped, not one that was scheduled late.
	//
	//mutant:limit milliseconds
	SleepDriftThresholdMs = 10

	LinuxProcSelfStatusPath = "/proc/self/status"
)

var (
	fridaEnvMarkers  = []string{"FRIDA", "FRIDA_AGENT", "FRIDA_GADGET"}
	fridaTaskMarkers = []string{"frida", "frida-helper", "frida-server", "frida-agent"}

	// AntiTamperSupportedProbes are the names probeOne routes to a probe; any
	// other name is answered with the unknown probe signal. Six names left the
	// list in 2.6.0 and are unknown probes now. acpi_pci and cpuid_hypervisor
	// re-reported the sandbox detector, which sandbox_status reports in its own
	// fields, and read no ACPI table, PCI device or CPUID of their own.
	// hardware_breakpoint and syscall re-reported the debugger detector, whose
	// checks are debug_status's indicators, and made no check of their own:
	// neither read a debug register (M26-TMP-014). gpu_feature was never
	// implemented. And ld_preload could not have seen anything, because a
	// statically linked build (CGO_ENABLED=0, as every release is) never runs
	// the dynamic loader LD_PRELOAD works through. rdtsc_drift is called
	// sleep_drift, after what it times -- short sleeps, on the OS clock --
	// because it made no RDTSC check. docs/ANTITAMPER_PROBE.md says the same.
	AntiTamperSupportedProbes = []string{
		ProbeTiming,
		ProbeFridaPtrace,
		ProbeSleepDrift,
		ProbeIATGOT,
		ProbeSyscallTable,
		ProbeTrampoline,
		ProbeProcessInjection,
		ProbeModuleIntegrity,
		ProbeMemoryPageAnomaly,
	}
)
