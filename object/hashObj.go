package object

import (
	"bytes"
	"fmt"
	"math"
	"strings"
)

// HashKey identifies a hash entry, and every field takes part when a map
// compares two of them, which is the point: two keys are one entry only when
// their values are equal. A boolean, an integer or a float fits in Value
// exactly. A string is carried whole in Text -- it shares the string's memory,
// so nothing is copied -- because a digest in Value made two strings with one
// digest a single key, and a pair of FNV-1a-64 collisions takes under a minute
// to find. A buffer carries its SHA-256 in Text; see Bytes.HashKey.
type HashKey struct {
	Type  ObjectType
	Value uint64
	Text  string
}

type HashPair struct {
	Key   Object
	Value Object
}

type Hash struct{ Pairs map[HashKey]HashPair }

type Hashable interface{ HashKey() HashKey }

func (b *Boolean) HashKey() HashKey {
	var value uint64
	if b.Value {
		value = 1
	} else {
		value = 0
	}
	return HashKey{Type: b.Type(), Value: value}
}

func (i *Integer) HashKey() HashKey {
	return HashKey{Type: i.Type(), Value: uint64(i.Value)}
}

func (f *Float) HashKey() HashKey {
	return HashKey{Type: f.Type(), Value: math.Float64bits(f.Value)}
}

func (s *String) HashKey() HashKey {
	return HashKey{Type: s.Type(), Text: s.Value}
}

func (h *Hash) Type() ObjectType { return HASH_OBJ }

// Inspect renders the hash with its keys in a stable order.
//
// Pairs live in a Go map, and ranging a map yields a different order on every
// run. That made a program's own output differ run to run for the same input --
// which for reports that get diffed, hashed, or committed is a defect rather
// than a cosmetic detail. Sorting here rather than at each call site keeps every
// path that prints a hash (putln, putf, string conversion, nested hashes)
// consistent.
//
// Integer and float keys are ordered numerically, so {2: ..., 10: ...} does not
// come out with 10 first the way a plain string sort would. For floats that is
// also what keeps the order fixed: Float.Inspect keeps six decimals, so keys
// such as 0.0000001 and 0.0000002 render alike, and ordered by their renders
// they came out in whatever order the map handed them over.
func (h *Hash) Inspect() string {
	var out bytes.Buffer

	pairs := h.SortedPairs()

	rendered := make([]string, 0, len(pairs))
	for _, pair := range pairs {
		rendered = append(rendered, fmt.Sprintf("%s: %s", pair.Key.Inspect(), pair.Value.Inspect()))
	}

	out.WriteString("{")
	out.WriteString(strings.Join(rendered, ", "))
	out.WriteString("}")

	return out.String()
}

// hashKeyLess orders keys of the same type naturally and groups differing types
// by type name, so a mixed-key hash still has one definite order.
func hashKeyLess(a, b Object) bool {
	if a.Type() != b.Type() {
		return a.Type() < b.Type()
	}

	switch left := a.(type) {
	case *Integer:
		if right, ok := b.(*Integer); ok {
			return left.Value < right.Value
		}
	case *String:
		if right, ok := b.(*String); ok {
			return left.Value < right.Value
		}
	case *Float:
		if right, ok := b.(*Float); ok {
			return floatKeyLess(left.Value, right.Value)
		}
	case *Bytes:
		if right, ok := b.(*Bytes); ok {
			return bytes.Compare(left.Value, right.Value) < 0
		}
	case *Boolean:
		if right, ok := b.(*Boolean); ok {
			return !left.Value && right.Value
		}
	}

	return a.Inspect() < b.Inspect()
}

// floatKeyLess orders float keys numerically, and orders the keys `<` cannot
// tell apart as well, because each of them is a key of its own: a float key is
// one entry by its bits. -0.0 comes before 0.0, every NaN comes after every
// number, and NaNs order among themselves by their bits. Without that last part
// a NaN key would compare equal to every other key, which is no order at all.
func floatKeyLess(left, right float64) bool {
	leftNaN, rightNaN := math.IsNaN(left), math.IsNaN(right)
	if leftNaN || rightNaN {
		if leftNaN != rightNaN {
			return rightNaN
		}
		return math.Float64bits(left) < math.Float64bits(right)
	}
	if left != right {
		return left < right
	}
	return math.Signbit(left) && !math.Signbit(right)
}
