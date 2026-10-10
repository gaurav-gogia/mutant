package ast_test

import (
	"testing"

	"mutant/ast"
	"mutant/lexer"
	"mutant/parser"
)

// Equal exists because Program.String() was being asked a question it cannot
// answer. String() is a debug rendering and lossy on purpose: a block prints
// its statements with no braces, and a string literal prints its decoded text
// with no quotes. So distinct programs render to one string, and the
// formatter's structure test -- which compared renderings -- could watch a
// program turn into a different one and see nothing (M26-LEX-011).
//
// Every pair below was measured, not reasoned about: each really does render
// identically at 4b906cd, and each really is a different program.

func program(t *testing.T, src string) *ast.Program {
	t.Helper()
	p := parser.New(lexer.New(src))
	out := p.ParseProgram()
	if errs := p.Errors(); len(errs) != 0 {
		t.Fatalf("%q did not parse: %v", src, errs)
	}
	return out
}

// TestEqualSeesTheDifferencesStringCannotShow is the row's regression test.
//
// It asserts the collision as well as the fix, which is deliberate. If
// String() is ever made unambiguous, this test fails and says which pair stopped
// colliding, so the question "is Equal still needed here" gets asked rather
// than answered by silence. That is the same kind of pin as the hex mask in
// lexer/bitwise_test.go, and it fired exactly as intended when the lexer
// changed under it.
func TestEqualSeesTheDifferencesStringCannotShow(t *testing.T) {
	for _, tc := range []struct{ name, a, b string }{
		// A string literal renders as its decoded text, so it is
		// indistinguishable from a name, a number or a boolean.
		{"a string and a name", `let x = "a";`, `let x = a;`},
		{"a string and a name, as an argument", `putln("a");`, `putln(a);`},
		{"a string key and a name key", `let h = {"k": 1};`, `let h = {k: 1};`},
		{"a string and a boolean", `let t = true;`, `let t = "true";`},
		{"a string and a number", `let n = 1;`, `let n = "1";`},

		// A block renders without braces, so a statement moved out of a body
		// renders in the same place it did inside one. The loop cases are the
		// serious ones: how many times the statement runs is the difference.
		{"a statement moved out of an if", `if (c) { a; b; }`, `if (c) { a; } b;`},
		{"a statement moved out of a while", `while (c) { a; b; }`, `while (c) { a; } b;`},
		{"a statement moved out of a for",
			`for (let i = 0; i < 2; i++) { a; b; }`,
			`for (let i = 0; i < 2; i++) { a; } b;`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			first, second := program(t, tc.a), program(t, tc.b)

			if first.String() != second.String() {
				t.Fatalf("these no longer render alike, so the premise has changed:\n  %q renders %q\n  %q renders %q",
					tc.a, first.String(), tc.b, second.String())
			}
			if ast.Equal(first, second) {
				t.Errorf("Equal says %q and %q are the same program; both render as %q",
					tc.a, tc.b, first.String())
			}
		})
	}
}

// What Equal must treat as the same program. Each of these is something the
// formatter is allowed to do, so a comparator that reported them would make the
// corpus test fail on correct output.
func TestEqualIgnoresSpellingAndLayout(t *testing.T) {
	for _, tc := range []struct{ name, a, b string }{
		{"whitespace", "let x = 1;", "let    x   =   1 ;"},
		{"a line break", "let x = 1; let y = 2;", "let x = 1;\nlet y = 2;"},
		{"redundant parentheses", "let x = (1 + 2);", "let x = 1 + 2;"},
		{"two pairs of parentheses", "let x = ((a));", "let x = (a);"},
		{"a raw string with the same value", `let s = "a";`, `let s = r"a";`},
		{"a triple-quoted string with the same value", `let s = "a";`, `let s = """a""";`},
		{"an indented body", "if (c) { a; }", "if (c) {\n\ta;\n}"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if !ast.Equal(program(t, tc.a), program(t, tc.b)) {
				t.Errorf("Equal says %q and %q are different programs", tc.a, tc.b)
			}
		})
	}
}

// And the control that Equal has not been made lax to achieve the above: the
// differences String() does catch, it must catch too.
func TestEqualStillSeesTheDifferencesStringCatches(t *testing.T) {
	for _, tc := range []struct{ name, a, b string }{
		{"a dropped statement", "let x = 1; let y = 2;", "let x = 1;"},
		{"reordered statements", "let x = 1; let y = 2;", "let y = 2; let x = 1;"},
		{"a re-associated operator", "let x = a + b * c;", "let x = (a + b) * c;"},
		{"a different operator", "let x = a + b;", "let x = a - b;"},
		{"a dropped argument", "putln(a, b);", "putln(a);"},
		{"a changed number", "let x = 1;", "let x = 2;"},
		{"an int and a float", "let x = 1;", "let x = 1.0;"},
		{"a renamed binding", "let x = 1;", "let y = 1;"},
		{"a dropped else", "if (c) { a; } else { b; }", "if (c) { a; }"},
		{"reordered array elements", "let a = [1, 2];", "let a = [2, 1];"},
		{"reordered hash pairs", `let h = {"a": 1, "b": 2};`, `let h = {"b": 2, "a": 1};`},
		{"a hole become a concatenation", `let s = "a${b}c";`, `let s = "a" + b + "c";`},
		{"a different hole", `let s = "${a}";`, `let s = "${b}";`},
		{"a dropped parameter", "let f = fn(a, b) { return a; };", "let f = fn(a) { return a; };"},
		{"a renamed parameter", "let f = fn(a) { return 1; };", "let f = fn(b) { return 1; };"},
		{"a while become an if", "while (c) { a; }", "if (c) { a; }"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if ast.Equal(program(t, tc.a), program(t, tc.b)) {
				t.Errorf("Equal says %q and %q are the same program", tc.a, tc.b)
			}
		})
	}
}

// Nothing, and a nil pointer wrapped in a Node, are the same kind of nothing.
// An absent else branch is a nil *ast.BlockStatement stored in a field, which is
// not equal to nil as an interface, so a comparator written without this says
// two programs that both lack an else are different and then dereferences one.
func TestEqualHandlesNothing(t *testing.T) {
	var absent *ast.BlockStatement

	if !ast.Equal(nil, nil) {
		t.Error("Equal says nothing differs from nothing")
	}
	if !ast.Equal(absent, nil) {
		t.Error("Equal says an absent block differs from nothing")
	}
	if !ast.Equal(absent, (*ast.BlockStatement)(nil)) {
		t.Error("Equal says two absent blocks differ")
	}
	if ast.Equal(nil, &ast.Identifier{Value: "a"}) {
		t.Error("Equal says nothing is a name")
	}
	if ast.Equal(&ast.Identifier{Value: "a"}, absent) {
		t.Error("Equal says a name is an absent block")
	}

	// Two programs with no else, built by the parser, are the path this really
	// runs on.
	if !ast.Equal(program(t, "if (c) { a; }"), program(t, "if (c) { a; }")) {
		t.Error("Equal says an if with no else differs from itself")
	}
}

// Equal is reflexive on every shape the language has, parsed twice. A false
// here is a missing switch arm, which is the failure the enumerating test in
// node_coverage_test.go reports by name; this is the same property through the
// parser, on real source, so a node the parser builds differently from the test
// samples is covered too.
func TestEqualIsReflexiveOnEveryShape(t *testing.T) {
	for _, src := range []string{
		`let x = 1; let y = 1.5; let s = "t"; let b = true;`,
		`let s = "a${ b + 1 }c${ d }";`,
		`let f = fn(a, b) { return a + b; };`,
		`let f = fn(a) { return a, a; };`,
		`let m = macro(a) { quote(unquote(a)); };`,
		`let h = {"k": 1, "j": [1, 2]};`,
		`let a = [1, [2, 3]];`,
		`struct Point { x; y; }`,
		`enum Colour { RED, GREEN }`,
		`let p = Point{x: 1, y: 2};`,
		`import "m";`,
		`import m "m"; m.f();`,
		`for (let i = 0; i < 3; i++) { putln(i); }`,
		`for (;;) { break; }`,
		`while (c) { continue; }`,
		`let x, err = f();`,
		`let z = -a * (b + c)[0].d;`,
		`let q = a++; let r = b--;`,
		`x += 1; y -= 2;`,
		`if (a) { b; } else { c; }`,
		`match (x) { 1 => { a; }, 2 => { b; } }`,
	} {
		if !ast.Equal(program(t, src), program(t, src)) {
			t.Errorf("Equal says %q differs from itself", src)
		}
	}
}
