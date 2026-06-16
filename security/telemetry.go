package security

import (
	"encoding/json"
	"fmt"
	"os"
	"sync/atomic"
	"time"
)

const SecurityAuditEnv = "MUTANT_SECURITY_AUDIT"
const SecurityTelemetryFileEnv = "MUTANT_SECURITY_TELEMETRY_FILE"

var (
	telemetryDebuggerDetected uint64
	telemetryIntegrityFailed  uint64
	telemetrySignatureFailed  uint64
	telemetrySandboxDetected  uint64
	telemetryRustProbeInvoked uint64
	telemetryRustProbeError   uint64
)

func RecordDebuggerDetected(stage string) {
	atomic.AddUint64(&telemetryDebuggerDetected, 1)
	auditEvent("debugger_detected", stage)
}

func RecordIntegrityFailure(stage string) {
	atomic.AddUint64(&telemetryIntegrityFailed, 1)
	auditEvent("integrity_failed", stage)
}

func RecordSignatureFailure(stage string) {
	atomic.AddUint64(&telemetrySignatureFailed, 1)
	auditEvent("signature_failed", stage)
}

func RecordSandboxDetected(stage string) {
	atomic.AddUint64(&telemetrySandboxDetected, 1)
	auditEvent("sandbox_detected", stage)
}

func RecordRustProbeInvoked(stage string) {
	atomic.AddUint64(&telemetryRustProbeInvoked, 1)
	auditEvent("rust_probe_invoked", stage)
}

func RecordRustProbeError(stage string) {
	atomic.AddUint64(&telemetryRustProbeError, 1)
	auditEvent("rust_probe_error", stage)
}

func SecurityTelemetrySnapshot() map[string]uint64 {
	return map[string]uint64{
		"debugger_detected":  atomic.LoadUint64(&telemetryDebuggerDetected),
		"integrity_failed":   atomic.LoadUint64(&telemetryIntegrityFailed),
		"signature_failed":   atomic.LoadUint64(&telemetrySignatureFailed),
		"sandbox_detected":   atomic.LoadUint64(&telemetrySandboxDetected),
		"rust_probe_invoked": atomic.LoadUint64(&telemetryRustProbeInvoked),
		"rust_probe_error":   atomic.LoadUint64(&telemetryRustProbeError),
	}
}

func SecurityTelemetryJSON() ([]byte, error) {
	snapshot := SecurityTelemetrySnapshot()
	return json.Marshal(snapshot)
}

func ExportSecurityTelemetry(path string) error {
	if path == "" {
		return nil
	}

	data, err := SecurityTelemetryJSON()
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0600)
}

func ResetSecurityTelemetry() {
	atomic.StoreUint64(&telemetryDebuggerDetected, 0)
	atomic.StoreUint64(&telemetryIntegrityFailed, 0)
	atomic.StoreUint64(&telemetrySignatureFailed, 0)
	atomic.StoreUint64(&telemetrySandboxDetected, 0)
	atomic.StoreUint64(&telemetryRustProbeInvoked, 0)
	atomic.StoreUint64(&telemetryRustProbeError, 0)
}

func auditEvent(event, stage string) {
	if os.Getenv(SecurityAuditEnv) != "1" {
		return
	}

	fmt.Fprintf(os.Stderr, "[security-audit] ts=%d event=%s stage=%s\n", time.Now().Unix(), event, stage)
}
