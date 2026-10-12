package compiler

import (
	"errors"
	"testing"

	"mutant/lexer"
	"mutant/parser"
	"mutant/sema"
)

// A refusal names where it was raised: the innermost node with a recorded
// range, which is what the editor needs to put the compiler's own sentence on
// the right line (M26-LSP-028). The text is unchanged and the refusal
// underneath still answers errors.As.
func TestARefusalCarriesTheRangeOfTheNodeThatRaisedIt(t *testing.T) {
	for _, c := range []struct {
		name, src, message string
		line, column       int
		// endLine and endColumn are checked when set, for the case whose
		// start a smaller node shares.
		endLine, endColumn int
	}{
		{"a break in a closure inside a loop", "for (let i = 0; i < 2; i++) {\n    let f = fn() { break; };\n}\n",
			"break used outside of for loop", 2, 20, 0, 0},
		{"an undefined name inside a call", "putln(nope);\n", "undefined variable: nope", 1, 7, 0, 0},
		{"an assignment to a builtin, raised below the assignment", "len = 5;\n", "cannot assign to builtin: len", 1, 1, 1, 8},
		{"a struct literal missing a field", "struct P { x, y };\nlet p = P{x: 1};\n",
			"struct P needs a value for field y: it declares x, y", 2, 9, 0, 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			p := parser.New(lexer.New(c.src))
			program := p.ParseProgram()
			if len(p.Errors()) > 0 {
				t.Fatalf("the fixture does not parse: %v", p.Errors())
			}
			err := New().Compile(program)
			if err == nil {
				t.Fatalf("compiled; want %q", c.message)
			}
			if err.Error() != c.message {
				t.Fatalf("message = %q, want %q unchanged", err.Error(), c.message)
			}
			rng, ok := ErrorRange(err)
			if !ok {
				t.Fatalf("%q carries no range", err)
			}
			if rng.Start.Line != c.line || rng.Start.Column != c.column {
				t.Fatalf("range starts %d:%d, want %d:%d", rng.Start.Line, rng.Start.Column, c.line, c.column)
			}
			if c.endLine != 0 && (rng.End.Line != c.endLine || rng.End.Column != c.endColumn) {
				t.Fatalf("range ends %d:%d, want %d:%d", rng.End.Line, rng.End.Column, c.endLine, c.endColumn)
			}
		})
	}

	// An error that no node raised carries no range, and asking is not an
	// error.
	for _, err := range []error{nil, errors.New("not a compile error")} {
		if rng, ok := ErrorRange(err); ok {
			t.Errorf("ErrorRange(%v) = %+v, true; want no range", err, rng)
		}
	}

	program := parser.New(lexer.New("break;\n")).ParseProgram()
	var refusal *sema.Refusal
	if err := New().Compile(program); !errors.As(err, &refusal) || refusal.Code != sema.RefuseLoopControlOutsideLoop {
		t.Fatalf("errors.As lost the refusal under the range: %v", err)
	}
}
