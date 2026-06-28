package security

import "testing"

func TestRunAntiTamperProbeDisabled(t *testing.T) {
	t.Setenv(AntiTamperProbeEnableEnv, "0")
	ResetSecurityTelemetry()

	signals, enabled, err := RunAntiTamperProbe([]string{"cpuid_hypervisor"}, "test-disabled")
	if err != nil {
		t.Fatalf("expected nil error when anti-tamper probe disabled, got: %v", err)
	}
	if enabled {
		t.Fatalf("expected enabled=false")
	}
	if len(signals) != 0 {
		t.Fatalf("expected no signals, got %d", len(signals))
	}

	snapshot := SecurityTelemetrySnapshot()
	if snapshot["anti_tamper_probe_invoked"] != 0 || snapshot["anti_tamper_probe_error"] != 0 {
		t.Fatalf("expected no telemetry changes when disabled, got %+v", snapshot)
	}
}

func TestRunAntiTamperProbeEnabled(t *testing.T) {
	t.Setenv(AntiTamperProbeEnableEnv, "1")
	ResetSecurityTelemetry()

	signals, enabled, err := RunAntiTamperProbe([]string{"cpuid_hypervisor"}, "test-enabled")
	if !enabled {
		t.Fatalf("expected enabled=true")
	}
	if err != nil {
		t.Fatalf("expected nil error with native go probe engine, got: %v", err)
	}
	if len(signals) != 1 {
		t.Fatalf("expected one signal, got %d", len(signals))
	}
	if signals[0].Name != "cpuid_hypervisor" {
		t.Fatalf("expected cpuid_hypervisor signal, got %q", signals[0].Name)
	}

	snapshot := SecurityTelemetrySnapshot()
	if snapshot["anti_tamper_probe_invoked"] != 1 {
		t.Fatalf("expected anti_tamper_probe_invoked=1, got %d", snapshot["anti_tamper_probe_invoked"])
	}
	if snapshot["anti_tamper_probe_error"] != 0 {
		t.Fatalf("expected anti_tamper_probe_error=0, got %d", snapshot["anti_tamper_probe_error"])
	}
}
