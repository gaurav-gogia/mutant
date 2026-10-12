package object

import (
	"fmt"
	"math"
	"strings"
	"testing"
)

// sortedPairsSampleKeys holds keys of every hashable type, chosen to make an
// order hard: keys of different types that render alike (1 and "1", true and
// "true", the buffer ab and the string "ab"), floats that render alike under
// %f, both zeros, both infinities, and NaNs with three different bit patterns.
// Each is a different hash key.
func sortedPairsSampleKeys() []Object {
	return []Object{
		&Integer{Value: -5}, &Integer{Value: 0}, &Integer{Value: 1}, &Integer{Value: 2}, &Integer{Value: 10},
		&String{Value: ""}, &String{Value: "1"}, &String{Value: "10"}, &String{Value: "2"},
		&String{Value: "ab"}, &String{Value: "true"}, &String{Value: "NaN"},
		&Boolean{Value: false}, &Boolean{Value: true},
		&Bytes{Value: []byte{}}, &Bytes{Value: []byte{0xab}}, &Bytes{Value: []byte{0x01, 0x00}}, &Bytes{Value: []byte{0x02}},
		&Float{Value: -1}, &Float{Value: math.Copysign(0, -1)}, &Float{Value: 0}, &Float{Value: 1},
		&Float{Value: 9}, &Float{Value: 10}, &Float{Value: 1e-7}, &Float{Value: 2e-7}, &Float{Value: 3e-7},
		&Float{Value: math.Inf(1)}, &Float{Value: math.Inf(-1)},
		&Float{Value: math.NaN()},
		&Float{Value: math.Float64frombits(0x7ff8000000000002)},
		&Float{Value: math.Float64frombits(0xfff8000000000000)},
	}
}

// SortedPairs breaks hashKeyLess's ties by HashKey, and that gives one total
// order only while hashKeyLess is a strict weak order: never less than itself,
// never both ways, transitive, and with "neither is less" transitive too. A
// comparison that broke the last rule -- a float compared by value leaves a
// NaN unordered against every other float, while -1 < 1 -- would let the map's
// order back into the result, and keys(h) and values(h) would disagree again
// (M26-BLT-007).
func TestHashKeyLessStaysAStrictWeakOrderForSortedPairs(t *testing.T) {
	keys := sortedPairsSampleKeys()
	seen := map[HashKey]string{}
	for _, k := range keys {
		hk := k.(Hashable).HashKey()
		if prior, dup := seen[hk]; dup {
			t.Fatalf("the sample holds one key twice: %s and %s", prior, k.Inspect())
		}
		seen[hk] = k.Inspect()
	}

	name := func(o Object) string { return fmt.Sprintf("%s %s", o.Type(), o.Inspect()) }
	unordered := func(a, b Object) bool { return !hashKeyLess(a, b) && !hashKeyLess(b, a) }

	// A broken comparison breaks many triples at once; the first few say why.
	broken := 0
	report := func(format string, args ...any) {
		t.Errorf(format, args...)
		if broken++; broken == 5 {
			t.FailNow()
		}
	}

	for _, a := range keys {
		if hashKeyLess(a, a) {
			report("hashKeyLess(%s, itself) is true", name(a))
		}
		for _, b := range keys {
			if hashKeyLess(a, b) && hashKeyLess(b, a) {
				report("%s and %s are each less than the other", name(a), name(b))
			}
			for _, c := range keys {
				if hashKeyLess(a, b) && hashKeyLess(b, c) && !hashKeyLess(a, c) {
					report("%s < %s < %s, but not %s < %s", name(a), name(b), name(c), name(a), name(c))
				}
				if unordered(a, b) && unordered(b, c) && !unordered(a, c) {
					report("%s ~ %s and %s ~ %s, but %s and %s are ordered", name(a), name(b), name(b), name(c), name(a), name(c))
				}
			}
		}
	}
}

// Keys that are easy to leave unordered -- floats that render alike, NaNs with
// different bit patterns -- still print and iterate in one order, and in the
// same order, on every call. Where hashKeyLess left two of them unordered, they
// used to come out in the order the Go map was ranged in, a different one from
// call to call.
func TestHashPrintsAndIteratesInOneOrderWhenKeysTie(t *testing.T) {
	h := buildHash(t,
		&Float{Value: 1e-7}, &Float{Value: 2e-7}, &Float{Value: 3e-7},
		&Float{Value: math.NaN()}, &Float{Value: math.Float64frombits(0x7ff8000000000002)},
	)

	iterated := func() string {
		it, ok := NewIterator(h)
		if !ok {
			t.Fatal("a hash is not iterable")
		}
		parts := make([]string, len(it.Keys))
		for i := range it.Keys {
			parts[i] = fmt.Sprintf("%s: %s", it.Keys[i].Inspect(), it.Values[i].Inspect())
		}
		return "{" + strings.Join(parts, ", ") + "}"
	}

	printed, walked := h.Inspect(), iterated()
	if printed != walked {
		t.Fatalf("printing and for-in disagree about the order:\n printed: %s\n walked:  %s", printed, walked)
	}
	for i := 0; i < 200; i++ {
		if got := h.Inspect(); got != printed {
			t.Fatalf("Inspect changed between calls:\n first: %s\n now:   %s", printed, got)
		}
		if got := iterated(); got != walked {
			t.Fatalf("the for-in order changed between calls:\n first: %s\n now:   %s", walked, got)
		}
	}
}
