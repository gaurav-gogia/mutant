package security

import (
	"strings"
	"testing"
)

// M26-TMP-014. Four probes carried the names of measurements they never made.
// hardware_breakpoint and syscall each re-ran the debugger detector and
// reported its verdict -- a parent process named gdb, say -- under their own
// names; cpuid_hypervisor reported the sandbox detector's verdict, and denied a
// hypervisor on a host whose CPUID reports one; and rdtsc_drift timed three
// sleeps on the OS clock and made no RDTSC check. The first three are not
// probes now -- debug_status and sandbox_status report those detectors in their
// own fields -- and the fourth is called sleep_drift, after what it times.
func TestAntiTamperProbeTMP014RetiredAndRenamed(t *testing.T) {
	supported := map[string]bool{}
	for _, name := range AntiTamperSupportedProbes {
		supported[name] = true
	}
	for _, retired := range []string{"hardware_breakpoint", "syscall", "cpuid_hypervisor", "rdtsc_drift"} {
		if supported[retired] {
			t.Errorf("%s is still listed as a supported probe", retired)
		}
		if signal := probeOne(retired); signal.Detail != AntiTamperDetailUnknownProbe || signal.Measured || signal.Detected {
			t.Errorf("%s is still routed to a measurement: %+v", retired, signal)
		}
	}

	if !supported["sleep_drift"] {
		t.Errorf("sleep_drift is not listed as a supported probe")
	}
	signal := probeOne("sleep_drift")
	if signal.Name != "sleep_drift" || !signal.Measured || !strings.HasPrefix(signal.Detail, "sleep_ms=") {
		t.Errorf("sleep_drift does not report the sleeps it timed: %+v", signal)
	}
}
