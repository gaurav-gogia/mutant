package builtin

import (
	"testing"

	"mutant/object"
	"mutant/security"
)

func TestDebugStatusBuiltin(t *testing.T) {
	security.ResetSecurityTelemetry()

	result := DebugStatus()
	hash, ok := result.(*object.Hash)
	if !ok {
		t.Fatalf("debug_status() result is not Hash. got=%T", result)
	}

	assertHashHasKeyType(t, hash, "detected", object.BOOLEAN_OBJ)
	assertHashHasKeyType(t, hash, "type", object.STRING_OBJ)
	assertHashHasKeyType(t, hash, "confidence", object.INTEGER_OBJ)
	assertHashHasKeyType(t, hash, "indicators", object.ARRAY_OBJ)
	assertHashHasKeyType(t, hash, "rust_signals", object.ARRAY_OBJ)
	assertHashHasKeyType(t, hash, "rust_enabled", object.BOOLEAN_OBJ)
	assertHashHasKeyType(t, hash, "rust_error", object.STRING_OBJ)
	assertHashHasKeyType(t, hash, "source", object.STRING_OBJ)
	assertHashHasKeyType(t, hash, "advisory", object.BOOLEAN_OBJ)
	assertHashHasKeyType(t, hash, "event_count", object.INTEGER_OBJ)
	assertHashHasKeyType(t, hash, "error", object.STRING_OBJ)
	assertHashHasKeyType(t, hash, "schema_version", object.INTEGER_OBJ)
}

func TestSandboxStatusBuiltin(t *testing.T) {
	security.ResetSecurityTelemetry()

	result := SandboxStatus()
	hash, ok := result.(*object.Hash)
	if !ok {
		t.Fatalf("sandbox_status() result is not Hash. got=%T", result)
	}

	assertHashHasKeyType(t, hash, "detected", object.BOOLEAN_OBJ)
	assertHashHasKeyType(t, hash, "type", object.STRING_OBJ)
	assertHashHasKeyType(t, hash, "confidence", object.INTEGER_OBJ)
	assertHashHasKeyType(t, hash, "indicators", object.ARRAY_OBJ)
	assertHashHasKeyType(t, hash, "rust_signals", object.ARRAY_OBJ)
	assertHashHasKeyType(t, hash, "rust_enabled", object.BOOLEAN_OBJ)
	assertHashHasKeyType(t, hash, "rust_error", object.STRING_OBJ)
	assertHashHasKeyType(t, hash, "source", object.STRING_OBJ)
	assertHashHasKeyType(t, hash, "advisory", object.BOOLEAN_OBJ)
	assertHashHasKeyType(t, hash, "event_count", object.INTEGER_OBJ)
	assertHashHasKeyType(t, hash, "error", object.STRING_OBJ)
	assertHashHasKeyType(t, hash, "schema_version", object.INTEGER_OBJ)
}

func TestSecurityDiagnosticsBuiltin(t *testing.T) {
	security.ResetSecurityTelemetry()

	result := SecurityDiagnostics()
	hash, ok := result.(*object.Hash)
	if !ok {
		t.Fatalf("security_diagnostics() result is not Hash. got=%T", result)
	}

	assertHashHasKeyType(t, hash, "debugger", object.HASH_OBJ)
	assertHashHasKeyType(t, hash, "sandbox", object.HASH_OBJ)
	assertHashHasKeyType(t, hash, "source", object.STRING_OBJ)
	assertHashHasKeyType(t, hash, "schema_version", object.INTEGER_OBJ)

	debuggerHash := getHashField(t, hash, "debugger")
	assertHashHasKeyType(t, debuggerHash, "detected", object.BOOLEAN_OBJ)
	assertHashHasKeyType(t, debuggerHash, "methods", object.ARRAY_OBJ)
	assertHashHasKeyType(t, debuggerHash, "event_count", object.INTEGER_OBJ)
	assertHashHasKeyType(t, debuggerHash, "schema_version", object.INTEGER_OBJ)

	sandboxHash := getHashField(t, hash, "sandbox")
	assertHashHasKeyType(t, sandboxHash, "detected", object.BOOLEAN_OBJ)
	assertHashHasKeyType(t, sandboxHash, "type", object.STRING_OBJ)
	assertHashHasKeyType(t, sandboxHash, "confidence", object.INTEGER_OBJ)
	assertHashHasKeyType(t, sandboxHash, "indicators", object.ARRAY_OBJ)
	assertHashHasKeyType(t, sandboxHash, "event_count", object.INTEGER_OBJ)
	assertHashHasKeyType(t, sandboxHash, "error", object.STRING_OBJ)
	assertHashHasKeyType(t, sandboxHash, "schema_version", object.INTEGER_OBJ)
}

func TestSecurityStatusBuiltinWrongArgs(t *testing.T) {
	debugErr := DebugStatus(&object.Integer{Value: 1})
	if _, ok := debugErr.(*object.Error); !ok {
		t.Fatalf("expected error for debug_status wrong args, got=%T", debugErr)
	}

	sandboxErr := SandboxStatus(&object.Integer{Value: 1})
	if _, ok := sandboxErr.(*object.Error); !ok {
		t.Fatalf("expected error for sandbox_status wrong args, got=%T", sandboxErr)
	}

	diagnosticsErr := SecurityDiagnostics(&object.Integer{Value: 1})
	if _, ok := diagnosticsErr.(*object.Error); !ok {
		t.Fatalf("expected error for security_diagnostics wrong args, got=%T", diagnosticsErr)
	}
}

func getHashField(t *testing.T, h *object.Hash, key string) *object.Hash {
	t.Helper()

	keyObj := &object.String{Value: key}
	pair, ok := h.Pairs[keyObj.HashKey()]
	if !ok {
		t.Fatalf("missing key %q", key)
	}

	value, ok := pair.Value.(*object.Hash)
	if !ok {
		t.Fatalf("key %q is not Hash. got=%T", key, pair.Value)
	}

	return value
}

func assertHashHasKeyType(t *testing.T, h *object.Hash, key string, expected object.ObjectType) {
	t.Helper()

	keyObj := &object.String{Value: key}
	pair, ok := h.Pairs[keyObj.HashKey()]
	if !ok {
		t.Fatalf("missing key %q", key)
	}

	if pair.Value.Type() != expected {
		t.Fatalf("wrong value type for key %q. got=%s, want=%s", key, pair.Value.Type(), expected)
	}
}
