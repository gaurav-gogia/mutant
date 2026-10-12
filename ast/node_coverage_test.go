package ast

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// Three walks over this package's node types -- Clone, Modify and Equal -- and
// each is a type switch, so a node type with no arm is a silent dead end
// rather than an error: Clone shares the node between the original and the
// copy, Modify never reaches the subtree, and Equal reports two identical
// programs as different.
//
// Both coverage tests used to assert that against a hand-written list of node
// types, which is a list that cannot notice what is missing from it. By
// 2026-10-11 it was four types stale -- WhileStatement, ForInStatement,
// MatchExpression and TemplateLiteral had all been added without it -- and
// deleting any of those four from the list left every test passing
// (M26-LEX-012).
//
// So the list is read out of the package's own source now. A node type is one
// that declares expressionNode() or statementNode(), which is this package's
// own definition of a node, and adding one without a sample below fails here
// with its name. Running it that way found a gap the enumeration was for:
// MacroLiteral had no Modify arm, although it holds a body and Modify's own
// doc comment says every type that can hold an expression needs one.

// nodeSample is one node of some type, together with the node a walk over it
// has to reach. Writing the two together is the point: a sample whose marker is
// in a slot the walk does not visit tests nothing, and that is how a stale list
// goes unnoticed in the first place.
type nodeSample struct {
	node  Node
	reach Node
}

// declaredNodeTypes reads this package's source and returns the name of every
// type declaring expressionNode() or statementNode().
//
// Program is not among them: it is a Node and it is walked by all three
// functions, but it is neither a statement nor an expression, so it carries
// neither marker method. It is added by nodeSamples explicitly, which is the
// one entry that has to be written down.
func declaredNodeTypes(t *testing.T) []string {
	t.Helper()

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}

	fset := token.NewFileSet()
	names := map[string]bool{}
	scanned := 0
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, filepath.Join(".", name), nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		scanned++
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || len(fn.Recv.List) != 1 {
				continue
			}
			if fn.Name.Name != "expressionNode" && fn.Name.Name != "statementNode" {
				continue
			}
			star, ok := fn.Recv.List[0].Type.(*ast.StarExpr)
			if !ok {
				t.Fatalf("%s: %s has a non-pointer receiver, which this scan does not expect", name, fn.Name.Name)
			}
			ident, ok := star.X.(*ast.Ident)
			if !ok {
				t.Fatalf("%s: %s has a receiver this scan does not understand", name, fn.Name.Name)
			}
			names[ident.Name] = true
		}
	}
	if scanned == 0 {
		t.Fatal("scanned no files; the test is not running in the package directory")
	}
	if len(names) == 0 {
		t.Fatal("found no node types; the marker methods must have been renamed")
	}

	out := make([]string, 0, len(names))
	for name := range names {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// nodeSamples builds one node of every type, each carrying a marker whose value
// depends on marker, so that two samples of one type built with different
// markers are different programs and two built with the same marker are the
// same one.
//
// Carrying a marker is what makes this a test of the walks rather than of the
// type switches alone: an arm that exists but ignores one of its children
// passes a "does it have a case" check and fails these.
func nodeSamples(marker int64) map[string]nodeSample {
	word := "one"
	if marker != 1 {
		word = "two"
	}
	ident := func() *Identifier { return &Identifier{Value: "fixed"} }
	plain := func() *BlockStatement {
		return &BlockStatement{Statements: []Statement{&ExpressionStatement{Expression: ident()}}}
	}

	samples := map[string]nodeSample{}

	// withExpression: the type has a slot an arbitrary expression can sit in,
	// so the marker is an integer literal there and the walk has to reach it.
	withExpression := func(name string, build func(Expression) Node) {
		m := &IntegerLiteral{Value: marker}
		samples[name] = nodeSample{node: build(m), reach: m}
	}
	// withIdentifier: the type's only children are names, so the marker is one
	// of those.
	withIdentifier := func(name string, build func(*Identifier) Node) {
		m := &Identifier{Value: word}
		samples[name] = nodeSample{node: build(m), reach: m}
	}
	// itself: a leaf. There is no child to plant anything in, so what the walk
	// has to reach is the node, and a modifier is applied to every node it
	// visits including the one it started from.
	itself := func(name string, node Node) {
		samples[name] = nodeSample{node: node, reach: node}
	}

	itself("Identifier", &Identifier{Value: word})
	itself("IntegerLiteral", &IntegerLiteral{Value: marker})
	itself("FloatLiteral", &FloatLiteral{Value: float64(marker)})
	itself("StringLiteral", &StringLiteral{Value: word})
	itself("Boolean", &Boolean{Value: marker == 1})
	itself("BreakStatement", &BreakStatement{})
	itself("ContinueStatement", &ContinueStatement{})

	withIdentifier("ImportStatement", func(m *Identifier) Node {
		return &ImportStatement{Alias: m, Path: &StringLiteral{Value: "path"}}
	})
	withIdentifier("FieldExpression", func(m *Identifier) Node {
		return &FieldExpression{Left: ident(), Field: m}
	})
	withIdentifier("StructStatement", func(m *Identifier) Node {
		return &StructStatement{Name: ident(), Fields: []*Identifier{m}}
	})
	withIdentifier("EnumStatement", func(m *Identifier) Node {
		return &EnumStatement{Name: ident(), Variants: []*Identifier{m}}
	})

	withExpression("Program", func(m Expression) Node {
		return &Program{Statements: []Statement{&ExpressionStatement{Expression: m}}}
	})
	withExpression("PrefixExpression", func(m Expression) Node {
		return &PrefixExpression{Operator: "-", Right: m}
	})
	withExpression("InfixExpression", func(m Expression) Node {
		return &InfixExpression{Left: m, Operator: "+", Right: ident()}
	})
	withExpression("IndexExpression", func(m Expression) Node {
		return &IndexExpression{Left: ident(), Index: m}
	})
	withExpression("CallExpression", func(m Expression) Node {
		return &CallExpression{Function: ident(), Arguments: []Expression{m}}
	})
	withExpression("AssignExpression", func(m Expression) Node {
		return &AssignExpression{Left: ident(), Operator: "=", Value: m}
	})
	// The marker goes in the arm's body and not in its pattern, because Modify
	// walks bodies and deliberately does not walk patterns -- a pattern is a
	// literal or an enum variant by grammar, and rewriting one would leave the
	// arm with nothing to compare against. TestModifyLeavesMatchPatternsAlone
	// holds that decision in place.
	withExpression("MatchExpression", func(m Expression) Node {
		return &MatchExpression{Subject: ident(), Arms: []*MatchArm{{
			Patterns: []Expression{&IntegerLiteral{Value: 7}},
			Body:     &BlockStatement{Statements: []Statement{&ExpressionStatement{Expression: m}}},
		}}}
	})
	withExpression("IfExpression", func(m Expression) Node {
		return &IfExpression{Condition: m, Consequence: plain(), Alternative: plain()}
	})
	withExpression("FunctionLiteral", func(m Expression) Node {
		return &FunctionLiteral{
			Parameters: []*Identifier{ident()},
			Body:       &BlockStatement{Statements: []Statement{&ExpressionStatement{Expression: m}}},
		}
	})
	withExpression("MacroLiteral", func(m Expression) Node {
		return &MacroLiteral{
			Parameters: []*Identifier{ident()},
			Body:       &BlockStatement{Statements: []Statement{&ExpressionStatement{Expression: m}}},
		}
	})
	withExpression("ArrayLiteral", func(m Expression) Node {
		return &ArrayLiteral{Elements: []Expression{m}}
	})
	// In Parts and not in Texts: a text piece is a string this package stores
	// as a string, so there is no node there for a walk to visit.
	withExpression("TemplateLiteral", func(m Expression) Node {
		return &TemplateLiteral{Texts: []string{"a", ""}, Parts: []Expression{m}}
	})
	withExpression("HashLiteral", func(m Expression) Node {
		return &HashLiteral{Pairs: []HashPair{{Key: &StringLiteral{Value: "k"}, Value: m}}}
	})
	withExpression("StructLiteral", func(m Expression) Node {
		return &StructLiteral{Name: ident(), Fields: []*StructFieldValue{{Name: ident(), Value: m}}}
	})
	withExpression("BlockStatement", func(m Expression) Node {
		return &BlockStatement{Statements: []Statement{&ExpressionStatement{Expression: m}}}
	})
	withExpression("ExpressionStatement", func(m Expression) Node {
		return &ExpressionStatement{Expression: m}
	})
	withExpression("LetStatement", func(m Expression) Node {
		return &LetStatement{Name: ident(), Names: []*Identifier{ident()}, Value: m}
	})
	withExpression("ReturnStatement", func(m Expression) Node {
		return &ReturnStatement{ReturnValue: m}
	})
	withExpression("ForInStatement", func(m Expression) Node {
		return &ForInStatement{Key: ident(), Value: ident(), Iterable: m, Body: plain()}
	})
	withExpression("WhileStatement", func(m Expression) Node {
		return &WhileStatement{Condition: m, Body: plain()}
	})
	withExpression("ForStatement", func(m Expression) Node {
		return &ForStatement{
			Init:      &ExpressionStatement{Expression: m},
			Condition: ident(),
			Post:      ident(),
			Body:      plain(),
		}
	})

	return samples
}

// markerless are the two node types that cannot carry a marker at all, because
// they have no children and no value: a break is a break. They are exempt from
// the difference half of the Equal test and from nothing else.
var markerless = map[string]bool{"BreakStatement": true, "ContinueStatement": true}

// notWalkedByModify are the node types Modify does not descend into on purpose,
// with the reason, which is modify.go's own.
//
// An entry here is a decision and not a gap, and the coverage test asserts the
// walk really does stop rather than merely skipping the check -- so each line
// is a pin on the decision in both directions. The list is why the enumeration
// is worth having: without it, "Modify has no MacroLiteral arm" and "Modify has
// no arm for the node type added last week" look identical.
var notWalkedByModify = map[string]string{
	"MacroLiteral": "a macro body is a template, expanded per call site in the caller's " +
		"environment, so walking it here would expand it once at the definition against the wrong bindings",
	"ImportStatement": "an import path is resolved before macros are defined, so rewriting it " +
		"could name a file that was never read, and the alias is a binding rather than a reference a template can capture",
}

// reaches reports whether a walk over node visits target, by pointer identity,
// which is exact and needs no assumption about what the modifier may do to it.
func reaches(node, target Node) bool {
	found := false
	Modify(node, func(n Node) Node {
		if n == target {
			found = true
		}
		return n
	})
	return found
}

// TestEveryNodeTypeHasASample is the test the other three rest on, and the one
// that replaces the hand-written lists: the samples have to be exactly the node
// types the package declares, with Program added.
func TestEveryNodeTypeHasASample(t *testing.T) {
	samples := nodeSamples(1)

	declared := map[string]bool{"Program": true}
	for _, name := range declaredNodeTypes(t) {
		declared[name] = true
		if _, ok := samples[name]; !ok {
			t.Errorf("%s declares expressionNode or statementNode but nodeSamples has no entry for it, "+
				"so Clone, Modify and Equal are all unchecked for it", name)
		}
	}
	for name := range samples {
		if !declared[name] {
			t.Errorf("nodeSamples has an entry for %s, which is not a node type in this package", name)
		}
	}

	// The sample's Go type has to be the type its key names, or a copy-paste in
	// the table above would hide a type rather than cover it.
	for name, sample := range samples {
		if got := reflect.TypeOf(sample.node).Elem().Name(); got != name {
			t.Errorf("the sample keyed %s is a %s", name, got)
		}
		if sample.reach == nil {
			t.Errorf("the sample keyed %s names no node for a walk to reach", name)
		}
	}
}

// TestCloneCoversEveryNodeType replaces the hand-written version that was in
// clone_test.go. Clone returns the node as-is for a type it has no case for, so
// the original and the copy share it and a modifier run over the copy reaches
// into the original.
func TestCloneCoversEveryNodeType(t *testing.T) {
	for name, sample := range nodeSamples(1) {
		if cloned := Clone(sample.node); cloned == sample.node {
			t.Errorf("Clone returned the same %s pointer, so it has no case for it", name)
		}
	}
}

// TestModifyCoversEveryNodeType asserts the walk reaches each type's marker,
// and for the two it deliberately stops at, that it really does stop.
func TestModifyCoversEveryNodeType(t *testing.T) {
	for name, sample := range nodeSamples(1) {
		reached := reaches(sample.node, sample.reach)
		if why, deliberate := notWalkedByModify[name]; deliberate {
			if reached {
				t.Errorf("Modify descended into a %s, which it must not: %s", name, why)
			}
			continue
		}
		if !reached {
			t.Errorf("Modify never reached the %T inside a %s", sample.reach, name)
		}
	}
}

// TestModifyLeavesMatchPatternsAlone pins the one thing Modify does not walk on
// purpose, so that a later reader working from the coverage test above does not
// read the gap as an oversight and close it.
func TestModifyLeavesMatchPatternsAlone(t *testing.T) {
	pattern := &IntegerLiteral{Value: 7}
	match := &MatchExpression{
		Subject: &Identifier{Value: "x"},
		Arms:    []*MatchArm{{Patterns: []Expression{pattern}, Body: &BlockStatement{}}},
	}
	if reaches(match, pattern) {
		t.Error("Modify walked a match pattern; a pattern is a literal or an enum variant by " +
			"grammar, and rewriting one leaves the arm with nothing to compare against")
	}
}

// TestEqualCoversEveryNodeType is the same question for Equal, which has the
// opposite failure: a missing arm reports two identical programs as different,
// and an arm that ignores a child reports two different programs as the same.
// Both halves are checked for every type.
func TestEqualCoversEveryNodeType(t *testing.T) {
	first, second := nodeSamples(1), nodeSamples(1)
	other := nodeSamples(2)

	for name := range first {
		if !Equal(first[name].node, second[name].node) {
			t.Errorf("Equal says two separately built %s nodes are different, "+
				"which is what a missing arm looks like", name)
		}
		if markerless[name] {
			continue
		}
		if Equal(first[name].node, other[name].node) {
			t.Errorf("Equal says a %s is the same whatever its children are, "+
				"so its arm does not read them", name)
		}
	}
}

// And the cross-type half: no node is equal to a node of another kind. Checked
// over every pair rather than a few, because a switch arm that forgot its type
// assertion would pass a sampling.
func TestNoNodeIsEqualToAnotherKind(t *testing.T) {
	samples := nodeSamples(1)
	names := make([]string, 0, len(samples))
	for name := range samples {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, a := range names {
		for _, b := range names {
			if a == b {
				continue
			}
			if Equal(samples[a].node, samples[b].node) {
				t.Errorf("Equal says a %s and a %s are the same program", a, b)
			}
		}
	}
}
