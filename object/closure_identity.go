package object

// closureIdentity is what makes a closure the same function value after it has
// been copied.
//
// The VM never holds one closure for long: EncryptObject and DecryptObject
// rebuild it on every push, pop and store, so the pointer a program read a
// moment ago is not the one it reads now. The identity is the part every copy
// carries over, and it is made exactly once, when OpClosure makes the function
// value.
//
// It holds a byte so that two identities are two allocations: Go allows
// pointers to distinct zero-size values to compare equal.
type closureIdentity struct{ _ byte }

// NewClosure makes a new function value: equal to itself and to every copy of
// itself, and to no other closure, whatever its Fn and Free.
func NewClosure(fn *CompiledFunction, free []Object) *Closure {
	return &Closure{Fn: fn, Free: free, id: &closureIdentity{}}
}

// Rebuilt returns a closure over free that is the same function value as c.
// It is how a copy is made: storage rebuilding the captures, or a worker
// detaching them.
func (c *Closure) Rebuilt(free []Object) *Closure {
	return &Closure{Fn: c.Fn, Free: free, id: c.id}
}

// SameFunction reports whether c and other are one function value.
//
// A closure not made by NewClosure has no identity to share, so it is the same
// function only as itself: pointer equality, never "equal because it looks the
// same".
func (c *Closure) SameFunction(other *Closure) bool {
	if c == other {
		return true
	}
	if c == nil || other == nil {
		return false
	}
	return c.id != nil && c.id == other.id
}

// HoldsFunction reports whether o is a function value or holds one anywhere
// inside it: an element of an array, a value of a hash, a field of a struct or
// a related value of an error, at any depth. A builtin is a function value
// too. A hash key is never a function, so keys are not searched.
//
// It decides whether a value may be compared by its render. A function's
// render cannot say which function it is -- a closure renders an address that
// changes with every copy the VM makes, and an evaluator function renders
// source text two distinct functions can share -- so neither can the render of
// anything that holds one. Every builtin renders as the same constant, which
// says even less.
func HoldsFunction(o Object) bool {
	switch v := o.(type) {
	case nil:
		return false
	case *Closure, *Function:
		return true
	case *Array:
		for _, el := range v.Elements {
			if HoldsFunction(el) {
				return true
			}
		}
	case *Hash:
		for _, pair := range v.Pairs {
			if HoldsFunction(pair.Value) {
				return true
			}
		}
	case *Struct:
		for _, field := range v.Fields {
			if HoldsFunction(field) {
				return true
			}
		}
	case *Error:
		// Error.Equals compares related values with ValuesEqual, and an
		// error renders each related value through its own Inspect, so an
		// error holding a function is no more identified by its render than
		// an array holding one (M26-EVL-002).
		for _, related := range v.Related {
			if HoldsFunction(related) {
				return true
			}
		}
	default:
		// A builtin. Its type lives in package builtin, which imports this
		// one, so it is known here by its ObjectType.
		return v.Type() == BUILTIN_OBJ
	}
	return false
}
