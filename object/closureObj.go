package object

type Closure struct {
	Fn   *CompiledFunction
	Free []Object

	// id is what makes this the same function value as every copy of it the
	// VM makes. See closure_identity.go.
	id *closureIdentity
}

func (c *Closure) Type() ObjectType { return CLOSURE_OBJ }

// Inspect renders the function the closure runs, by its name and parameters
// (see CompiledFunction.Inspect), never the closure's address. The VM rebuilds
// a closure every time it stores or loads one, so an address differed between
// two reads of one variable as well as between two runs (M26-EVL-002).
func (c *Closure) Inspect() string {
	if c == nil {
		return (*CompiledFunction)(nil).Inspect()
	}
	return c.Fn.Inspect()
}
