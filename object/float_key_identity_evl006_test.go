package object

import (
	"math"
	"strings"
	"testing"
)

// M26-EVL-006. Float.Inspect keeps six decimals, so a render is not an identity
// for a float: 7.9999991 and 7.9999994 render alike, and so do 0.0000001 and
// 0.0000002. Equality of arrays and hashes, an error's related values and the
// order of float hash keys all used to be decided by renders.

const (
	evl006A = 7.9999991
	evl006B = 7.9999994
)

func TestEVL006ContainersOfFloatsThatRenderAlikeAreNotEqual(t *testing.T) {
	a, b := &Float{Value: evl006A}, &Float{Value: evl006B}
	if a.Inspect() != b.Inspect() {
		t.Fatalf("the premise moved: %s and %s no longer render alike", a.Inspect(), b.Inspect())
	}

	if ValuesEqual(&Array{Elements: []Object{a}}, &Array{Elements: []Object{b}}) {
		t.Errorf("[%v] is equal to [%v]", evl006A, evl006B)
	}
	if !ValuesEqual(&Array{Elements: []Object{a}}, &Array{Elements: []Object{&Float{Value: evl006A}}}) {
		t.Errorf("[%v] is not equal to a second [%v]", evl006A, evl006A)
	}

	ha := buildHashWithValue(t, &Integer{Value: 1}, a)
	hb := buildHashWithValue(t, &Integer{Value: 1}, b)
	if ValuesEqual(ha, hb) {
		t.Errorf("{1: %v} is equal to {1: %v}", evl006A, evl006B)
	}
	if !ValuesEqual(ha, buildHashWithValue(t, &Integer{Value: 1}, &Float{Value: evl006A})) {
		t.Errorf("{1: %v} is not equal to a second {1: %v}", evl006A, evl006A)
	}

	// Float keys that render alike are two keys, not one.
	ka := buildHashWithValue(t, &Float{Value: 0.0000001}, &Integer{Value: 1})
	kb := buildHashWithValue(t, &Float{Value: 0.0000002}, &Integer{Value: 1})
	if ValuesEqual(ka, kb) {
		t.Errorf("{0.0000001: 1} is equal to {0.0000002: 1}")
	}
}

// A render also drops a string's quotes, so ["1"] used to equal [1]. The type
// rule ValuesEqual states for scalars now holds inside an array too.
func TestEVL006ArrayEqualityKeepsTheTypeRuleForEachElement(t *testing.T) {
	if ValuesEqual(&Array{Elements: []Object{&String{Value: "1"}}}, &Array{Elements: []Object{&Integer{Value: 1}}}) {
		t.Errorf(`["1"] is equal to [1]`)
	}
	if ValuesEqual(&Array{Elements: []Object{&Null{}}}, &Array{Elements: []Object{}}) {
		t.Errorf("[null] is equal to []")
	}
}

func TestEVL006ErrorRelatedFloatsAreComparedByValue(t *testing.T) {
	ea := &Error{Message: "m", Related: map[string]Object{"x": &Float{Value: evl006A}}}
	eb := &Error{Message: "m", Related: map[string]Object{"x": &Float{Value: evl006B}}}
	if ea.Equals(eb) {
		t.Errorf("errors whose related x is %v and %v are equal", evl006A, evl006B)
	}
	ec := &Error{Message: "m", Related: map[string]Object{"x": &Float{Value: evl006A}}}
	if !ea.Equals(ec) {
		t.Errorf("two errors with related x %v are not equal", evl006A)
	}
}

// Three keys that all render as 0.000000 used to come out in map order, which
// is a different order from one call to the next.
func TestEVL006FloatKeysThatRenderAlikeHaveOneOrder(t *testing.T) {
	h := buildHash(t, &Float{Value: 0.0000003}, &Float{Value: 0.0000001}, &Float{Value: 0.0000002})
	keyOrder := func() string {
		it, ok := NewIterator(h)
		if !ok {
			t.Fatal("a hash gave no iterator")
		}
		var parts []string
		for _, value := range it.Values {
			parts = append(parts, value.Inspect())
		}
		return strings.Join(parts, ",")
	}

	// buildHash numbers the values in the order the keys were given, so the
	// numeric key order 1e-7, 2e-7, 3e-7 holds the values 1, 2, 0.
	const wantValues = "1,2,0"
	first := h.Inspect()
	for i := 0; i < 50; i++ {
		if got := h.Inspect(); got != first {
			t.Fatalf("call %d rendered %s, the first rendered %s", i, got, first)
		}
		if got := keyOrder(); got != wantValues {
			t.Fatalf("iteration %d visited values %s, want %s", i, got, wantValues)
		}
	}
	if want := "{0.000000: 1, 0.000000: 2, 0.000000: 0}"; first != want {
		t.Errorf("Inspect = %s, want %s", first, want)
	}
}

// A string sort puts "10.000000" before "9.000000".
func TestEVL006FloatKeysAreOrderedNumerically(t *testing.T) {
	h := buildHash(t, &Float{Value: 10}, &Float{Value: 9}, &Float{Value: -1.5})
	if got, want := h.Inspect(), "{-1.500000: 2, 9.000000: 1, 10.000000: 0}"; got != want {
		t.Errorf("Inspect = %s, want %s", got, want)
	}
}

// The keys `<` cannot separate are distinct keys all the same, so they still
// need one order: -0.0 before 0.0, and NaN after every number. A NaN that `<`
// called equal to every key would leave sort.Slice no order to keep.
func TestEVL006FloatKeyOrderIsTotal(t *testing.T) {
	h := buildHash(t,
		&Float{Value: math.NaN()}, &Float{Value: 1}, &Float{Value: math.Copysign(0, -1)},
		&Float{Value: 0}, &Float{Value: math.Inf(-1)}, &Float{Value: math.Inf(1)},
	)
	const want = "{-Inf: 4, -0.000000: 2, 0.000000: 3, 1.000000: 1, +Inf: 5, NaN: 0}"
	for i := 0; i < 50; i++ {
		if got := h.Inspect(); got != want {
			t.Fatalf("call %d: Inspect = %s, want %s", i, got, want)
		}
	}
}

func buildHashWithValue(t *testing.T, key Object, value Object) *Hash {
	t.Helper()
	hashable, ok := key.(Hashable)
	if !ok {
		t.Fatalf("%T cannot be a hash key", key)
	}
	return &Hash{Pairs: map[HashKey]HashPair{hashable.HashKey(): {Key: key, Value: value}}}
}
