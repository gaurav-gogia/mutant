package evaluator

import (
	"mutant/ast"
	"mutant/object"
)

// A container is a value. Binding one to a second name, passing it to a
// function, storing it in another container or walking it with a loop hands
// over a copy, so a write through one name is never seen through another:
//
//	let a = [1, 2];
//	let b = a;
//	b[0] = 9;        // a is still [1, 2]
//
// That is the VM's rule, and the VM is what every program runs on: it seals a
// value on every store and opens a fresh copy of it on every load
// (mutil.DecryptObject rebuilds each array, hash, struct, buffer and enum
// payload it opens). This engine used to share the container instead, so the
// same lines gave a different answer inside a macro body than they give when
// they run (M26-VM-019).
//
// The copy is made in the same place the VM makes it -- where a variable is
// read (evalIdentifier) -- and never where one is written. A write goes through
// evalTarget, which reaches the stored container itself, so `a[0] = 9` and
// `grid[0][1] = 9` still land in the variable they name. A closure shares its
// enclosing variables, not copies of them, because it reads them through the
// same environment; that is the by-reference capture the language reference
// describes, and the VM's cells give the same answer.

// copyValue returns a deep copy of v where v can be changed in place through an
// index or a field, and v itself otherwise.
//
// Strings, numbers, booleans and null cannot be changed in place, so sharing
// them is not observable. A function is shared deliberately: its environment is
// its captured variables, which are shared by reference in both engines. An
// error is shared because nothing in the language writes into one.
func copyValue(v object.Object) object.Object {
	switch c := v.(type) {
	case *object.Array:
		elements := make([]object.Object, len(c.Elements))
		for i, element := range c.Elements {
			elements[i] = copyValue(element)
		}
		return &object.Array{Elements: elements}

	case *object.Hash:
		pairs := make(map[object.HashKey]object.HashPair, len(c.Pairs))
		for hashKey, pair := range c.Pairs {
			// The key is copied too: a buffer is a valid key, and a buffer can
			// be changed in place.
			pairs[hashKey] = object.HashPair{Key: copyValue(pair.Key), Value: copyValue(pair.Value)}
		}
		return &object.Hash{Pairs: pairs}

	case *object.Struct:
		fields := make(map[string]object.Object, len(c.Fields))
		for name, value := range c.Fields {
			fields[name] = copyValue(value)
		}
		return &object.Struct{
			TypeName:   c.TypeName,
			FieldOrder: append([]string(nil), c.FieldOrder...),
			Fields:     fields,
		}

	case *object.Bytes:
		// The classification travels with the bytes, as it does when the VM
		// opens a sealed buffer.
		return &object.Bytes{Value: append([]byte{}, c.Value...), Classified: c.Classified}

	case *object.EnumValue:
		var payload object.Object
		if c.Value != nil {
			payload = copyValue(c.Value)
		}
		return &object.EnumValue{TypeName: c.TypeName, Tag: c.Tag, Value: payload}

	case *object.MultiValue:
		values := make([]object.Object, len(c.Values))
		for i, value := range c.Values {
			values[i] = copyValue(value)
		}
		return &object.MultiValue{Values: values}
	}
	return v
}

// evalTarget evaluates the container an assignment writes into, reaching the
// stored value rather than the copy a read would make: a variable, then any
// number of index and field hops below it. Each hop's index is evaluated once,
// as it is on the read path.
//
// Anything else -- a literal, a call -- is evaluated as a read, because what it
// produces is already a value nothing else holds.
func evalTarget(n ast.Node, env *object.Environment) object.Object {
	switch node := n.(type) {
	case *ast.Identifier:
		if val, ok := env.Get(node.Value); ok {
			return val
		}
		// A builtin's name, or a name that is not bound at all: the read path
		// gives the answer, and neither is storage a write could reach.
		return evalIdentifier(node, env)

	case *ast.IndexExpression:
		left := evalTarget(node.Left, env)
		if isSignal(left) {
			return left
		}
		index := eval(node.Index, env)
		if isSignal(index) {
			return index
		}
		return evalIndexExpression(left, index)

	case *ast.FieldExpression:
		// `Colour.Red` and a builtin fold such as `fs.write` are not a field of
		// anything bound to the name on the left; evalFieldExpression asks sema
		// which one this is, and there is no stored container under either.
		if ident, ok := node.Left.(*ast.Identifier); ok {
			_, isEnum := env.Get(enumDefinitionKey(ident.Value))
			_, bound := env.Get(ident.Value)
			if isEnum || !bound {
				return evalFieldExpression(node, env)
			}
		}
		left := evalTarget(node.Left, env)
		if isSignal(left) {
			return left
		}
		// The same three answers evalFieldExpression gives once it has the
		// value on the left: a struct's field, an error's field (null when the
		// error does not carry it), or a refusal.
		switch value := left.(type) {
		case *object.Struct:
			if field, ok := value.Fields[node.Field.Value]; ok {
				return field
			}
			return newError("%s", object.UnknownStructFieldMessage(
				value.TypeName, node.Field.Value, value.FieldNames()))
		case *object.Error:
			if field, ok := value.Field(node.Field.Value); ok {
				return field
			}
			return NULL
		}
		return newError("cannot access field %s on type %s", node.Field.Value, left.Type())
	}
	return eval(n, env)
}
