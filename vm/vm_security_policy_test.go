package vm

import (
	"strings"
	"testing"
	"time"

	"mutant/code"
	"mutant/compiler"
	"mutant/mutil"
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

func TestValidateSecurityCheckOpcodesPresent(t *testing.T) {
	ins := append(code.Make(code.OpChkDbg), code.Make(code.OpChkSnd)...)
	ins = append(ins, code.Make(code.OpNull)...)

	bc := &compiler.ByteCode{Instructions: ins}
	encrypted := mutil.EncryptByteCode(bc, "testpwd")
	vm := NewWithPassword(encrypted, "testpwd")

	if err := vm.validateSecurityCheckOpcodes("before-execution"); err != nil {
		t.Fatalf("expected required security opcodes to be present: %v", err)
	}
}

func TestValidateSecurityCheckOpcodesMissing(t *testing.T) {
	bc := &compiler.ByteCode{Instructions: code.Make(code.OpNull)}
	encrypted := mutil.EncryptByteCode(bc, "testpwd")
	vm := NewWithPassword(encrypted, "testpwd")

	err := vm.validateSecurityCheckOpcodes("before-execution")
	if err == nil {
		t.Fatalf("expected missing security opcode validation error")
	}

	if !strings.Contains(err.Error(), "required security check opcodes missing") {
		t.Fatalf("unexpected validation error: %v", err)
	}
}
