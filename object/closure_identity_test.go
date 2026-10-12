package object

import (
	"testing"

	"mutant/ast"
)

// M26-EVL-005: a closure is the same function value as every copy Rebuilt makes
// of it, and never the same as another closure, even one over the same
// compiled function and the same captures.
func TestClosureIdentityEVL005(t *testing.T) {
	fn := &CompiledFunction{}
	free := []Object{&Cell{Value: &Integer{Value: 1}}}

	a := NewClosure(fn, free)
	copyOfA := a.Rebuilt([]Object{&Cell{Value: &Integer{Value: 1}}})
	copyOfCopy := copyOfA.Rebuilt(nil)
	b := NewClosure(fn, free)
	bare := &Closure{Fn: fn}

	for _, tt := range []struct {
		name        string
		left, right Object
		want        bool
	}{
		{"itself", a, a, true},
		{"a copy", a, copyOfA, true},
		{"a copy of a copy", copyOfCopy, a, true},
		{"another closure over the same function and captures", a, b, false},
		{"a copy of another closure", copyOfA, b.Rebuilt(free), false},
		{"a value of another type", a, &String{Value: a.Inspect()}, false},
		// A closure not made by NewClosure carries no identity, so only the
		// pointer can make it one function with another.
		{"a closure with no identity, against itself", bare, bare, true},
		{"two closures with no identity", bare, &Closure{Fn: fn}, false},
	} {
		if got := ValuesEqual(tt.left, tt.right); got != tt.want {
			t.Errorf("%s: ValuesEqual = %t, want %t", tt.name, got, tt.want)
		}
	}
}

// The evaluator's functions compare by pointer, not by their rendered source.
func TestFunctionIdentityEVL005(t *testing.T) {
	f := &Function{Body: &ast.BlockStatement{}}
	g := &Function{Body: &ast.BlockStatement{}}
	if !ValuesEqual(f, f) {
		t.Error("a function is not equal to itself")
	}
	if ValuesEqual(f, g) {
		t.Error("two functions with the same source compare equal")
	}
}

// M26-EVL-005: an array or hash holding a function is compared element by
// element, so the function inside decides by identity rather than by render.
func TestCompositeHoldingFunctionEVL005(t *testing.T) {
	fn := &CompiledFunction{}
	a := NewClosure(fn, nil)
	copyOfA := a.Rebuilt(nil)
	b := NewClosure(fn, nil)
	src := &Function{Body: &ast.BlockStatement{}}
	twin := &Function{Body: &ast.BlockStatement{}}
	key := &String{Value: "k"}
	hash := func(v Object) *Hash {
		return &Hash{Pairs: map[HashKey]HashPair{key.HashKey(): {Key: key, Value: v}}}
	}
	arr := func(els ...Object) *Array { return &Array{Elements: els} }

	for _, tt := range []struct {
		name        string
		left, right Object
		want        bool
	}{
		{"an array holding a closure, against one holding its copy", arr(a), arr(copyOfA), true},
		{"arrays holding two closures", arr(a), arr(b), false},
		{"arrays of different lengths", arr(a, &Integer{Value: 1}), arr(copyOfA), false},
		{"nested arrays holding one closure", arr(arr(a)), arr(arr(copyOfA)), true},
		{"a hash holding a closure, against one holding its copy", hash(a), hash(copyOfA), true},
		{"hashes holding two closures", hash(a), hash(b), false},
		{"a struct in an array", arr(&Struct{TypeName: "P", Fields: map[string]Object{"h": a}}),
			arr(&Struct{TypeName: "P", Fields: map[string]Object{"h": copyOfA}}), true},
		{"arrays holding one evaluator function", arr(src), arr(src), true},
		{"arrays holding two functions with one source text", arr(src), arr(twin), false},
		{"an array holding a function, against its render", arr(a), &String{Value: arr(a).Inspect()}, false},
	} {
		if got := ValuesEqual(tt.left, tt.right); got != tt.want {
			t.Errorf("%s: ValuesEqual = %t, want %t", tt.name, got, tt.want)
		}
	}

	for _, tt := range []struct {
		name string
		v    Object
		want bool
	}{
		{"a closure", a, true},
		{"an evaluator function", src, true},
		{"an array of integers", arr(&Integer{Value: 1}), false},
		{"a hash of a nested array holding a closure", hash(arr(a)), true},
		{"a struct field", &Struct{TypeName: "P", Fields: map[string]Object{"h": src}}, true},
		{"nil", nil, false},
	} {
		if got := HoldsFunction(tt.v); got != tt.want {
			t.Errorf("HoldsFunction(%s) = %t, want %t", tt.name, got, tt.want)
		}
	}
}
