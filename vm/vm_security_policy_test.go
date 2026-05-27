package vm

import (
	"testing"
	"time"

	"mutant/compiler"
	"mutant/security"
)

func TestVMIntegrityTamperResponseModes(t *testing.T) {
	t.Run("warn", func(t *testing.T) {
		security.ResetSecurityTelemetry()
		t.Setenv(security.TamperResponseEnv, security.TamperResponseWarn)
		vm := tamperedVMForPolicyTest()

		err := vm.verifyCurrentFrameIntegrity()
		if err != nil {
			t.Fatalf("expected warn mode to continue, got: %v", err)
		}

		snapshot := security.SecurityTelemetrySnapshot()
		if snapshot["integrity_failed"] == 0 {
			t.Fatalf("expected integrity failure telemetry increment in warn mode")
		}
	})

	t.Run("delay", func(t *testing.T) {
		security.ResetSecurityTelemetry()
		t.Setenv(security.TamperResponseEnv, security.TamperResponseDelay)
		t.Setenv(security.TamperDelayMsEnv, "1")
		vm := tamperedVMForPolicyTest()

		start := time.Now()
		err := vm.verifyCurrentFrameIntegrity()
		elapsed := time.Since(start)
		if err != nil {
			t.Fatalf("expected delay mode to continue, got: %v", err)
		}
		if elapsed <= 0 {
			t.Fatalf("expected measurable delay in delay mode")
		}

		snapshot := security.SecurityTelemetrySnapshot()
		if snapshot["integrity_failed"] == 0 {
			t.Fatalf("expected integrity failure telemetry increment in delay mode")
		}
	})

	t.Run("terminate", func(t *testing.T) {
		security.ResetSecurityTelemetry()
		t.Setenv(security.TamperResponseEnv, security.TamperResponseTerminate)
		vm := tamperedVMForPolicyTest()

		err := vm.verifyCurrentFrameIntegrity()
		if err == nil {
			t.Fatalf("expected terminate mode to fail on integrity tamper")
		}

		snapshot := security.SecurityTelemetrySnapshot()
		if snapshot["integrity_failed"] == 0 {
			t.Fatalf("expected integrity failure telemetry increment in terminate mode")
		}
	})
}

func tamperedVMForPolicyTest() *VM {
	bc := &compiler.ByteCode{Instructions: []byte{0x42}}
	vm := New(bc)
	frame := vm.currentFrame()
	frame.cl.Fn.Instructions[0] ^= 0xFF
	return vm
}
