package builtin

import (
	"sort"
	"strconv"
	"strings"

	"mutant/object"
)

// objectsEqual is how `contains` and `index_of` decide that an element is the
// one being looked for.
//
// It is object.ValuesEqual, the same definition `==` is built on, so a search
// cannot answer differently from the comparison a caller would have written by
// hand. It used to be a second copy of that switch, and the copy had no struct
// case: `contains(records, r)` was a coin toss for as long as comparing two
// structs was.
func objectsEqual(a, b object.Object) bool {
	return object.ValuesEqual(a, b)
}

func requireArrayArg(op string, arg object.Object, pos int) (*object.Array, *object.Error) {
	arr, ok := arg.(*object.Array)
	if !ok {
		return nil, newError("argument %d to `%s` must be ARRAY, got %s", pos, op, arg.Type())
	}
	return arr, nil
}

func requireHashArg(op string, arg object.Object, pos int) (*object.Hash, *object.Error) {
	h, ok := arg.(*object.Hash)
	if !ok {
		return nil, newError("argument %d to `%s` must be HASH, got %s", pos, op, arg.Type())
	}
	return h, nil
}

// --- array operations ---

func Sort(args ...object.Object) object.Object {
	if len(args) != 1 {
		return newError("wrong number of arguments. got=%d, want=1", len(args))
	}
	arr, errObj := requireArrayArg(BuiltinNameSort, args[0], 1)
	if errObj != nil {
		return errObj
	}
	out := make([]object.Object, len(arr.Elements))
	copy(out, arr.Elements)
	if len(out) < 2 {
		return &object.Array{Elements: out}
	}

	allNumeric, allString := true, true
	for _, el := range out {
		switch el.(type) {
		case *object.Integer, *object.Float:
			allString = false
		case *object.String:
			allNumeric = false
		default:
			allNumeric, allString = false, false
		}
	}
	switch {
	case allNumeric:
		// numericLess compares exactly. numericValue, which this called, read
		// every element as a float64, so a run of FILETIMEs a microsecond
		// apart all compared equal and sort.SliceStable left them in the order
		// they arrived -- a timeline that reports itself as sorted and is not
		// (M26-BLT-004).
		sort.SliceStable(out, func(i, j int) bool { return numericLess(out[i], out[j]) })
	case allString:
		sort.SliceStable(out, func(i, j int) bool {
			return out[i].(*object.String).Value < out[j].(*object.String).Value
		})
	default:
		return newError("sort: array must be all numbers or all strings")
	}
	return &object.Array{Elements: out}
}

func ReverseArray(args ...object.Object) object.Object {
	if len(args) != 1 {
		return newError("wrong number of arguments. got=%d, want=1", len(args))
	}
	arr, errObj := requireArrayArg(BuiltinNameReverseArray, args[0], 1)
	if errObj != nil {
		return errObj
	}
	out := make([]object.Object, len(arr.Elements))
	for i, el := range arr.Elements {
		out[len(arr.Elements)-1-i] = el
	}
	return &object.Array{Elements: out}
}

func Contains(args ...object.Object) object.Object {
	if len(args) != 2 {
		return newError("wrong number of arguments. got=%d, want=2", len(args))
	}
	arr, errObj := requireArrayArg(BuiltinNameContains, args[0], 1)
	if errObj != nil {
		return errObj
	}
	for _, el := range arr.Elements {
		if objectsEqual(el, args[1]) {
			return boolObj(true)
		}
	}
	return boolObj(false)
}

func IndexOf(args ...object.Object) object.Object {
	if len(args) != 2 {
		return newError("wrong number of arguments. got=%d, want=2", len(args))
	}
	arr, errObj := requireArrayArg(BuiltinNameIndexOf, args[0], 1)
	if errObj != nil {
		return errObj
	}
	for i, el := range arr.Elements {
		if objectsEqual(el, args[1]) {
			return intObj(int64(i))
		}
	}
	return intObj(-1)
}

func Slice(args ...object.Object) object.Object {
	if len(args) != 3 {
		return newError("wrong number of arguments. got=%d, want=3", len(args))
	}
	arr, errObj := requireArrayArg(BuiltinNameSlice, args[0], 1)
	if errObj != nil {
		return errObj
	}
	start, errObj := requireIntArg(BuiltinNameSlice, args[1], 2)
	if errObj != nil {
		return errObj
	}
	end, errObj := requireIntArg(BuiltinNameSlice, args[2], 3)
	if errObj != nil {
		return errObj
	}
	n := int64(len(arr.Elements))
	if start < 0 {
		start = 0
	}
	if end > n {
		end = n
	}
	if start > end {
		return &object.Array{Elements: []object.Object{}}
	}
	out := make([]object.Object, end-start)
	copy(out, arr.Elements[start:end])
	return &object.Array{Elements: out}
}

func Concat(args ...object.Object) object.Object {
	if len(args) != 2 {
		return newError("wrong number of arguments. got=%d, want=2", len(args))
	}
	a, errObj := requireArrayArg(BuiltinNameConcat, args[0], 1)
	if errObj != nil {
		return errObj
	}
	b, errObj := requireArrayArg(BuiltinNameConcat, args[1], 2)
	if errObj != nil {
		return errObj
	}
	out := make([]object.Object, 0, len(a.Elements)+len(b.Elements))
	out = append(out, a.Elements...)
	out = append(out, b.Elements...)
	return &object.Array{Elements: out}
}

func Flatten(args ...object.Object) object.Object {
	if len(args) != 1 {
		return newError("wrong number of arguments. got=%d, want=1", len(args))
	}
	arr, errObj := requireArrayArg(BuiltinNameFlatten, args[0], 1)
	if errObj != nil {
		return errObj
	}
	out := make([]object.Object, 0, len(arr.Elements))
	for _, el := range arr.Elements {
		if inner, ok := el.(*object.Array); ok {
			out = append(out, inner.Elements...)
		} else {
			out = append(out, el)
		}
	}
	return &object.Array{Elements: out}
}

func Unique(args ...object.Object) object.Object {
	if len(args) != 1 {
		return newError("wrong number of arguments. got=%d, want=1", len(args))
	}
	arr, errObj := requireArrayArg(BuiltinNameUnique, args[0], 1)
	if errObj != nil {
		return errObj
	}
	seen := make(map[string]struct{}, len(arr.Elements))
	out := make([]object.Object, 0, len(arr.Elements))
	var functions []object.Object
	for _, el := range arr.Elements {
		// A function has no render that identifies it: a closure renders an
		// address that changes every time the VM copies it, and an evaluator
		// function renders source text two distinct functions can share. It
		// is a duplicate only of the same function value, and an array, hash
		// or struct holding one is a duplicate only of a value equal to it
		// (M26-EVL-005).
		if object.HoldsFunction(el) {
			if !uniqueHoldsEqual(functions, el) {
				functions = append(functions, el)
				out = append(out, el)
			}
			continue
		}
		key := uniqueKey(el)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, el)
	}
	return &object.Array{Elements: out}
}

// uniqueHoldsEqual reports whether kept already holds a value equal to el.
func uniqueHoldsEqual(kept []object.Object, el object.Object) bool {
	for _, k := range kept {
		if objectsEqual(k, el) {
			return true
		}
	}
	return false
}

// uniqueKey is what `unique` files an element under: two elements are one
// element when their keys match.
//
// It used to be the element's type and its Inspect, and a render is a display
// form rather than an identity. Float.Inspect keeps six decimals, so 7.9999991
// and 7.9999994 were one element, and a string inside an array renders without
// its quotes, so ["1"] and [1] were one element too. The key spells a float
// with as many digits as it takes to tell it from every other float, and spells
// every part with a type tag and a length, so two different values cannot run
// together into one key.
//
// Nothing else about the answer moves: -0.0 and 0.0 stay two elements and every
// NaN is one, as their renders made them, and a value with no case below is
// still keyed by its type and its render.
func uniqueKey(obj object.Object) string {
	var key strings.Builder
	writeUniqueKey(&key, obj)
	return key.String()
}

func writeUniqueKey(key *strings.Builder, obj object.Object) {
	switch v := obj.(type) {
	case nil:
		key.WriteByte('z')
	case *object.Float:
		writeUniqueKeyPart(key, 'f', strconv.FormatFloat(v.Value, 'g', -1, 64))
	case *object.Integer:
		writeUniqueKeyPart(key, 'i', strconv.FormatInt(v.Value, 10))
	case *object.String:
		writeUniqueKeyPart(key, 's', v.Value)
	case *object.Array:
		writeUniqueKeyCount(key, 'a', len(v.Elements))
		for _, el := range v.Elements {
			writeUniqueKey(key, el)
		}
	case *object.Hash:
		// Each pair's key followed by its value, sorted, so the order the map
		// hands the pairs over in takes no part. Every part is self-delimiting,
		// so the sorted pairs still read back as exactly one set of pairs.
		pairs := make([]string, 0, len(v.Pairs))
		for _, pair := range v.Pairs {
			var one strings.Builder
			writeUniqueKey(&one, pair.Key)
			writeUniqueKey(&one, pair.Value)
			pairs = append(pairs, one.String())
		}
		sort.Strings(pairs)
		writeUniqueKeyCount(key, 'h', len(pairs))
		for _, pair := range pairs {
			key.WriteString(pair)
		}
	case *object.Struct:
		names := make([]string, 0, len(v.Fields))
		for name := range v.Fields {
			names = append(names, name)
		}
		sort.Strings(names)
		writeUniqueKeyPart(key, 't', v.TypeName)
		writeUniqueKeyCount(key, 'n', len(names))
		for _, name := range names {
			writeUniqueKeyPart(key, 'k', name)
			writeUniqueKey(key, v.Fields[name])
		}
	default:
		writeUniqueKeyPart(key, 'x', string(obj.Type()))
		writeUniqueKeyPart(key, 'r', obj.Inspect())
	}
}

func writeUniqueKeyPart(key *strings.Builder, tag byte, text string) {
	key.WriteByte(tag)
	key.WriteString(strconv.Itoa(len(text)))
	key.WriteByte(':')
	key.WriteString(text)
}

func writeUniqueKeyCount(key *strings.Builder, tag byte, count int) {
	key.WriteByte(tag)
	key.WriteString(strconv.Itoa(count))
	key.WriteByte(':')
}

func Range(args ...object.Object) object.Object {
	if len(args) != 2 && len(args) != 3 {
		return newError("wrong number of arguments. got=%d, want=2 or 3", len(args))
	}
	start, errObj := requireIntArg(BuiltinNameRange, args[0], 1)
	if errObj != nil {
		return errObj
	}
	end, errObj := requireIntArg(BuiltinNameRange, args[1], 2)
	if errObj != nil {
		return errObj
	}
	step := int64(1)
	if len(args) == 3 {
		step, errObj = requireIntArg(BuiltinNameRange, args[2], 3)
		if errObj != nil {
			return errObj
		}
	}
	if step == 0 {
		return newError("range: step must not be zero")
	}
	const maxRange = 10_000_000
	out := make([]object.Object, 0)
	if step > 0 {
		for i := start; i < end; i += step {
			out = append(out, intObj(i))
			if len(out) > maxRange {
				return newError("range: exceeds maximum length %d", maxRange)
			}
		}
	} else {
		for i := start; i > end; i += step {
			out = append(out, intObj(i))
			if len(out) > maxRange {
				return newError("range: exceeds maximum length %d", maxRange)
			}
		}
	}
	return &object.Array{Elements: out}
}

func Zip(args ...object.Object) object.Object {
	if len(args) != 2 {
		return newError("wrong number of arguments. got=%d, want=2", len(args))
	}
	a, errObj := requireArrayArg(BuiltinNameZip, args[0], 1)
	if errObj != nil {
		return errObj
	}
	b, errObj := requireArrayArg(BuiltinNameZip, args[1], 2)
	if errObj != nil {
		return errObj
	}
	n := len(a.Elements)
	if len(b.Elements) < n {
		n = len(b.Elements)
	}
	out := make([]object.Object, n)
	for i := 0; i < n; i++ {
		out[i] = &object.Array{Elements: []object.Object{a.Elements[i], b.Elements[i]}}
	}
	return &object.Array{Elements: out}
}

// --- hash operations ---

// hashSortedPairs is the order keys, values and entries list a hash in: the
// order printing it and a for-in loop over it use, so keys(h)[i] and
// values(h)[i] are one entry of h even when separate calls produced them, as
// long as h did not change between the calls.
//
// These three used to sort for themselves, on the rendered key alone. That put
// 10 before 2, and keys that render alike -- 1 and "1", true and "true" -- tied
// and kept whatever order the Go map was ranged in, which differs from call to
// call, so keys(h)[0] and values(h)[0] could come from different entries
// (M26-BLT-007).
func hashSortedPairs(h *object.Hash) []object.HashPair {
	return h.SortedPairs()
}

func Keys(args ...object.Object) object.Object {
	if len(args) != 1 {
		return newError("wrong number of arguments. got=%d, want=1", len(args))
	}
	h, errObj := requireHashArg(BuiltinNameKeys, args[0], 1)
	if errObj != nil {
		return errObj
	}
	pairs := hashSortedPairs(h)
	out := make([]object.Object, len(pairs))
	for i, p := range pairs {
		out[i] = p.Key
	}
	return &object.Array{Elements: out}
}

func Values(args ...object.Object) object.Object {
	if len(args) != 1 {
		return newError("wrong number of arguments. got=%d, want=1", len(args))
	}
	h, errObj := requireHashArg(BuiltinNameValues, args[0], 1)
	if errObj != nil {
		return errObj
	}
	pairs := hashSortedPairs(h)
	out := make([]object.Object, len(pairs))
	for i, p := range pairs {
		out[i] = p.Value
	}
	return &object.Array{Elements: out}
}

func Entries(args ...object.Object) object.Object {
	if len(args) != 1 {
		return newError("wrong number of arguments. got=%d, want=1", len(args))
	}
	h, errObj := requireHashArg(BuiltinNameEntries, args[0], 1)
	if errObj != nil {
		return errObj
	}
	pairs := hashSortedPairs(h)
	out := make([]object.Object, len(pairs))
	for i, p := range pairs {
		out[i] = &object.Array{Elements: []object.Object{p.Key, p.Value}}
	}
	return &object.Array{Elements: out}
}

func hashableKey(op string, key object.Object) (object.HashKey, *object.Error) {
	hashable, ok := key.(object.Hashable)
	if !ok {
		return object.HashKey{}, newError("`%s` key must be a hashable type (STRING/BYTES/INTEGER/FLOAT/BOOLEAN), got %s", op, key.Type())
	}
	return hashable.HashKey(), nil
}

func HasKey(args ...object.Object) object.Object {
	if len(args) != 2 {
		return newError("wrong number of arguments. got=%d, want=2", len(args))
	}
	h, errObj := requireHashArg(BuiltinNameHasKey, args[0], 1)
	if errObj != nil {
		return errObj
	}
	hk, errObj := hashableKey(BuiltinNameHasKey, args[1])
	if errObj != nil {
		return errObj
	}
	_, ok := h.Pairs[hk]
	return boolObj(ok)
}

func Get(args ...object.Object) object.Object {
	if len(args) != 3 {
		return newError("wrong number of arguments. got=%d, want=3 (hash, key, default)", len(args))
	}
	h, errObj := requireHashArg(BuiltinNameGet, args[0], 1)
	if errObj != nil {
		return errObj
	}
	hk, errObj := hashableKey(BuiltinNameGet, args[1])
	if errObj != nil {
		return errObj
	}
	if pair, ok := h.Pairs[hk]; ok {
		return pair.Value
	}
	return args[2]
}

func Set(args ...object.Object) object.Object {
	if len(args) != 3 {
		return newError("wrong number of arguments. got=%d, want=3 (hash, key, value)", len(args))
	}
	h, errObj := requireHashArg(BuiltinNameSet, args[0], 1)
	if errObj != nil {
		return errObj
	}
	hk, errObj := hashableKey(BuiltinNameSet, args[1])
	if errObj != nil {
		return errObj
	}
	pairs := make(map[object.HashKey]object.HashPair, len(h.Pairs)+1)
	for k, v := range h.Pairs {
		pairs[k] = v
	}
	pairs[hk] = object.HashPair{Key: args[1], Value: args[2]}
	return &object.Hash{Pairs: pairs}
}

func Merge(args ...object.Object) object.Object {
	if len(args) != 2 {
		return newError("wrong number of arguments. got=%d, want=2", len(args))
	}
	a, errObj := requireHashArg(BuiltinNameMerge, args[0], 1)
	if errObj != nil {
		return errObj
	}
	b, errObj := requireHashArg(BuiltinNameMerge, args[1], 2)
	if errObj != nil {
		return errObj
	}
	pairs := make(map[object.HashKey]object.HashPair, len(a.Pairs)+len(b.Pairs))
	for k, v := range a.Pairs {
		pairs[k] = v
	}
	for k, v := range b.Pairs {
		pairs[k] = v
	}
	return &object.Hash{Pairs: pairs}
}

func Delete(args ...object.Object) object.Object {
	if len(args) != 2 {
		return newError("wrong number of arguments. got=%d, want=2", len(args))
	}
	h, errObj := requireHashArg(BuiltinNameDelete, args[0], 1)
	if errObj != nil {
		return errObj
	}
	hk, errObj := hashableKey(BuiltinNameDelete, args[1])
	if errObj != nil {
		return errObj
	}
	pairs := make(map[object.HashKey]object.HashPair, len(h.Pairs))
	for k, v := range h.Pairs {
		if k == hk {
			continue
		}
		pairs[k] = v
	}
	return &object.Hash{Pairs: pairs}
}
