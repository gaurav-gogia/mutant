package mutil

import (
	"testing"

	"mutant/object"
)

// Storage rebuilds a closure on every store and load. TestEveryObjectFieldSurvivesStorage
// compares with reflect.DeepEqual and cannot tell a copy that is the same
// function value from one that is merely built the same way, so the identity is
// held to its answer here (M26-EVL-005).
func TestClosureIdentitySurvivesStorageEVL005(t *testing.T) {
	const password, length = "closure-identity-test", 1234
	fn := &object.CompiledFunction{}
	original := object.NewClosure(fn, []object.Object{&object.Integer{Value: 5}})
	twin := object.NewClosure(fn, []object.Object{&object.Integer{Value: 5}})

	stored, err := EncryptObject(original, length, password)
	if err != nil {
		t.Fatalf("EncryptObject: %v", err)
	}
	loaded, err := DecryptObject(stored, length, password)
	if err != nil {
		t.Fatalf("DecryptObject: %v", err)
	}
	if loaded == object.Object(original) {
		t.Fatal("storage handed back the same pointer; this test no longer exercises a copy")
	}
	if !object.ValuesEqual(original, stored) {
		t.Error("the stored form of a closure is not the same function value")
	}
	if !object.ValuesEqual(original, loaded) {
		t.Error("a closure loaded from storage is not the same function value it was stored as")
	}
	if object.ValuesEqual(twin, loaded) {
		t.Error("a closure loaded from storage equals a different closure over the same function")
	}
}
