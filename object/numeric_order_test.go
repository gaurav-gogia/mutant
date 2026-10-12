package object

import (
	"math"
	"testing"
)

// TestCompareNumericIsExactAcrossIntegerAndFloat pins the shared ordering
// sort_by uses in both engines (M26-EVL-021). Every case where the INTEGER
// differs from the FLOAT here would compare as equal through float64.
func TestCompareNumericIsExactAcrossIntegerAndFloat(t *testing.T) {
	i := func(v int64) Object { return &Integer{Value: v} }
	f := func(v float64) Object { return &Float{Value: v} }
	cases := []struct {
		name string
		a, b Object
		want int
		ok   bool
	}{
		{"2^53+1 above the float 2^53", i(1<<53 + 1), f(1 << 53), 1, true},
		{"the float 2^53 below 2^53+1", f(1 << 53), i(1<<53 + 1), -1, true},
		{"2^53 equals the float 2^53", i(1 << 53), f(1 << 53), 0, true},
		{"a nanosecond timestamp above its float neighbour", i(1700000000000000001), f(1.7e18), 1, true},
		{"-2^53-1 below the float -2^53", i(-(1<<53 + 1)), f(-(1 << 53)), -1, true},
		{"an integer above a fraction under it", i(3), f(2.5), 1, true},
		{"an integer below a fraction over it", i(2), f(2.5), -1, true},
		{"a negative fraction", i(-3), f(-2.5), -1, true},
		{"MaxInt64 below the float 2^63", i(math.MaxInt64), f(9223372036854775808.0), -1, true},
		{"MinInt64 equals the float -2^63", i(math.MinInt64), f(-9223372036854775808.0), 0, true},
		{"anything below +Inf", i(math.MaxInt64), f(math.Inf(1)), -1, true},
		{"anything above -Inf", f(math.Inf(-1)), i(math.MinInt64), -1, true},
		{"integers", i(1<<62 + 1), i(1 << 62), 1, true},
		{"floats", f(1.5), f(2.5), -1, true},
		{"NaN against an integer", i(0), f(math.NaN()), 0, false},
		{"NaN against a float", f(math.NaN()), f(1), 0, false},
		{"a string is not numeric", i(1), &String{Value: "1"}, 0, false},
	}
	for _, c := range cases {
		got, ok := CompareNumeric(c.a, c.b)
		if got != c.want || ok != c.ok {
			t.Errorf("%s: CompareNumeric = (%d, %t), want (%d, %t)", c.name, got, ok, c.want, c.ok)
		}
	}
}
