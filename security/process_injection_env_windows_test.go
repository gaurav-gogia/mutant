//go:build windows

package security

import (
	"strings"
	"testing"
)

// TestProcessInjectionIgnoresConfigurationVariables is M26-TMP-004. Any one of
// these five set made process_injection answer 90, and secure mode stopped
// with "injection or hooking activity in this process". They configure the
// .NET runtime, a Java VM and symbol handlers such as dbghelp's, none of which
// runs in this process, and the values are the kind an ordinary workstation
// holds. All five are set at once, so the test fails if any of them is read
// again. The probe may still find a tool in this host's process list, which
// the test does not control, but never anything in the environment.
func TestProcessInjectionIgnoresConfigurationVariables(t *testing.T) {
	variables := []struct{ name, value string }{
		{"_NT_SYMBOL_PATH", `srv*C:\symbols*https://msdl.microsoft.com/download/symbols`},
		{"JAVA_TOOL_OPTIONS", "-Dfile.encoding=UTF-8"},
		{"COR_ENABLE_PROFILING", "1"},
		{"COR_PROFILER", "{36032161-FFC0-4B61-B559-F6C5D41BAE5A}"},
		{"COR_PROFILER_PATH", `C:\agent\profiler.dll`},
	}
	for _, v := range variables {
		t.Setenv(v.name, v.value)
	}

	signal := detectProcessInjection()
	if signal.Name != ProbeProcessInjection {
		t.Fatalf("signal name %q, want %q", signal.Name, ProbeProcessInjection)
	}
	for _, v := range variables {
		if strings.Contains(signal.Detail, v.name) {
			t.Errorf("process_injection reported %s: detected=%t confidence=%d detail=%q",
				v.name, signal.Detected, signal.Confidence, signal.Detail)
		}
	}
	if signal.Detected && (signal.Confidence != ConfidenceProcessInjectionDetected || !strings.HasPrefix(signal.Detail, "processes=")) {
		t.Errorf("a detection here can only be a tool in the process list, at confidence %d; got confidence=%d detail=%q",
			ConfidenceProcessInjectionDetected, signal.Confidence, signal.Detail)
	}
	if !signal.Detected && signal.Confidence != ConfidenceNone {
		t.Errorf("not detected, yet confidence %d", signal.Confidence)
	}
}
