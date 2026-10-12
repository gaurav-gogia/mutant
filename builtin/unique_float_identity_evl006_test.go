package builtin

import (
	"math"
	"testing"

	"mutant/object"
)

// M26-EVL-006. unique keyed an element by its type and Inspect, and
// Float.Inspect keeps six decimals, so two different floats were one element.
// contains and index_of on composites, and keys/values/entries on float keys,
// leaned on the same renders.

const (
	evl006UniqueA = 7.9999991
	evl006UniqueB = 7.9999994
)

func evl006Floats(values ...float64) []object.Object {
	out := make([]object.Object, len(values))
	for i, v := range values {
		out[i] = &object.Float{Value: v}
	}
	return out
}

func evl006Array(elements ...object.Object) *object.Array {
	return &object.Array{Elements: elements}
}

func evl006UniqueLen(t *testing.T, arr *object.Array) int {
	t.Helper()
	got, ok := Unique(arr).(*object.Array)
	if !ok {
		t.Fatalf("unique returned %T", Unique(arr))
	}
	return len(got.Elements)
}

func TestEVL006UniqueKeepsFloatsThatRenderAlike(t *testing.T) {
	if n := evl006UniqueLen(t, evl006Array(evl006Floats(evl006UniqueA, evl006UniqueB)...)); n != 2 {
		t.Errorf("len(unique([%v, %v])) = %d, want 2", evl006UniqueA, evl006UniqueB, n)
	}
	nested := evl006Array(
		evl006Array(evl006Floats(evl006UniqueA)...),
		evl006Array(evl006Floats(evl006UniqueB)...),
	)
	if n := evl006UniqueLen(t, nested); n != 2 {
		t.Errorf("len(unique([[%v], [%v]])) = %d, want 2", evl006UniqueA, evl006UniqueB, n)
	}
	if n := evl006UniqueLen(t, evl006Array(evl006Floats(evl006UniqueA, evl006UniqueA)...)); n != 1 {
		t.Errorf("len(unique([%v, %v])) = %d, want 1", evl006UniqueA, evl006UniqueA, n)
	}
}

// What unique already answered from renders, apart from the lost digits, it
// still answers: -0.0 and 0.0 are two elements, two NaNs are one, and two
// arrays holding the same values are one.
func TestEVL006UniqueKeepsItsOtherAnswers(t *testing.T) {
	for _, c := range []struct {
		name string
		arr  *object.Array
		want int
	}{
		{"-0.0 and 0.0", evl006Array(evl006Floats(math.Copysign(0, -1), 0)...), 2},
		{"two NaNs", evl006Array(evl006Floats(math.NaN(), math.NaN())...), 1},
		{"two equal arrays", evl006Array(
			evl006Array(&object.Integer{Value: 1}, &object.String{Value: "a"}),
			evl006Array(&object.Integer{Value: 1}, &object.String{Value: "a"})), 1},
		{"1 and 1.0", evl006Array(&object.Integer{Value: 1}, &object.Float{Value: 1}), 2},
	} {
		if n := evl006UniqueLen(t, c.arr); n != c.want {
			t.Errorf("%s: unique left %d elements, want %d", c.name, n, c.want)
		}
	}
}

// A render drops a string's quotes and a null's whole spelling, so these pairs
// rendered alike and unique merged them.
func TestEVL006UniqueDoesNotMergeArraysThatRenderAlike(t *testing.T) {
	for _, c := range []struct {
		name string
		arr  *object.Array
	}{
		{`["a, b"] and ["a", "b"]`, evl006Array(
			evl006Array(&object.String{Value: "a, b"}),
			evl006Array(&object.String{Value: "a"}, &object.String{Value: "b"}))},
		{`["1"] and [1]`, evl006Array(
			evl006Array(&object.String{Value: "1"}),
			evl006Array(&object.Integer{Value: 1}))},
		{"[null] and []", evl006Array(
			evl006Array(&object.Null{}),
			evl006Array())},
	} {
		if n := evl006UniqueLen(t, c.arr); n != 2 {
			t.Errorf("%s: unique left %d elements, want 2", c.name, n)
		}
	}
}

func TestEVL006ContainsAndIndexOfCompareNestedFloatsByValue(t *testing.T) {
	haystack := evl006Array(evl006Array(evl006Floats(evl006UniqueA)...))
	needle := evl006Array(evl006Floats(evl006UniqueB)...)
	if got, ok := Contains(haystack, needle).(*object.Boolean); !ok || got.Value {
		t.Errorf("contains([[%v]], [%v]) = %s, want false", evl006UniqueA, evl006UniqueB, got.Inspect())
	}
	if got, ok := IndexOf(haystack, needle).(*object.Integer); !ok || got.Value != -1 {
		t.Errorf("index_of([[%v]], [%v]) = %s, want -1", evl006UniqueA, evl006UniqueB, IndexOf(haystack, needle).Inspect())
	}
}

// keys sorts by render, and three float keys that all render as 0.000000 came
// back in the map's order, which changes from call to call.
func TestEVL006KeysOfFloatsThatRenderAlikeHaveOneOrder(t *testing.T) {
	pairs := map[object.HashKey]object.HashPair{}
	for i, v := range []float64{0.0000003, 0.0000001, 0.0000002} {
		key := &object.Float{Value: v}
		pairs[key.HashKey()] = object.HashPair{Key: key, Value: &object.Integer{Value: int64(i)}}
	}
	h := &object.Hash{Pairs: pairs}

	for i := 0; i < 50; i++ {
		got, ok := Keys(h).(*object.Array)
		if !ok || len(got.Elements) != 3 {
			t.Fatalf("keys returned %v", Keys(h))
		}
		for j, want := range []float64{0.0000001, 0.0000002, 0.0000003} {
			if f, ok := got.Elements[j].(*object.Float); !ok || f.Value != want {
				t.Fatalf("call %d: keys[%d] = %v, want %v", i, j, got.Elements[j], want)
			}
		}
	}
}
