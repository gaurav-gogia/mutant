package object

import "math"

// Exact ordering of Mutant's two numeric types, for the comparators both
// engines share.
//
// sort_by is implemented twice -- once in vm/higher_order.go and once in
// evaluator/higher_order.go, which runs it when a macro body calls it -- and
// the two copies used to order an INTEGER key against a FLOAT key by widening
// the integer to float64. That conversion is lossy above 2^53 and silent about
// it, and the lossy range is this tool's working range: a Unix nanosecond
// timestamp is about 1.7e18, where float64 spacing is 256. So 2^53+1 and the
// float 2^53 compared as equal, and the larger key could sort first
// (M26-EVL-021). Keeping the rule here, once, is what stops the two engines
// from ever ordering the same keys two different ways.
//
// Nothing here converts an integer to a float. Where the two types meet the
// comparison is decided without one, because the conversion is the defect.

// twoPow63Float is the smallest float64 above MaxInt64, and the boundary every
// range test here uses. MaxInt64 itself is not a float64 -- float64(MaxInt64)
// rounds up to this value -- so comparing against a conversion of it would
// admit one number it must not.
const twoPow63Float = 9223372036854775808.0

// compareIntFloat compares a against b exactly, returning -1, 0 or +1. ok is
// false for NaN alone, which stands in no order with anything.
func compareIntFloat(a int64, b float64) (int, bool) {
	if math.IsNaN(b) {
		return 0, false
	}
	// Outside int64's range the sign of b settles it. -twoPow63Float is exactly
	// MinInt64, so a b equal to it is in range and only a b below it is not.
	if b >= twoPow63Float {
		return -1, true
	}
	if b < -twoPow63Float {
		return 1, true
	}
	// b now lies in [-2^63, 2^63), so its floor converts to an int64 exactly
	// and comparing whole parts is comparing integers.
	whole := math.Floor(b)
	ib := int64(whole)
	if a < ib {
		return -1, true
	}
	if a > ib {
		return 1, true
	}
	// Equal whole parts. b carries a fraction exactly when it stands above its
	// own floor, and then a, which equals that floor, is the smaller.
	if b > whole {
		return -1, true
	}
	return 0, true
}

// CompareNumeric compares two numeric objects (INTEGER or FLOAT, in any mix)
// exactly, returning -1, 0 or +1. ok is false when either argument is not
// numeric, and when either is NaN, which stands in no order with anything.
func CompareNumeric(a, b Object) (int, bool) {
	ai, aIsInt := a.(*Integer)
	bi, bIsInt := b.(*Integer)
	switch {
	case aIsInt && bIsInt:
		if ai.Value < bi.Value {
			return -1, true
		}
		if ai.Value > bi.Value {
			return 1, true
		}
		return 0, true
	case aIsInt:
		bf, ok := b.(*Float)
		if !ok {
			return 0, false
		}
		return compareIntFloat(ai.Value, bf.Value)
	case bIsInt:
		af, ok := a.(*Float)
		if !ok {
			return 0, false
		}
		// The arguments go in the other way round, so the answer comes back
		// the other way round.
		c, ok := compareIntFloat(bi.Value, af.Value)
		return -c, ok
	}
	af, aOK := a.(*Float)
	bf, bOK := b.(*Float)
	if !aOK || !bOK {
		return 0, false
	}
	if math.IsNaN(af.Value) || math.IsNaN(bf.Value) {
		return 0, false
	}
	if af.Value < bf.Value {
		return -1, true
	}
	if af.Value > bf.Value {
		return 1, true
	}
	return 0, true
}
