package parity

// sort_by orders an INTEGER key against a FLOAT key exactly, in both engines
// (M26-EVL-021).
//
// Both comparators used to widen the integer to float64, which rounds above
// 2^53: 2^53+1 and the float 2^53 compared as equal, so the larger key kept
// its input place ahead of the smaller. The two engines agreed on that wrong
// order, so asking only whether they match would have passed throughout. Each
// engine is therefore checked against the expected order on its own, and the
// agreement between them follows from that.

import (
	"fmt"
	"strings"
	"testing"

	"mutant/object"
)

// sortByMixedKeyCases holds each sort_by call with the elements it must return,
// in order, rendered as KIND(exact value).
func sortByMixedKeyCases() []struct{ label, src, want string } {
	return []struct{ label, src, want string }{
		{"2^53+1 against the float 2^53",
			`sort_by([9007199254740993, 9007199254740992.0], fn(x) { return x; })`,
			"FLOAT(9007199254740992) INTEGER(9007199254740993)"},
		{"the float 2^53 against 2^53+1",
			`sort_by([9007199254740992.0, 9007199254740993], fn(x) { return x; })`,
			"FLOAT(9007199254740992) INTEGER(9007199254740993)"},
		{"a nanosecond timestamp against its float neighbour",
			`sort_by([1700000000000000001, 1700000000000000000.0], fn(x) { return x; })`,
			"FLOAT(1700000000000000000) INTEGER(1700000000000000001)"},
		{"the negative side of 2^53",
			`sort_by([-9007199254740992.0, -9007199254740993], fn(x) { return x; })`,
			"INTEGER(-9007199254740993) FLOAT(-9007199254740992)"},
		// The integer 2^53 and the float 2^53 are equal, so the stable sort keeps
		// them in input order; only 2^53+1 moves. Widening made the float equal to
		// both integers while the integers stayed unequal, which no order can
		// honour, and the sort left 2^53+1 first.
		{"equal keys keep their order and the larger one moves",
			`sort_by([9007199254740993, 9007199254740992.0, 9007199254740992], fn(x) { return x; })`,
			"FLOAT(9007199254740992) INTEGER(9007199254740992) INTEGER(9007199254740993)"},
		{"a fraction below an integer",
			`sort_by([3, 2.5], fn(x) { return x; })`,
			"FLOAT(2.5) INTEGER(3)"},
		{"a float above the int64 range",
			`sort_by([10000000000000000000.0, 9223372036854775807], fn(x) { return x; })`,
			"INTEGER(9223372036854775807) FLOAT(10000000000000000000)"},
	}
}

// renderSortByMixedKeyResult renders an array result exactly, without the
// rounding Float.Inspect and normalize's %g apply.
func renderSortByMixedKeyResult(o object.Object) string {
	arr, ok := o.(*object.Array)
	if !ok {
		if o == nil {
			return "<nil>"
		}
		return fmt.Sprintf("not an array: %s %s", o.Type(), o.Inspect())
	}
	parts := make([]string, len(arr.Elements))
	for i, el := range arr.Elements {
		switch v := el.(type) {
		case *object.Integer:
			parts[i] = fmt.Sprintf("INTEGER(%d)", v.Value)
		case *object.Float:
			parts[i] = "FLOAT(" + strings.TrimSuffix(fmt.Sprintf("%.1f", v.Value), ".0") + ")"
		default:
			parts[i] = string(el.Type()) + "(" + el.Inspect() + ")"
		}
	}
	return strings.Join(parts, " ")
}

func TestSortByOrdersMixedNumericKeysExactlyInBothEngines(t *testing.T) {
	for _, c := range sortByMixedKeyCases() {
		t.Run(c.label, func(t *testing.T) {
			if got := renderSortByMixedKeyResult(evalViaEvaluator(c.src)); got != c.want {
				t.Errorf("evaluator: %s\n got  %s\n want %s", c.src, got, c.want)
			}
			vmOut, err := evalViaVM(t, c.src)
			if err != nil {
				t.Fatalf("vm: %s: %v", c.src, err)
			}
			if got := renderSortByMixedKeyResult(vmOut); got != c.want {
				t.Errorf("vm: %s\n got  %s\n want %s", c.src, got, c.want)
			}
		})
	}
}
