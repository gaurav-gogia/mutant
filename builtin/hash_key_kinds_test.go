package builtin

import (
	"slices"
	"strings"
	"testing"

	"mutant/object"
)

// A hash key may be any value that implements object.Hashable, and the four
// builtins that take one -- has_key, get, set and delete -- take it through
// hashableKey, which asks exactly that. Their declared key contract is
// hashableKinds, and the editor holds a call to it. The two were written apart:
// hashableKinds predates the BYTES type, and Bytes.HashKey arrived without it,
// so the editor flagged `has_key(h, buffer)` on a call that answers true and
// named bytes_to_string as the repair. A buffer and a string are different
// keys, so the repaired call answered false.

// hashKeyKindSample is one value of each parameter kind. ParamAny is a contract
// word rather than a value, so it has none; any other kind validParamKinds names
// without a sample here fails TestHashableKindsMatchObjectHashable rather than
// going unchecked.
func hashKeyKindSample(kind ParamKind) (object.Object, bool) {
	switch kind {
	case ParamInt:
		return &object.Integer{Value: 7}, true
	case ParamFloat:
		return &object.Float{Value: 1.5}, true
	case ParamString:
		return &object.String{Value: "ab"}, true
	case ParamBytes:
		return &object.Bytes{Value: []byte("ab")}, true
	case ParamBool:
		return &object.Boolean{Value: true}, true
	case ParamArray:
		return &object.Array{Elements: []object.Object{}}, true
	case ParamHash:
		return &object.Hash{Pairs: map[object.HashKey]object.HashPair{}}, true
	case ParamFn:
		return &object.Function{}, true
	case ParamNull:
		return &object.Null{}, true
	case ParamStruct:
		return &object.Struct{TypeName: "Probe", Fields: map[string]object.Object{}}, true
	case ParamEnum:
		return &object.EnumValue{TypeName: "Probe", Tag: "A"}, true
	case ParamError:
		return &object.Error{Message: "probe"}, true
	}
	return nil, false
}

// hashKeyValueKinds is every kind a value can have, in a fixed order so a
// failure reads the same on every run.
func hashKeyValueKinds() []ParamKind {
	kinds := make([]ParamKind, 0, len(validParamKinds))
	for kind := range validParamKinds {
		if kind != ParamAny {
			kinds = append(kinds, kind)
		}
	}
	slices.Sort(kinds)
	return kinds
}

// hashKeyBuiltinCall calls one of the four key-taking builtins with key in its
// key position and a valid value in every other.
func hashKeyBuiltinCall(name string, h *object.Hash, key object.Object) object.Object {
	switch name {
	case BuiltinNameHasKey:
		return HasKey(h, key)
	case BuiltinNameGet:
		return Get(h, key, &object.Integer{Value: 0})
	case BuiltinNameSet:
		return Set(h, key, &object.Integer{Value: 1})
	case BuiltinNameDelete:
		return Delete(h, key)
	}
	return nil
}

// TestHashableKindsMatchObjectHashable holds hashableKinds to what it says it
// is: exactly the kinds whose values implement object.Hashable. A hashable kind
// left out is a key the builtins take and the editor flags; a kind listed that
// is not hashable is a key the editor passes and the builtins refuse.
func TestHashableKindsMatchObjectHashable(t *testing.T) {
	for _, kind := range hashKeyValueKinds() {
		sample, ok := hashKeyKindSample(kind)
		if !ok {
			t.Errorf("no sample value for kind %s; add one to hashKeyKindSample", kind)
			continue
		}
		_, hashable := sample.(object.Hashable)
		if declared := slices.Contains(hashableKinds, kind); declared != hashable {
			t.Errorf("kind %s: its values implement object.Hashable = %v, but hashableKinds lists it = %v",
				kind, hashable, declared)
		}
	}
}

// TestHashKeyBuiltinsDeclareTheKeysTheyTake asks has_key, get, set and delete
// themselves which kinds their key position takes, and holds the declared
// contract to the answer in both directions.
func TestHashKeyBuiltinsDeclareTheKeysTheyTake(t *testing.T) {
	for _, name := range []string{BuiltinNameHasKey, BuiltinNameGet, BuiltinNameSet, BuiltinNameDelete} {
		params, ok := ParamSpecs(name)
		if !ok || len(params) < 2 || params[1].Name != "key" {
			t.Errorf("%s: no declared key parameter in position 2", name)
			continue
		}
		key := params[1]
		for _, kind := range hashKeyValueKinds() {
			sample, ok := hashKeyKindSample(kind)
			if !ok {
				continue // reported by TestHashableKindsMatchObjectHashable
			}
			result := hashKeyBuiltinCall(name, &object.Hash{Pairs: map[object.HashKey]object.HashPair{}}, sample)
			_, refused := result.(*object.Error)
			if accepts := key.Accepts(kind); accepts == refused {
				verdict := "took one"
				if refused {
					verdict = "refused one"
				}
				t.Errorf("%s: the key declared %s accepts %s = %v, but the builtin %s (returned %s)",
					name, key.KindsText(), kind, accepts, verdict, result.Inspect())
			}
		}
	}
}

// TestHashKeyRefusalNamesEveryHashableKind keeps the runtime's refusal of an
// unusable key in step with the keys it takes. The message is the list a
// reader sees when the call fails, and it left BYTES out.
func TestHashKeyRefusalNamesEveryHashableKind(t *testing.T) {
	_, refusal := hashableKey(BuiltinNameHasKey, &object.Array{Elements: []object.Object{}})
	if refusal == nil {
		t.Fatal("an ARRAY key was not refused")
	}
	for _, kind := range hashKeyValueKinds() {
		sample, ok := hashKeyKindSample(kind)
		if !ok {
			continue // reported by TestHashableKindsMatchObjectHashable
		}
		if _, hashable := sample.(object.Hashable); hashable && !strings.Contains(refusal.Message, string(kind)) {
			t.Errorf("the refusal %q does not name %s, which a hash key may be", refusal.Message, kind)
		}
	}
}

// TestBytesHashKeyIsItsOwnKeyspace pins what the key parameter's prose says: a
// buffer finds the entry it made, and a string with the same text is a
// different key. That second half is why converting the key, as the editor
// once suggested, changes the answer.
func TestBytesHashKeyIsItsOwnKeyspace(t *testing.T) {
	buffer := &object.Bytes{Value: []byte("ab")}
	h, ok := Set(&object.Hash{Pairs: map[object.HashKey]object.HashPair{}}, buffer, &object.Integer{Value: 1}).(*object.Hash)
	if !ok {
		t.Fatal("set with a BYTES key did not return a hash")
	}

	if got := HasKey(h, &object.Bytes{Value: []byte("ab")}); got.Inspect() != "true" {
		t.Errorf("has_key(h, buffer) = %s, want true", got.Inspect())
	}
	if got := Get(h, &object.Bytes{Value: []byte("ab")}, &object.Integer{Value: 0}); got.Inspect() != "1" {
		t.Errorf("get(h, buffer, 0) = %s, want 1", got.Inspect())
	}
	if got := HasKey(h, &object.String{Value: "ab"}); got.Inspect() != "false" {
		t.Errorf("has_key(h, \"ab\") = %s, want false: a string is not the buffer's key", got.Inspect())
	}
	if rest, ok := Delete(h, buffer).(*object.Hash); !ok || len(rest.Pairs) != 0 {
		t.Errorf("delete(h, buffer) = %s, want an empty hash", Delete(h, buffer).Inspect())
	}
}
