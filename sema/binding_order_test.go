package sema

import (
	"testing"

	"mutant/ast"
	"mutant/lexer"
	"mutant/parser"
)

// The compiler compiles some things written after a name before it defines
// the name: a for-in's iterable before the loop's bindings
// (compileForInStatement), and a let's value before the let's names
// (M26-CMP-001), except that a function literal sees its own name through
// DefineFunctionName. The graph has to agree in the walk, which decides what
// each reference resolves to, and in the position queries, LocalScopeAt and
// VisibleAt, which the builtin-call rules, signature help and completion read.
// It used to declare first in both statements: M26-SEM-007 for the for-in,
// M26-SEM-021 for the let.

func TestAForInIterableResolvesToTheBindingBeforeTheLoop(t *testing.T) {
	// line 2: `for (xs in xs) { xs; }` -- binding at 2:6, iterable at 2:12,
	// body use at 2:18.
	g := graphOf(t, "let xs = [1];\nfor (xs in xs) { xs; }\n")

	var outer, loop *Node
	for _, node := range g.Declarations() {
		if node.Name != "xs" {
			continue
		}
		switch node.Kind {
		case KindLoopBind:
			loop = node
		default:
			outer = node
		}
	}
	if outer == nil || loop == nil {
		t.Fatalf("want an outer let and a loop binding named xs, got %v", declaredNames(g))
	}

	ref, found := g.ReferenceAt(2, 12)
	if !found {
		t.Fatalf("the iterable xs at 2:12 records no reference at all")
	}
	if ref.Target != outer.ID {
		target, _ := g.NodeFor(ref.Target)
		t.Errorf("the iterable xs at 2:12 resolves to %s declared at %d:%d; the compiler "+
			"compiles the iterable before defining the binding, so it means the let on line 1",
			target.Kind, target.DeclRange.Start.Line, target.DeclRange.Start.Column)
	}
	if uses := g.UsesOf(outer.ID); len(uses) != 1 {
		t.Errorf("the outer let is read once, by the iterable; UsesOf reports %d uses %v", len(uses), uses)
	}
	if uses := g.UsesOf(loop.ID); len(uses) != 1 || uses[0].Start.Column != 18 {
		t.Errorf("the loop binding is read once, in the body at 2:18; got %v", uses)
	}
}

func TestAForInIterableNamingItsOwnBindingIsUnbound(t *testing.T) {
	// `for (y in y) { }`: the compiler refuses with "undefined variable: y".
	g := graphOf(t, "for (y in y) { }\n")
	for _, miss := range g.UnboundUses() {
		if miss.Name == "y" && miss.UseRange.Start.Line == 1 && miss.UseRange.Start.Column == 11 {
			return
		}
	}
	t.Fatalf("the iterable y at 1:11 is not recorded as unbound (unbound = %v, refs = %d); "+
		"the compiler refuses this program with `undefined variable: y`",
		unboundNames(g), len(g.References()))
}

func TestAForInBindingIsNotVisibleInsideItsIterable(t *testing.T) {
	// `for (keys in keys({"a": 1})) { keys; }` -- the iterable's keys is at
	// 1:14 and is the builtin to the compiler, whose symbol table does not yet
	// hold the loop binding there. The body's is at 1:32.
	src := "for (keys in keys({\"a\": 1})) { keys; }\n"
	g := graphOf(t, src)

	if g.LocalScopeAt(1, 14).Bound("keys") {
		t.Errorf("LocalScopeAt(1:14).Bound(keys) = true inside the iterable; the compiler's " +
			"answer is false there, and the editor then stops treating keys(...) as the builtin call it is")
	}
	for _, node := range g.VisibleAt(1, 14) {
		if node.Name == "keys" {
			t.Errorf("VisibleAt(1:14) offers the loop binding %q (%s) inside the iterable", node.Name, node.Kind)
		}
	}
	if !g.LocalScopeAt(1, 32).Bound("keys") {
		t.Errorf("control: inside the body the loop binding must be bound")
	}
}

func TestALetValueReadsTheNameFromBeforeTheLet(t *testing.T) {
	// line 2: `if (true) { let w = w + 1; w; }` -- the value's w at 2:21 is the
	// OUTER w to the compiler: the program prints 2 inside and 1 outside.
	g := graphOf(t, "let w = 1;\nif (true) { let w = w + 1; w; }\n")
	ref, found := g.ReferenceAt(2, 21)
	if !found {
		t.Fatalf("no reference recorded at 2:21")
	}
	target, _ := g.NodeFor(ref.Target)
	if target.DeclRange.Start.Line != 1 {
		t.Errorf("the value's w at 2:21 resolves to the declaration at %d:%d; the compiler "+
			"compiles the value before declaring the inner w, so it is the w on line 1",
			target.DeclRange.Start.Line, target.DeclRange.Start.Column)
	}
	if !g.LocalScopeAt(2, 28).Bound("w") {
		t.Errorf("control: after the let, w must be bound")
	}
}

func TestALetValueNamingItsOwnLetIsUnbound(t *testing.T) {
	// `let z = z + 1;` -- the compiler refuses: undefined variable: z.
	g := graphOf(t, "let z = z + 1;\n")
	for _, miss := range g.UnboundUses() {
		if miss.Name == "z" {
			return
		}
	}
	t.Errorf("let z = z + 1 records no unbound use of z (unbound = %v); the build refuses it "+
		"with `undefined variable: z`", unboundNames(g))
}

func TestAFunctionLiteralStillSeesItsOwnLet(t *testing.T) {
	// The exception: the compiler gives a function literal its own name, so
	// recursion resolves to the let.
	g := graphOf(t, "let down = fn(n) { return down(n); };\n")
	if names := unboundNames(g); len(names) != 0 {
		t.Fatalf("a recursive call records unbound uses %v; the literal sees its own name", names)
	}
	var let *Node
	for _, node := range g.Declarations() {
		if node.Name == "down" {
			let = node
		}
	}
	if let == nil {
		t.Fatalf("no declaration of down, got %v", declaredNames(g))
	}
	if uses := g.UsesOf(let.ID); len(uses) != 1 {
		t.Errorf("the recursive call is the let's one use; UsesOf reports %v", uses)
	}
}

// A let's names are visible strictly after the End of its value, never at it.
// The End is exclusive -- the position right after the value's last character
// -- and it is exactly where an editor's cursor sits while that character is
// being typed. The keys that completion finishes at `let keys = ke|` is the
// builtin to the compiler, so completion and hover, which ask LocalScopeAt and
// VisibleAt at the raw cursor, must not see the let there. One column further
// on, or on a later line, they must. A for-in needs no such care, and keeps
// seeing its bindings from the body's opening brace: the header's `)` always
// stands between the iterable's End and the body.
func TestADeclarationIsNotVisibleAtTheEndOfWhatIsCompiledBeforeIt(t *testing.T) {
	type position struct{ line, column int }
	for _, c := range []struct {
		name  string
		src   string
		names []string
		end   position // the End of the value, or of the iterable
		seen  position // the first position at which the names are visible
		use   position // a use of names[0], which the walk resolves to it
	}{
		{
			name:  "a value being typed, the cursor right after it",
			src:   "let keys = ke\nkeys;\n",
			names: []string{"keys"},
			end:   position{1, 14}, seen: position{2, 1}, use: position{2, 1},
		},
		{
			name:  "a use written straight after the semicolon",
			src:   "let x = 1;x;\n",
			names: []string{"x"},
			end:   position{1, 10}, seen: position{1, 11}, use: position{1, 11},
		},
		{
			name:  "a value over two lines",
			src:   "let len = 1 +\n  le;len;\n",
			names: []string{"len"},
			end:   position{2, 5}, seen: position{2, 6}, use: position{2, 6},
		},
		{
			name:  "a multi-name let",
			src:   "let a, b = f();a;\n",
			names: []string{"a", "b"},
			end:   position{1, 15}, seen: position{1, 16}, use: position{1, 16},
		},
		{
			name:  "a for-in, visible from its body's opening brace",
			src:   "for (keys in ke) {keys;}\n",
			names: []string{"keys"},
			end:   position{1, 16}, seen: position{1, 18}, use: position{1, 19},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			g := graphOf(t, c.src)
			visible := func(at position, name string) bool {
				for _, node := range g.VisibleAt(at.line, at.column) {
					if node.Name == name {
						return true
					}
				}
				return false
			}
			for _, name := range c.names {
				if g.LocalScopeAt(c.end.line, c.end.column).Bound(name) {
					t.Errorf("LocalScopeAt(%d:%d).Bound(%s) = true at the End of what is compiled "+
						"before %s is declared; a cursor there is still typing it",
						c.end.line, c.end.column, name, name)
				}
				if visible(c.end, name) {
					t.Errorf("VisibleAt(%d:%d) offers %s at the End of what is compiled before it is declared",
						c.end.line, c.end.column, name)
				}
				if node, found := g.Resolve(c.end.line, c.end.column); found && node.Name == name {
					t.Errorf("Resolve(%d:%d) names the declaration of %s at %d:%d at the End of what is "+
						"compiled before it", c.end.line, c.end.column, name,
						node.DeclRange.Start.Line, node.DeclRange.Start.Column)
				}
				if !g.LocalScopeAt(c.seen.line, c.seen.column).Bound(name) {
					t.Errorf("LocalScopeAt(%d:%d).Bound(%s) = false; %s is declared by then",
						c.seen.line, c.seen.column, name, name)
				}
				if !visible(c.seen, name) {
					t.Errorf("VisibleAt(%d:%d) does not offer %s; it is declared by then",
						c.seen.line, c.seen.column, name)
				}
			}
			declared := find(t, g, c.names[0])
			if node, found := g.Resolve(c.use.line, c.use.column); !found || node.ID != declared.ID {
				gotLine, gotColumn := 0, 0
				if found {
					gotLine, gotColumn = node.DeclRange.Start.Line, node.DeclRange.Start.Column
				}
				t.Errorf("Resolve(%d:%d) names the declaration at %d:%d (found=%t), want %s declared at %d:%d",
					c.use.line, c.use.column, gotLine, gotColumn, found, c.names[0],
					declared.DeclRange.Start.Line, declared.DeclRange.Start.Column)
			}
		})
	}
}

func TestResolveAndVisibleAtAgreeAtTheEndOfAValue(t *testing.T) {
	// line 2: `let w = w;w;` -- the value's w at 2:9 with its End at 2:10, and
	// a use after the semicolon at 2:11. At 2:10 the cursor is still on the
	// value's w, which is the w of line 1: the walk resolved it before
	// declaring the w of line 2, and VisibleAt must not offer that one in its
	// place. At 2:11 both name the w of line 2.
	g := graphOf(t, "let w = 1;\nlet w = w;w;\n")
	for _, c := range []struct {
		line, column int
		wantLine     int // the line of the declaration both must name
	}{
		{2, 10, 1},
		{2, 11, 2},
	} {
		resolvedLine := 0
		if resolved, found := g.Resolve(c.line, c.column); found {
			resolvedLine = resolved.DeclRange.Start.Line
		}
		if resolvedLine != c.wantLine {
			t.Errorf("Resolve(%d:%d) names the w declared on line %d (0: nothing), want line %d",
				c.line, c.column, resolvedLine, c.wantLine)
		}
		visibleLine := 0
		for _, node := range g.VisibleAt(c.line, c.column) {
			if node.Name == "w" {
				visibleLine = node.DeclRange.Start.Line
				break
			}
		}
		if visibleLine != c.wantLine {
			t.Errorf("VisibleAt(%d:%d) offers the w declared on line %d (0: none), want line %d",
				c.line, c.column, visibleLine, c.wantLine)
		}
	}
}

func TestEveryNameOfAMultiNameLetIsAValue(t *testing.T) {
	// Walking a single-name let's value first sent it down the path a
	// multi-name let takes, and that path must still not hand every name the
	// value's kind. destructureValues puts a value that is neither an array
	// nor a multi-value whole into the first name and null into the rest, so
	// below a holds the closure and b holds null: a function was false of b.
	// A value is true of both, a included, because a function is a value too,
	// and it is all the graph claims of either.
	// TestADestructuredNameIsAFunctionOnlyIfItHoldsOne checks what each name
	// holds against the engines.
	g := graphOf(t, "let a, b = fn(x) { return x; };\nlet f = fn(x) { return x; };\n")
	for _, name := range []string{"a", "b"} {
		if node := find(t, g, name); node.Kind != KindValue {
			t.Errorf("%s of a multi-name let is a %s, want a value", name, node.Kind)
		}
	}
	if node := find(t, g, "f"); node.Kind != KindFunction {
		t.Errorf("control: the single-name let of a function literal is a %s, want a function", node.Kind)
	}
}

// TestAPairLetsFunctionCallsItselfByItsFirstName: the parser names a function
// literal after the first name of the let that holds it, a multi-name let
// included, and DefineFunctionName binds that name inside the body, so the
// call on line 1 recurses in the build. The walk bound none of a multi-name
// let's names before the value, so the call was unbound (M26-SEM-030). The
// other names still bind after the value, and every name stays a value.
func TestAPairLetsFunctionCallsItselfByItsFirstName(t *testing.T) {
	// line 1: `let a, b = fn(n) { if (n > 0) { return a(n - 1); } return b; };`
	// -- the self-call's a at 1:40, the b read inside the body at 1:59.
	g := graphOf(t, "let a, b = fn(n) { if (n > 0) { return a(n - 1); } return b; };\na(3);\n")
	a, b := find(t, g, "a"), find(t, g, "b")

	if node, found := g.Resolve(1, 40); !found || node.ID != a.ID {
		t.Errorf("the call of a inside its own literal resolves to %v (found=%t), want the let's a", node, found)
	}
	if node, found := g.Resolve(1, 59); found && node.ID == b.ID {
		t.Errorf("b inside the literal resolves to the let's b, which the compiler declares only after the value")
	}
	if node, found := g.Resolve(2, 1); !found || node.ID != a.ID {
		t.Errorf("a on line 2 resolves to %v (found=%t), want the let's a", node, found)
	}
	if a.Kind != KindValue || b.Kind != KindValue {
		t.Errorf("a is a %s and b a %s; every name of a multi-name let is a value", a.Kind, b.Kind)
	}
	if !a.Grouped || !b.Grouped {
		t.Errorf("a grouped=%t, b grouped=%t; both come from one multi-name let", a.Grouped, b.Grouped)
	}
}

// TestALetWhoseValueDidNotParseIsSeenOnlyAfterItsStatement: while a value is
// half typed the parser can drop it, leaving the let with no value to be seen
// after. Its names were then seen from the name on, so at the cursor typing
// `let keys = (ke` the let already bound keys, and completion hid the
// builtin the compiler would read there. The statement's End, which is the
// cursor, stands in for the value's.
func TestALetWhoseValueDidNotParseIsSeenOnlyAfterItsStatement(t *testing.T) {
	for _, src := range []string{"let keys = (ke", "let keys = if (ke", "let keys = "} {
		t.Run(src, func(t *testing.T) {
			p := parser.New(lexer.New(src))
			program := p.ParseProgram()
			if len(p.Errors()) == 0 {
				t.Fatalf("%q parses; this test needs a value the parser drops", src)
			}
			if len(program.Statements) != 1 {
				t.Fatalf("%q parses to %d statements, want the one let", src, len(program.Statements))
			}
			let, isLet := program.Statements[0].(*ast.LetStatement)
			if !isLet || let.Value != nil {
				t.Fatalf("%q does not parse to a let with its value dropped: %T", src, program.Statements[0])
			}

			g := BuildFile("", program, nil, nil)
			find(t, g, "keys")
			column := len(src) + 1
			if g.LocalScopeAt(1, column).Bound("keys") {
				t.Errorf("LocalScopeAt(1:%d).Bound(keys) = true at the cursor typing the let's value", column)
			}
			for _, node := range g.VisibleAt(1, column) {
				if node.Name == "keys" {
					t.Errorf("VisibleAt(1:%d) offers the let being typed", column)
				}
			}
		})
	}
}
