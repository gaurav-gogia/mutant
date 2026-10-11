package object

import (
	"cmp"
	"slices"
)

// SortedPairs returns the hash's pairs in the order that printing the hash
// (Inspect), a for-in loop over it (NewIterator) and the keys, values and
// entries builtins all use. One function decides it so that those readers
// cannot drift apart.
//
// The order is hashKeyLess's -- keys grouped by type name, integers by value,
// strings and buffers by their bytes -- and where hashKeyLess leaves two
// different keys unordered, the tie is broken by their HashKeys, which no two
// entries share. The order is therefore total and depends on the pairs alone.
// A tie left to the sort would come out in an order that depends on how the Go
// map was ranged, which differs from call to call, and two readers of one hash
// -- keys(h) and values(h) -- could then list its entries in two different
// orders. All of that holds while hashKeyLess is a strict weak order, and
// TestHashKeyLessStaysAStrictWeakOrderForSortedPairs holds it to one.
func (h *Hash) SortedPairs() []HashPair {
	type entry struct {
		key  HashKey
		pair HashPair
	}
	entries := make([]entry, 0, len(h.Pairs))
	for key, pair := range h.Pairs {
		entries = append(entries, entry{key: key, pair: pair})
	}
	slices.SortFunc(entries, func(a, b entry) int {
		switch {
		case hashKeyLess(a.pair.Key, b.pair.Key):
			return -1
		case hashKeyLess(b.pair.Key, a.pair.Key):
			return 1
		}
		return compareHashKeys(a.key, b.key)
	})

	pairs := make([]HashPair, len(entries))
	for i, e := range entries {
		pairs[i] = e.pair
	}
	return pairs
}

// compareHashKeys orders two HashKeys field by field. It answers 0 only for
// equal keys, and two entries of one hash never have equal keys.
func compareHashKeys(a, b HashKey) int {
	if c := cmp.Compare(a.Type, b.Type); c != 0 {
		return c
	}
	if c := cmp.Compare(a.Value, b.Value); c != 0 {
		return c
	}
	return cmp.Compare(a.Text, b.Text)
}
