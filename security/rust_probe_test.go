package security

import "testing"

func TestRunRustProbeDisabled(t *testing.T) {
	t.Setenv(RustProbeEnableEnv, "0")
	ResetSecurityTelemetry()

	signals, enabled, err := RunRustProbe([]string{"cpuid_hypervisor"}, "test-disabled")
	if err != nil {
		t.Fatalf("expected nil error when rust probe disabled, got: %v", err)
	}
	if enabled {
		t.Fatalf("expected enabled=false")
	}
	if len(signals) != 0 {
		t.Fatalf("expected no signals, got %d", len(signals))
	}

	snapshot := SecurityTelemetrySnapshot()
	if snapshot["rust_probe_invoked"] != 0 || snapshot["rust_probe_error"] != 0 {
		t.Fatalf("expected no rust telemetry changes when disabled, got %+v", snapshot)
	}
}

func TestRunRustProbeEnabledUnavailable(t *testing.T) {
	t.Setenv(RustProbeEnableEnv, "1")
	ResetSecurityTelemetry()

	_, enabled, err := RunRustProbe([]string{"cpuid_hypervisor"}, "test-enabled")
	if !enabled {
		t.Fatalf("expected enabled=true")
	}
	if err == nil {
		t.Fatalf("expected error while provider is unavailable")
	}

	snapshot := SecurityTelemetrySnapshot()
	if snapshot["rust_probe_invoked"] != 1 {
		t.Fatalf("expected rust_probe_invoked=1, got %d", snapshot["rust_probe_invoked"])
	}
	if snapshot["rust_probe_error"] != 1 {
		t.Fatalf("expected rust_probe_error=1, got %d", snapshot["rust_probe_error"])
	}
}
