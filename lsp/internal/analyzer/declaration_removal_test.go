package analyzer

import (
	"fmt"
	"testing"

	mast "mutant/ast"
	"mutant/evaluator"
	"mutant/lexer"
	"mutant/object"
	"mutant/parser"

	lsp "github.com/tliron/glsp/protocol_3_16"
)

// The quick fixes that remove an unused or duplicated declaration deleted the
// diagnostic's whole line whatever else it held (M26-LSP-009). They now ask
// DeclarationRemovalRange, which answers only for a declaration that can be
// deleted without deleting anything else, and declines everything it cannot be
// sure of. The server's tests apply the fixes end to end; these pin the parts
// of the answer a server test reaches only indirectly.

// TestOnlyAnInitializerThatRunsNothingIsInert pins which initializers let an
// unused `let` be deleted outright. Deleting the let deletes its initializer,
// so the initializer must be one whose evaluation nothing could observe: no
// call, nothing that can fail at run time. Each row also pins the node the
// parser makes of the text, so a row cannot quietly come to test another shape.
func TestOnlyAnInitializerThatRunsNothingIsInert(t *testing.T) {
	for _, tc := range []struct {
		initializer string
		shape       string
		inert       bool
	}{
		{initializer: "1", shape: "*ast.IntegerLiteral", inert: true},
		{initializer: "2.5", shape: "*ast.FloatLiteral", inert: true},
		{initializer: `"text"`, shape: "*ast.StringLiteral", inert: true},
		{initializer: "false", shape: "*ast.Boolean", inert: true},
		{initializer: "other", shape: "*ast.Identifier", inert: true},
		{initializer: "fn(x) { return x * 2; }", shape: "*ast.FunctionLiteral", inert: true},
		{initializer: "macro(x) { quote(unquote(x)); }", shape: "*ast.MacroLiteral", inert: true},
		{initializer: "-1", shape: "*ast.PrefixExpression", inert: true},
		{initializer: "-2.5", shape: "*ast.PrefixExpression", inert: true},
		{initializer: "!true", shape: "*ast.PrefixExpression", inert: true},
		{initializer: "~1", shape: "*ast.PrefixExpression", inert: true},
		{initializer: "!2.5", shape: "*ast.PrefixExpression", inert: true},
		{initializer: "[]", shape: "*ast.ArrayLiteral", inert: true},
		{initializer: `[1, "two", false, [-3]]`, shape: "*ast.ArrayLiteral", inert: true},

		// Each of these runs code or can stop the program, so deleting the let
		// would change what the program does.
		{initializer: "launch()", shape: "*ast.CallExpression", inert: false},
		{initializer: "1 / n", shape: "*ast.InfixExpression", inert: false},  // n may be zero
		{initializer: "n + 1", shape: "*ast.InfixExpression", inert: false},  // n may be a string
		{initializer: "-n", shape: "*ast.PrefixExpression", inert: false},    // n may not be a number
		{initializer: "-true", shape: "*ast.PrefixExpression", inert: false}, // no negation of a boolean
		{initializer: "~2.5", shape: "*ast.PrefixExpression", inert: false},  // ~ wants an integer
		{initializer: "~true", shape: "*ast.PrefixExpression", inert: false},
		{initializer: "xs[0]", shape: "*ast.IndexExpression", inert: false}, // xs may not be indexable
		{initializer: "p.x", shape: "*ast.FieldExpression", inert: false},   // p may have no field x
		{initializer: "{key: 1}", shape: "*ast.HashLiteral", inert: false},  // key may not be hashable
		{initializer: "[1, launch()]", shape: "*ast.ArrayLiteral", inert: false},
		{initializer: `"total ${launch()}"`, shape: "*ast.TemplateLiteral", inert: false},
		{initializer: "if (n) { launch(); } else { 2; }", shape: "*ast.IfExpression", inert: false},
		{initializer: "match (n) { 1 => launch(), _ => 0 }", shape: "*ast.MatchExpression", inert: false},
	} {
		src := "let v = " + tc.initializer + ";\n"
		snapshot := New().Analyze(src)
		if len(snapshot.ParseErrors) > 0 {
			t.Errorf("%q does not parse: %v", src, snapshot.ParseErrors)
			continue
		}
		let, ok := snapshot.Program.Statements[0].(*mast.LetStatement)
		if !ok {
			t.Errorf("%q parses as %T, not a let", src, snapshot.Program.Statements[0])
			continue
		}
		if shape := fmt.Sprintf("%T", let.Value); shape != tc.shape {
			t.Errorf("%q parses its initializer as %s, want %s", src, shape, tc.shape)
			continue
		}
		if got := inertInitializer(let.Value); got != tc.inert {
			t.Errorf("inertInitializer(%s) = %v, want %v", tc.initializer, got, tc.inert)
		}
	}
}

// evaluationHalts runs src through the evaluator and reports whether it stopped
// on a run-time error, with the error's text. The evaluator and the VM refuse
// the same operand kinds -- the parity suite holds them to it -- and the
// evaluator needs no build. src ends in an expression statement, so a program
// that ran to the end has that value as its result rather than an error.
func evaluationHalts(t *testing.T, src string) (bool, string) {
	t.Helper()
	p := parser.New(lexer.New(src))
	program := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("%q does not parse: %v", src, p.Errors())
	}
	if failure, isError := evaluator.Eval(program, object.NewEnvironment()).(*object.Error); isError {
		return true, failure.Message
	}
	return false, ""
}

// TestTheOperatorDecidesWhetherAPrefixedLiteralRuns pins the run-time half of
// the table above: each prefixed literal it calls inert runs, and each one it
// does not halts the program, so deleting that let would also delete the
// failure the program stops on.
func TestTheOperatorDecidesWhetherAPrefixedLiteralRuns(t *testing.T) {
	for _, tc := range []struct {
		initializer string
		halts       bool
	}{
		{initializer: "-1"},
		{initializer: "-2.5"},
		{initializer: "~1"},
		{initializer: "!true"},
		{initializer: "!2.5"},
		{initializer: "-true", halts: true},
		{initializer: "~2.5", halts: true},
		{initializer: "~true", halts: true},
	} {
		src := "let v = " + tc.initializer + ";\n1;\n"
		halted, detail := evaluationHalts(t, src)
		if halted != tc.halts {
			t.Errorf("%q: halted=%v (%s), want %v", src, halted, detail, tc.halts)
			continue
		}
		let, ok := New().Analyze(src).Program.Statements[0].(*mast.LetStatement)
		if !ok {
			t.Errorf("%q does not begin with a let", src)
			continue
		}
		if inertInitializer(let.Value) == halted {
			t.Errorf("inertInitializer(%s) = %v, but evaluating it halted=%v", tc.initializer, !halted, halted)
		}
	}
}

// TestARemovalFixDeclinesATypeDeclaredAgainDifferently: the build accepts a
// struct declared twice under one name, and the later declaration is the one in
// effect, so removing it from `struct P { x }; struct P { y };` turned a program
// that builds into one the compiler refuses. Neither of two different
// declarations is offered; of two identical ones, which are dead copies of each
// other, either may go.
func TestARemovalFixDeclinesATypeDeclaredAgainDifferently(t *testing.T) {
	const differ = "struct P { x };\nstruct P { y };\nlet q = P{y: 1};\nq;\n"
	snapshot := New().Analyze(differ)
	if refusals := snapshot.CompileDiagnostics(nil); len(refusals) != 0 {
		t.Fatalf("the compiler refuses the fixture (%v), so it tests nothing", refusals)
	}
	if halted, detail := evaluationHalts(t, differ); halted {
		t.Fatalf("the fixture halts (%s), so the later struct is not the one in effect", detail)
	}
	for _, at := range []lsp.Position{{Line: 0, Character: 7}, {Line: 1, Character: 7}} {
		if rng, ok := snapshot.DeclarationRemovalRange(at, "P"); ok {
			t.Errorf("offered %+v at %d:%d for one of two different declarations of P", rng, at.Line, at.Character)
		}
	}
	if rng, ok := New().Analyze("enum E { A, B };\nenum E { A, C };\nE.C;\n").DeclarationRemovalRange(lsp.Position{Line: 1, Character: 5}, "E"); ok {
		t.Errorf("offered %+v for one of two different declarations of E", rng)
	}

	for _, tc := range []struct {
		src  string
		at   lsp.Position
		name string
	}{
		{src: "struct P { x };\nstruct P { x };\nlet q = P{x: 1};\nq;\n", at: lsp.Position{Line: 1, Character: 7}, name: "P"},
		{src: "enum E { A, B };\nenum E { A, B };\nE.A;\n", at: lsp.Position{Line: 1, Character: 5}, name: "E"},
	} {
		if _, ok := New().Analyze(tc.src).DeclarationRemovalRange(tc.at, tc.name); !ok {
			t.Errorf("%q: declined to remove one of two identical declarations", tc.src)
		}
	}
}

// TestDeclarationRemovalRangeDeclinesInAFileThatDoesNotParse: next to a parse
// error a statement's range ends where the parser stopped, not where the
// statement does, so no range is offered anywhere in such a file.
func TestDeclarationRemovalRangeDeclinesInAFileThatDoesNotParse(t *testing.T) {
	at := lsp.Position{Line: 0, Character: 4}

	clean := New().Analyze("let a = 1; putln(\"x\");\nlet b = 2;\n")
	rng, ok := clean.DeclarationRemovalRange(at, "a")
	if !ok {
		t.Fatal("declined in a file that parses, so declining below would prove nothing")
	}
	// The let and the blank after it, and nothing of the putln.
	if want := (lsp.Range{Start: lsp.Position{Line: 0, Character: 0}, End: lsp.Position{Line: 0, Character: 11}}); rng != want {
		t.Fatalf("range = %+v, want %+v", rng, want)
	}

	broken := New().Analyze("let a = 1; putln(\"x\");\nlet b = ;\n")
	if len(broken.ParseErrors) == 0 {
		t.Fatal("the broken fixture parses, so it tests nothing")
	}
	if rng, ok := broken.DeclarationRemovalRange(at, "a"); ok {
		t.Fatalf("offered %+v in a file that does not parse", rng)
	}
}

// TestDeclarationRemovalRangeDeclinesAPositionThatDoesNotHoldTheName: the
// position and the name come from a diagnostic, which can have been measured
// in older text than the snapshot's. Unless the name stands exactly there, the
// diagnostic is not about this text, and no range is offered.
func TestDeclarationRemovalRangeDeclinesAPositionThatDoesNotHoldTheName(t *testing.T) {
	snapshot := New().Analyze("let abc = 1;\nputln(\"x\");\n")
	if _, ok := snapshot.DeclarationRemovalRange(lsp.Position{Line: 0, Character: 4}, "abc"); !ok {
		t.Fatal("declined the name where it stands, so declining below would prove nothing")
	}
	for _, tc := range []struct {
		name  string
		pos   lsp.Position
		ident string
	}{
		{name: "another name is asked for", pos: lsp.Position{Line: 0, Character: 4}, ident: "abd"},
		{name: "the position is inside the name", pos: lsp.Position{Line: 0, Character: 5}, ident: "abc"},
		{name: "the position is on the let keyword", pos: lsp.Position{Line: 0, Character: 0}, ident: "abc"},
		{name: "the position is on another statement", pos: lsp.Position{Line: 1, Character: 0}, ident: "abc"},
		{name: "the line is past the end of the file", pos: lsp.Position{Line: 7, Character: 4}, ident: "abc"},
	} {
		if rng, ok := snapshot.DeclarationRemovalRange(tc.pos, tc.ident); ok {
			t.Errorf("%s: offered %+v for %q at %d:%d", tc.name, rng, tc.ident, tc.pos.Line, tc.pos.Character)
		}
	}
}
