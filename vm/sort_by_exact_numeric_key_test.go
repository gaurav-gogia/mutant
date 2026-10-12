package vm

import (
	"testing"

	"mutant/object"
)

// TestSortByOrdersAnIntegerKeyAgainstAFloatKeyExactly pins M26-EVL-021 on the
// VM: objectKeyLess used to widen the integer to float64, so above 2^53 two
// different keys compared as equal and the larger one kept its place first.
func TestSortByOrdersAnIntegerKeyAgainstAFloatKeyExactly(t *testing.T) {
	cases := []struct {
		src       string
		wantFirst string // KIND of the element that must sort first
	}{
		{`sort_by([9007199254740993, 9007199254740992.0], fn(x) { return x; })`, object.FLOAT_OBJ},
		{`sort_by([9007199254740992.0, 9007199254740993], fn(x) { return x; })`, object.FLOAT_OBJ},
		{`sort_by([1700000000000000001, 1700000000000000000.0], fn(x) { return x; })`, object.FLOAT_OBJ},
		{`sort_by([-9007199254740992.0, -9007199254740993], fn(x) { return x; })`, object.INTEGER_OBJ},
	}
	for _, c := range cases {
		machine, err := runEncryptedVM(c.src)
		if err != nil {
			t.Fatalf("%s -> vm error: %s", c.src, err)
		}
		arr, ok := machine.LastPoppedStackElement().(*object.Array)
		if !ok || len(arr.Elements) != 2 {
			t.Fatalf("%s: want a two-element array, got %v", c.src, machine.LastPoppedStackElement())
		}
		if got := string(arr.Elements[0].Type()); got != c.wantFirst {
			t.Errorf("%s: first element is %s %s, want the %s", c.src, got, arr.Elements[0].Inspect(), c.wantFirst)
		}
	}
}
