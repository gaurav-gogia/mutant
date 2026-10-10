package parser

import (
	"strings"
	"testing"
	"time"

	"mutant/ast"
	"mutant/lexer"
)

// Two rows, one cause. parseHole used to lex each hole behind padding -- a
// newline per line and a space per column, prepended to the hole's text -- so
// that the sub-parser would report file coordinates.
//
// It cost the square of the holes on a line, because hole k paid for padding as
// long as its own column (M26-LEX-009). And it bought only two of the three
// coordinates: one byte of padding stands in for a line of any width, so every
// offset the sub-parser produced was short by the difference. The node ranges
// were shifted back afterwards; the typed errors were not, and their comment
// said they needed no shifting, which was true of their lines and columns and
// false of their offsets (M26-LEX-018).
//
// The sub-lexer is now started at the hole's own position, so there is no
// padding to pay for and nothing to shift.

// offsetOf is the byte offset of a 1-based line and column in src, computed
// from src itself. It is the independent answer each reported offset is checked
// against -- and the check the padded parser could never have passed, since
// line and column were the two coordinates it got right.
func offsetOf(src string, line, column int) int {
	at := 0
	for l := 1; l < line; l++ {
		next := strings.IndexByte(src[at:], '\n')
		if next < 0 {
			return -1
		}
		at += next + 1
	}
	return at + column - 1
}

func TestAHoleErrorCarriesTheFilesOffsetAndNotTheFragments(t *testing.T) {
	for _, src := range []string{
		// Indented, on line 3, so the line, the column and the offset are
		// three different numbers and no two can be confused for each other.
		"let a = 1;\n\n        let s = \"x${ a + }y\";\n",
		// Deeper down the file, so the discrepancy is larger than any column.
		"let a = 1;\n\n\n\n            let s = \"${ a.. }\";\n",
		// Two holes on one line: the second is further right than the first,
		// which is what made the padding grow.
		"let a = 1;\n\n    let s = \"${ a + }and${ a.. }\";\n",
		// A hole on the first line, where padding and file agree about the
		// offset. This is the case that passed before the fix, kept so that a
		// regression cannot hide behind it.
		"let s = \"${ 1 + }\";\n",
	} {
		p := New(lexer.New(src))
		p.ParseProgram()

		typed := p.TypedErrors()
		if len(typed) == 0 {
			t.Fatalf("%q produced no typed errors, so there is nothing to check", src)
		}
		for _, e := range typed {
			for what, pos := range map[string]int{
				"start": e.Range.Start.Offset,
				"end":   e.Range.End.Offset,
			} {
				line, column := e.Range.Start.Line, e.Range.Start.Column
				if what == "end" {
					line, column = e.Range.End.Line, e.Range.End.Column
				}
				if want := offsetOf(src, line, column); want != pos {
					t.Errorf("%q: the %s offset of %q is %d, but its own line %d column %d is offset %d",
						src, what, e.Msg, pos, line, column, want)
				}
			}
		}
	}
}

// The same property for the positions a well-formed hole records, which is the
// path the formatter and the language server read. These were right before the
// fix, by an explicit shift that is now gone, so this is the test that the
// shift's removal took nothing with it.
func TestEveryNodeInsideAHoleHasAFileRange(t *testing.T) {
	src := "let a = 1;\n\n        let s = \"x${ a + 1 }y${ a }z\";\n"
	p := New(lexer.New(src))
	program := p.ParseProgram()

	if len(p.Errors()) != 0 {
		t.Fatalf("the well-formed source produced errors: %v", p.Errors())
	}
	if len(program.NodePositions) == 0 {
		t.Fatal("no node positions were recorded")
	}

	for node, rng := range program.NodePositions {
		if want := offsetOf(src, rng.Start.Line, rng.Start.Column); want != rng.Start.Offset {
			t.Errorf("%T starts at offset %d, but line %d column %d is offset %d",
				node, rng.Start.Offset, rng.Start.Line, rng.Start.Column, want)
		}
		if rng.Start.Offset < 0 || rng.End.Offset > len(src) || rng.Start.Offset > rng.End.Offset {
			t.Errorf("%T has the range %d:%d, which is not a range in a %d-byte file",
				node, rng.Start.Offset, rng.End.Offset, len(src))
			continue
		}
		// The identifiers inside the holes are the point, and they are the one
		// node kind whose spelling in the file is exactly its value, so the
		// file can be sliced at the range and compared. A statement or a
		// literal cannot be checked this way: Program.String() is a canonical
		// re-rendering that normalises spacing and adds parentheses, so it
		// never equals the source it came from.
		ident, ok := node.(*ast.Identifier)
		if !ok {
			continue
		}
		if got := src[rng.Start.Offset:rng.End.Offset]; got != ident.Value {
			t.Errorf("the identifier %q has a range the file reads as %q", ident.Value, got)
		}
	}
}

// A hole inside a hole: the inner sub-lexer is started at a position the outer
// one already reported, so the origins have to compose rather than accumulate.
func TestAHoleInsideAHoleStillGetsFilePositions(t *testing.T) {
	src := "let a = 1;\n\n       let s = \"${ \"inner ${ a }\" }\";\n"
	p := New(lexer.New(src))
	program := p.ParseProgram()

	if len(p.Errors()) != 0 {
		t.Fatalf("a nested hole produced errors: %v", p.Errors())
	}
	for node, rng := range program.NodePositions {
		if want := offsetOf(src, rng.Start.Line, rng.Start.Column); want != rng.Start.Offset {
			t.Errorf("%T starts at offset %d, but line %d column %d is offset %d",
				node, rng.Start.Offset, rng.Start.Line, rng.Start.Column, want)
		}
	}
}

// The cost. Thirty thousand holes on one line is 120 KB of source, which the
// padded parser turned into roughly 1.8 GB of padding read and discarded.
// Measured both ways at 4b906cd through go test -overlay: 12.22 s with the
// padding, 0.25 s without it, a factor of 49.
//
// The budget is wall-clock on a shared machine, which is why it is five seconds
// and not one: twenty times the figure the fix measures, and still well under
// half of what the defect costs, so neither a loaded box nor a faster one moves
// the verdict. The precedent is TestTheNestingBoundIsReachedQuickly in
// frontend_refusals_test.go, which guards a 200,000-token source with thirty
// seconds for the same reason.
func TestManyHolesOnOneLineDoNotCostTheSquareOfTheirNumber(t *testing.T) {
	const holes = 30000

	var b strings.Builder
	b.WriteString(`let a = 1; let s = "`)
	for i := 0; i < holes; i++ {
		b.WriteString("${a}")
	}
	b.WriteString("\";\n")
	src := b.String()

	began := time.Now()
	p := New(lexer.New(src))
	p.ParseProgram()
	elapsed := time.Since(began)

	if len(p.Errors()) != 0 {
		t.Fatalf("%d holes produced %d errors: %v", holes, len(p.Errors()), first(p.Errors(), 3))
	}
	if elapsed > 5*time.Second {
		t.Errorf("%d holes in %d bytes took %s, which is the quadratic cost returning",
			holes, len(src), elapsed)
	}
}

// And the control for it: the holes above all sit on one line, which is the
// shape that made the padding grow. A hole far down the file costs one newline
// per line even in the old code, so this says the fix did not trade one
// direction for the other.
func TestAHoleFarDownAFileIsStillCheap(t *testing.T) {
	src := strings.Repeat("let a = 1;\n", 20000) + "let s = \"${ a }\";\n"

	began := time.Now()
	p := New(lexer.New(src))
	p.ParseProgram()
	elapsed := time.Since(began)

	if len(p.Errors()) != 0 {
		t.Fatalf("20000 lines produced %d errors: %v", len(p.Errors()), first(p.Errors(), 3))
	}
	if elapsed > 5*time.Second {
		t.Errorf("a hole on line 20001 took %s", elapsed)
	}
}
