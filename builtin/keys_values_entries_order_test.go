package builtin

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"mutant/object"
)

// keysOrderHash builds a hash from alternating keys and values.
func keysOrderHash(t *testing.T, kv ...object.Object) *object.Hash {
	t.Helper()
	pairs := map[object.HashKey]object.HashPair{}
	for i := 0; i+1 < len(kv); i += 2 {
		hashable, ok := kv[i].(object.Hashable)
		if !ok {
			t.Fatalf("%s cannot be a hash key", kv[i].Type())
		}
		pairs[hashable.HashKey()] = object.HashPair{Key: kv[i], Value: kv[i+1]}
	}
	return &object.Hash{Pairs: pairs}
}

func keysOrderName(o object.Object) string { return fmt.Sprintf("%s %s", o.Type(), o.Inspect()) }

// keysOrderCases are hashes whose order the old text sort got wrong (10 before
// 2) or left to the Go map (keys that render alike), plus keys that are easy to
// leave unordered: floats that render alike, and NaNs.
func keysOrderCases(t *testing.T) []struct {
	name string
	h    *object.Hash
} {
	return []struct {
		name string
		h    *object.Hash
	}{
		{"integers, by value rather than by text", keysOrderHash(t,
			intObj(2), stringObj("two"), intObj(10), stringObj("ten"), intObj(-3), stringObj("minus three"))},
		{"an integer and a string that render alike", keysOrderHash(t,
			intObj(1), stringObj("int"), stringObj("1"), stringObj("str"))},
		{"a boolean and a string that render alike", keysOrderHash(t,
			&object.Boolean{Value: true}, stringObj("bool"), stringObj("true"), stringObj("str"))},
		{"a buffer and a string that render alike", keysOrderHash(t,
			&object.Bytes{Value: []byte{0xab}}, stringObj("bytes"), stringObj("ab"), stringObj("str"))},
		{"floats that render alike", keysOrderHash(t,
			&object.Float{Value: 1e-7}, stringObj("first"), &object.Float{Value: 2e-7}, stringObj("second"),
			&object.Float{Value: 3e-7}, stringObj("third"))},
		{"NaNs with different bit patterns", keysOrderHash(t,
			&object.Float{Value: math.NaN()}, stringObj("nan"),
			&object.Float{Value: math.Float64frombits(0x7ff8000000000002)}, stringObj("other nan"),
			&object.Float{Value: 1}, stringObj("one"))},
		{"every hashable type at once", keysOrderHash(t,
			stringObj("b"), intObj(1), intObj(10), intObj(2), &object.Boolean{Value: false}, intObj(3),
			&object.Float{Value: 2.5}, intObj(4), &object.Bytes{Value: []byte("a")}, intObj(5),
			stringObj("a"), intObj(6), intObj(2), intObj(7), &object.Boolean{Value: true}, intObj(8))},
	}
}

// M26-BLT-007. keys, values and entries list a hash in the order a for-in loop
// walks it and printing shows it. They sorted for themselves by rendered key,
// so keys({2: .., 10: ..}) was [10, 2] while the loop and the printer said
// [2, 10], and keys that render alike came out in whatever order the Go map
// was ranged in.
func TestKeysValuesEntriesListAHashInItsForInOrder(t *testing.T) {
	for _, c := range keysOrderCases(t) {
		t.Run(c.name, func(t *testing.T) {
			for round := 0; round < 50; round++ {
				it, ok := object.NewIterator(c.h)
				if !ok {
					t.Fatal("a hash is not iterable")
				}
				keys := mustArray(t, Keys(c.h)).Elements
				values := mustArray(t, Values(c.h)).Elements
				entries := mustArray(t, Entries(c.h)).Elements
				if len(keys) != len(it.Keys) || len(values) != len(it.Keys) || len(entries) != len(it.Keys) {
					t.Fatalf("lengths: keys %d, values %d, entries %d, for-in %d", len(keys), len(values), len(entries), len(it.Keys))
				}

				printed := make([]string, len(keys))
				for i := range it.Keys {
					if keys[i] != it.Keys[i] {
						t.Fatalf("round %d: keys(h)[%d] is %s, the for-in loop's key %d is %s", round, i, keysOrderName(keys[i]), i, keysOrderName(it.Keys[i]))
					}
					if values[i] != it.Values[i] {
						t.Fatalf("round %d: values(h)[%d] is %s, the for-in loop's value %d is %s", round, i, keysOrderName(values[i]), i, keysOrderName(it.Values[i]))
					}
					pair := entries[i].(*object.Array).Elements
					if len(pair) != 2 || pair[0] != it.Keys[i] || pair[1] != it.Values[i] {
						t.Fatalf("round %d: entries(h)[%d] is %s, the for-in loop's pair %d is [%s, %s]", round, i, entries[i].Inspect(), i, it.Keys[i].Inspect(), it.Values[i].Inspect())
					}
					printed[i] = keys[i].Inspect() + ": " + values[i].Inspect()
				}
				if got, want := "{"+strings.Join(printed, ", ")+"}", c.h.Inspect(); got != want {
					t.Fatalf("round %d: keys and values in order read %s, the hash prints as %s", round, got, want)
				}
			}
		})
	}
}

// M26-BLT-007. keys(h) and values(h) are separate calls, and a program pairs
// their answers by position. With 1 and "1" ordered by rendered text alone,
// each call kept the Go map's order for the tie, and keys(h)[0] and values(h)[0]
// came from different entries on a large share of calls.
func TestKeysAndValuesFromSeparateCallsNameOneEntry(t *testing.T) {
	for _, c := range keysOrderCases(t) {
		t.Run(c.name, func(t *testing.T) {
			first := ""
			for round := 0; round < 500; round++ {
				keys := mustArray(t, Keys(c.h)).Elements
				values := mustArray(t, Values(c.h)).Elements
				order := make([]string, len(keys))
				for i, k := range keys {
					pair, ok := c.h.Pairs[k.(object.Hashable).HashKey()]
					if !ok {
						t.Fatalf("keys(h)[%d] = %s is not a key of h", i, keysOrderName(k))
					}
					if pair.Value != values[i] {
						t.Fatalf("round %d: keys(h)[%d] is %s, whose value is %s, but values(h)[%d] is %s",
							round, i, keysOrderName(k), keysOrderName(pair.Value), i, keysOrderName(values[i]))
					}
					order[i] = keysOrderName(k)
				}
				got := strings.Join(order, ", ")
				if round == 0 {
					first = got
				} else if got != first {
					t.Fatalf("round %d: keys(h) changed order between calls:\n first: %s\n now:   %s", round, first, got)
				}
			}
		})
	}
}
