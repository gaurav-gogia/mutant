package ast

import "reflect"

// Equal reports whether two nodes are the same program: the same node kinds in
// the same places, with the same operators, names and literal values, and the
// same children in the same order.
//
// It exists because String() cannot answer that question and was being asked
// it. String() is a debug rendering, and a lossy one by design: a block prints
// its statements with no braces around them, a string literal prints its
// spelling, and `${...}` holes print without their delimiters. So distinct
// programs render to one string -- `fn() { a; }` and `fn() a;` are the same
// text, and so are the string "x" and the name x -- which means a test that
// compared renderings could watch the formatter rewrite a program into a
// different one and see nothing (M26-LEX-011).
//
// Positions are ignored, and so is spelling. Two literals with the same value
// are equal however they were written, because `"a"`, `r"a"` and `"""a"""` are
// one value and a formatter is allowed to prefer one of them; what it is not
// allowed to do is change the value, lose a statement, or re-associate an
// operator, and each of those is a difference here.
//
// The side tables on Program -- positions, comments, macro origins and the
// parenthesisation record -- are not structure and are not compared. Dropping a
// redundant pair of parentheses is a thing the formatter does on purpose.
func Equal(a, b Node) bool {
	if isNilNode(a) || isNilNode(b) {
		return isNilNode(a) && isNilNode(b)
	}

	switch x := a.(type) {
	case *Program:
		y, ok := b.(*Program)
		return ok && equalNodes(x.Statements, y.Statements)

	case *Identifier:
		y, ok := b.(*Identifier)
		return ok && x.Value == y.Value

	case *IntegerLiteral:
		y, ok := b.(*IntegerLiteral)
		return ok && x.Value == y.Value

	case *FloatLiteral:
		y, ok := b.(*FloatLiteral)
		return ok && x.Value == y.Value

	case *StringLiteral:
		y, ok := b.(*StringLiteral)
		return ok && x.Value == y.Value

	case *Boolean:
		y, ok := b.(*Boolean)
		return ok && x.Value == y.Value

	case *BreakStatement:
		// A break carries nothing but its token, so being a break is the whole
		// of what there is to compare.
		_, ok := b.(*BreakStatement)
		return ok

	case *ContinueStatement:
		_, ok := b.(*ContinueStatement)
		return ok

	case *ImportStatement:
		y, ok := b.(*ImportStatement)
		return ok && Equal(x.Alias, y.Alias) && Equal(x.Path, y.Path)

	case *PrefixExpression:
		y, ok := b.(*PrefixExpression)
		return ok && x.Operator == y.Operator && Equal(x.Right, y.Right)

	case *InfixExpression:
		y, ok := b.(*InfixExpression)
		return ok && x.Operator == y.Operator &&
			Equal(x.Left, y.Left) && Equal(x.Right, y.Right)

	case *IndexExpression:
		y, ok := b.(*IndexExpression)
		return ok && Equal(x.Left, y.Left) && Equal(x.Index, y.Index)

	case *CallExpression:
		y, ok := b.(*CallExpression)
		return ok && Equal(x.Function, y.Function) &&
			equalNodes(x.Arguments, y.Arguments)

	case *AssignExpression:
		y, ok := b.(*AssignExpression)
		return ok && x.Operator == y.Operator && x.Postfix == y.Postfix &&
			Equal(x.Left, y.Left) && Equal(x.Value, y.Value)

	case *FieldExpression:
		y, ok := b.(*FieldExpression)
		return ok && Equal(x.Left, y.Left) && Equal(x.Field, y.Field)

	case *MatchExpression:
		y, ok := b.(*MatchExpression)
		if !ok || !Equal(x.Subject, y.Subject) || len(x.Arms) != len(y.Arms) {
			return false
		}
		for i := range x.Arms {
			if !equalArm(x.Arms[i], y.Arms[i]) {
				return false
			}
		}
		return true

	case *IfExpression:
		y, ok := b.(*IfExpression)
		return ok && Equal(x.Condition, y.Condition) &&
			Equal(x.Consequence, y.Consequence) && Equal(x.Alternative, y.Alternative)

	case *FunctionLiteral:
		y, ok := b.(*FunctionLiteral)
		return ok && x.Name == y.Name &&
			equalNodes(x.Parameters, y.Parameters) && Equal(x.Body, y.Body)

	case *MacroLiteral:
		y, ok := b.(*MacroLiteral)
		return ok && equalNodes(x.Parameters, y.Parameters) && Equal(x.Body, y.Body)

	case *ArrayLiteral:
		y, ok := b.(*ArrayLiteral)
		return ok && equalNodes(x.Elements, y.Elements)

	case *TemplateLiteral:
		y, ok := b.(*TemplateLiteral)
		if !ok || len(x.Texts) != len(y.Texts) {
			return false
		}
		for i := range x.Texts {
			if x.Texts[i] != y.Texts[i] {
				return false
			}
		}
		return equalNodes(x.Parts, y.Parts)

	case *HashLiteral:
		y, ok := b.(*HashLiteral)
		if !ok || len(x.Pairs) != len(y.Pairs) {
			return false
		}
		// Compared in the order written. A hash's meaning does not depend on
		// that order, but nothing in this tree reorders one, so requiring the
		// order is the stricter reading and a reordering would be reported
		// rather than absorbed.
		for i := range x.Pairs {
			if !Equal(x.Pairs[i].Key, y.Pairs[i].Key) ||
				!Equal(x.Pairs[i].Value, y.Pairs[i].Value) {
				return false
			}
		}
		return true

	case *StructLiteral:
		y, ok := b.(*StructLiteral)
		if !ok || !Equal(x.Name, y.Name) || len(x.Fields) != len(y.Fields) {
			return false
		}
		for i := range x.Fields {
			if !equalFieldValue(x.Fields[i], y.Fields[i]) {
				return false
			}
		}
		return true

	case *BlockStatement:
		y, ok := b.(*BlockStatement)
		return ok && equalNodes(x.Statements, y.Statements)

	case *ExpressionStatement:
		y, ok := b.(*ExpressionStatement)
		return ok && Equal(x.Expression, y.Expression)

	case *LetStatement:
		y, ok := b.(*LetStatement)
		// Name and Names are both compared: a single binding and a one-element
		// destructuring are not the same statement.
		return ok && Equal(x.Name, y.Name) &&
			equalNodes(x.Names, y.Names) && Equal(x.Value, y.Value)

	case *ReturnStatement:
		y, ok := b.(*ReturnStatement)
		return ok && Equal(x.ReturnValue, y.ReturnValue) &&
			equalNodes(x.ReturnValues, y.ReturnValues)

	case *ForInStatement:
		y, ok := b.(*ForInStatement)
		return ok && Equal(x.Key, y.Key) && Equal(x.Value, y.Value) &&
			Equal(x.Iterable, y.Iterable) && Equal(x.Body, y.Body)

	case *WhileStatement:
		y, ok := b.(*WhileStatement)
		return ok && Equal(x.Condition, y.Condition) && Equal(x.Body, y.Body)

	case *ForStatement:
		y, ok := b.(*ForStatement)
		return ok && Equal(x.Init, y.Init) && Equal(x.Condition, y.Condition) &&
			Equal(x.Post, y.Post) && Equal(x.Body, y.Body)

	case *StructStatement:
		y, ok := b.(*StructStatement)
		return ok && Equal(x.Name, y.Name) && equalNodes(x.Fields, y.Fields)

	case *EnumStatement:
		y, ok := b.(*EnumStatement)
		return ok && Equal(x.Name, y.Name) && equalNodes(x.Variants, y.Variants)
	}

	// An unhandled node kind is reported as unequal rather than as equal, so a
	// node type added without an arm here makes a comparison fail instead of
	// quietly succeeding. TestEqualHandlesEveryNodeType enumerates the package
	// and names the missing arm outright, which is the check that matters; this
	// is what happens if that test is ever deleted.
	return false
}

// equalArm compares one match arm. Braced is not compared: it records whether
// the author wrote the body in braces, and the body itself is a BlockStatement
// either way, so the statements are compared regardless of how they were
// punctuated.
func equalArm(a, b *MatchArm) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return equalNodes(a.Patterns, b.Patterns) && Equal(a.Body, b.Body)
}

func equalFieldValue(a, b *StructFieldValue) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return Equal(a.Name, b.Name) && Equal(a.Value, b.Value)
}

// equalNodes compares two slices of nodes pairwise, in order. The type
// parameter takes []Statement, []Expression and []*Identifier alike, which is
// what keeps the arms above to one line each.
func equalNodes[T Node](a, b []T) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !Equal(a[i], b[i]) {
			return false
		}
	}
	return true
}

// isNilNode reports whether n holds nothing, counting both an empty interface
// and an interface holding a nil pointer.
//
// The second is the one that matters and the one that cannot be written without
// reflection: an absent `else` is a nil *BlockStatement stored in a Node, which
// is not equal to nil as an interface, so a plain n == nil would say an absent
// else branch and a present one are different kinds of absent and then
// dereference it.
func isNilNode(n Node) bool {
	if n == nil {
		return true
	}
	value := reflect.ValueOf(n)
	return value.Kind() == reflect.Ptr && value.IsNil()
}
