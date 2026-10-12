package compiler

import (
	"strings"
	"testing"

	"mutant/lexer"
	"mutant/parser"
)

// M26-LSP-035. `y = 1` with no y in scope used to compile: the assignment path
// defined the name in the current scope, so the line was an implicit let, while
// the editor and `mutant lint` reported `undefined identifier y` at Error. Only a
// let declares a name, so the build now refuses the line as the editor does.
func TestAssignmentToAnUndeclaredNameIsRefusedLSP035(t *testing.T) {
	refused := []struct {
		name string
		src  string
	}{
		{"at the top level", "y = 1;\nputln(y);\n"},
		{"inside a function", "let f = fn() { y = 5; return y; };\nputln(f());\n"},
		{"inside a block", "if (true) { y = 1; putln(y); }\n"},
		{"inside a loop body", "for (v in [1, 2]) { y = v; }\n"},
	}
	for _, tc := range refused {
		t.Run(tc.name, func(t *testing.T) {
			err := New().Compile(parser.New(lexer.New(tc.src)).ParseProgram())
			if err == nil {
				t.Fatalf("the compiler accepted an assignment to y, which nothing declares:\n%s", tc.src)
			}
			const want = "cannot assign to y: y is not declared -- declare it with `let y = ...`"
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("refused with %q, want it to say %q", err.Error(), want)
			}
		})
	}

	// Assignments to names that are declared -- here, in an enclosing scope, in
	// a loop header -- are what `=` is for and still compile.
	accepted := []struct {
		name string
		src  string
	}{
		{"a top-level name", "let y = 1;\ny = 2;\nputln(y);\n"},
		{"an enclosing name from a function", "let y = 1;\nlet f = fn() { y = 2; return y; };\nputln(f());\n"},
		{"a function's own local", "let f = fn() { let y = 1; y = 2; return y; };\nputln(f());\n"},
		{"an enclosing name from a block", "let y = 1;\nif (true) { y = 2; }\nputln(y);\n"},
		{"a loop header's variable", "for (let i = 0; i < 2; i = i + 1) { putln(i); }\n"},
	}
	for _, tc := range accepted {
		t.Run(tc.name, func(t *testing.T) {
			if err := New().Compile(parser.New(lexer.New(tc.src)).ParseProgram()); err != nil {
				t.Fatalf("refused an assignment to a declared name: %v\n%s", err, tc.src)
			}
		})
	}
}
