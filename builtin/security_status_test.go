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

func TestSecurityStatusBuiltinWrongArgs(t *testing.T) {
	debugErr := DebugStatus(&object.Integer{Value: 1})
	if _, ok := debugErr.(*object.Error); !ok {
		t.Fatalf("expected error for debug_status wrong args, got=%T", debugErr)
	}

	sandboxErr := SandboxStatus(&object.Integer{Value: 1})
	if _, ok := sandboxErr.(*object.Error); !ok {
		t.Fatalf("expected error for sandbox_status wrong args, got=%T", sandboxErr)
	}
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
