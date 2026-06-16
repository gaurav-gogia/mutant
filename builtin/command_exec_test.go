package builtin

import (
	"testing"

	"mutant/object"
	"mutant/security"
)

func TestExecStringBuiltinDisabledByDefault(t *testing.T) {
	security.ResetSecurityTelemetry()

	result := ExecString(&object.String{Value: "Write-Output 'mutant'"})
	hash, ok := result.(*object.Hash)
	if !ok {
		t.Fatalf("exec_string() result is not Hash. got=%T", result)
	}

	assertHashHasKeyType(t, hash, "ok", object.BOOLEAN_OBJ)
	assertHashHasKeyType(t, hash, "allowed", object.BOOLEAN_OBJ)
	assertHashHasKeyType(t, hash, "policy_decision", object.STRING_OBJ)
	assertHashHasKeyType(t, hash, "exit_code", object.INTEGER_OBJ)
	assertHashHasKeyType(t, hash, "stdout", object.STRING_OBJ)
	assertHashHasKeyType(t, hash, "stderr", object.STRING_OBJ)
	assertHashHasKeyType(t, hash, "timed_out", object.BOOLEAN_OBJ)
	assertHashHasKeyType(t, hash, "error", object.STRING_OBJ)
	assertHashHasKeyType(t, hash, "schema_version", object.INTEGER_OBJ)

	decisionObj, ok := hashValueByKey(hash, "policy_decision").(*object.String)
	if !ok {
		t.Fatalf("policy_decision is not String")
	}
	if decisionObj.Value != "blocked_missing_capability" {
		t.Fatalf("unexpected policy decision. got=%q, want=%q", decisionObj.Value, "blocked_missing_capability")
	}
}

func TestCommandBuilderRoundTrip(t *testing.T) {
	t.Setenv(BuiltinCapabilitiesEnv, CapabilityCommandExec)
	builder := CmdBuilder(&object.String{Value: "powershell"})
	builder = CmdAdd(builder, &object.String{Value: "$x='a'"})
	builder = CmdAdd(builder, &object.String{Value: "Write-Output $x"})

	hash, ok := builder.(*object.Hash)
	if !ok {
		t.Fatalf("builder is not Hash. got=%T", builder)
	}

	linesObj := hashValueByKey(hash, "lines")
	lines, ok := linesObj.(*object.Array)
	if !ok {
		t.Fatalf("lines is not Array. got=%T", linesObj)
	}
	if len(lines.Elements) != 2 {
		t.Fatalf("wrong line count. got=%d, want=2", len(lines.Elements))
	}
}

func TestCmdRunEmptyBuilderErrors(t *testing.T) {
	t.Setenv(BuiltinCapabilitiesEnv, CapabilityCommandExec)
	builder := CmdBuilder()
	result := CmdRun(builder)
	if _, ok := result.(*object.Error); !ok {
		t.Fatalf("expected Error, got=%T", result)
	}
}

func TestExecStringBlockedWhenCapabilityGrantedButExecutionDisabled(t *testing.T) {
	t.Setenv(BuiltinCapabilitiesEnv, CapabilityCommandExec)
	t.Setenv(security.CommandExecEnabledEnv, "")

	result := ExecString(&object.String{Value: "Write-Output 'mutant'"})
	hash, ok := result.(*object.Hash)
	if !ok {
		t.Fatalf("exec_string() result is not Hash. got=%T", result)
	}

	decisionObj, ok := hashValueByKey(hash, "policy_decision").(*object.String)
	if !ok {
		t.Fatalf("policy_decision is not String")
	}
	if decisionObj.Value != "blocked_disabled" {
		t.Fatalf("unexpected policy decision. got=%q, want=%q", decisionObj.Value, "blocked_disabled")
	}
}
