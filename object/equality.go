package object

import "bytes"

// ValuesEqual reports whether two values are the same value.
//
// It is the one definition of "equal" for a value against another value, and
// deliberately not the whole of `==`. Each engine reaches `==` through its own
// comparison function, and those decide what an operand type admits before any
// of this is asked: whether `<` has a meaning, whether a type mismatch is an
// error or a false, which numeric promotions apply. What they do not each need
// to re-derive is when two values of one type hold the same thing -- and nor
// does a builtin asking that same question of an array's elements.
//
// Two rules hold throughout.
//
// A value is never equal to a value of another type. Comparing rendered forms
// is the fallback at the bottom, and it is how a string spelling `P { a: 1 }`
// came to be equal to the struct spelling it. The type guard decides, not the
// render.
//
// There is no numeric promotion here: the integer 1 is not the float 1.0. That
// is already what `contains`, `index_of` and `unique` answer, and widening it
// would silently reclassify an element a program had been told was absent. `==`
// does promote, through each engine's numeric path, which never arrives here.
//
// Arrays and hashes are compared part by part through this same function, so a
// float inside one is compared by value like a float anywhere else: [-0.0] is
// equal to [0.0], and [NaN] is not equal to itself, as NaN is not.
func ValuesEqual(a, b Object) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	if a.Type() != b.Type() {
		return false
	}

	// The Type() guard above already rules out a mismatch, so each case below
	// asserts twice over. The second assertion is what keeps a type that one
	// day shares an ObjectType with another from panicking inside `==`.
	switch av := a.(type) {
	case *Integer:
		bv, ok := b.(*Integer)
		return ok && av.Value == bv.Value
	case *Float:
		bv, ok := b.(*Float)
		return ok && av.Value == bv.Value
	case *String:
		bv, ok := b.(*String)
		return ok && av.Value == bv.Value
	case *Boolean:
		bv, ok := b.(*Boolean)
		return ok && av.Value == bv.Value
	case *Null:
		return true
	case *Bytes:
		// Contents only. The Classified mark records where a buffer came from
		// rather than what it holds, and the VM's execBytesComparison ignores
		// it for the same reason.
		bv, ok := b.(*Bytes)
		return ok && bytes.Equal(av.Value, bv.Value)
	case *Struct:
		bv, ok := b.(*Struct)
		return ok && av.Equals(bv)
	case *EnumValue:
		// A variant identifies itself by type and tag. The payload is the
		// ordinal, which both engines already leave out of the comparison.
		bv, ok := b.(*EnumValue)
		return ok && av.TypeName == bv.TypeName && av.Tag == bv.Tag
	case *Error:
		bv, ok := b.(*Error)
		return ok && av.Equals(bv)
	case *Closure:
		// A function is equal to itself and to nothing else. Its render
		// cannot say which function it is -- a closure renders an address,
		// and the VM copies a closure on every store and load -- so the
		// identity the copies share decides (M26-EVL-005).
		bv, ok := b.(*Closure)
		return ok && av.SameFunction(bv)
	case *Function:
		// The evaluator's function renders as its source text, which two
		// distinct functions can share. The evaluator never copies a function
		// value, so the pointer is its identity.
		bv, ok := b.(*Function)
		return ok && av == bv
	case *Array:
		// Element by element. Arrays used to be compared by their renders, and
		// a render is a display form rather than an identity: Float.Inspect
		// keeps six decimals, so [7.9999991] rendered exactly as [7.9999994],
		// and a string renders without its quotes, so ["1"] rendered as [1].
		bv, ok := b.(*Array)
		if !ok || len(av.Elements) != len(bv.Elements) {
			return false
		}
		for i := range av.Elements {
			if !ValuesEqual(av.Elements[i], bv.Elements[i]) {
				return false
			}
		}
		return true
	case *Hash:
		// The same keys, each holding an equal value. A key is one entry by its
		// HashKey, which carries every bit of a float key, so two float keys
		// that render alike are still two keys here.
		bv, ok := b.(*Hash)
		if !ok || len(av.Pairs) != len(bv.Pairs) {
			return false
		}
		for key, pair := range av.Pairs {
			other, present := bv.Pairs[key]
			if !present || !ValuesEqual(pair.Value, other.Value) {
				return false
			}
		}
		return true
	default:
		// A builtin is a function value, and every builtin renders as the one
		// constant "builtin function", so its render cannot tell two apart.
		// Its pointer can: the builtin table holds one per builtin, both
		// engines hand out that pointer, and storage passes a builtin through
		// unchanged (mutil encryptObject/decryptObject). Its type lives in
		// package builtin, which imports this one, so it is known here by its
		// ObjectType alone.
		if a.Type() == BUILTIN_OBJ {
			return a == b
		}
		// Whatever else has no case above -- iterators and the like -- is
		// still compared by its rendered form.
		return a.Inspect() == b.Inspect()
	}
}
