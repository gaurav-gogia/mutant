package parser

import (
	"strings"
	"testing"

	"mutant/ast"
	"mutant/lexer"
)

// The release gate's fuzz step found no targets in the whole module until
// 2026-10-11, so it reported "SKIP: no fuzz targets" and passing it said
// nothing. These are the parser's share.
//
// Nothing here evaluates anything. The parser depends on token, ast and lexer
// and on nothing else, so a fuzz input is text that gets read and turned into a
// tree -- no macro is expanded, no builtin is called and no VM runs, which
// matters because `mutant gen` does expand macros and an unquote argument there
// is user code. A fuzzer over that would be a fuzzer over arbitrary execution.

// parserSeeds are shapes this review has filed defects against, so the corpus
// starts on the edges rather than on `let x = 1;`.
var parserSeeds = []string{
	"let x = 1;",
	"let s = \"a${ b + 1 }c\";",
	"let y = x--1;",
	"fn(a, a) { return a; };",
	"let z = 6 \\ 3;",
	"let m = 0xFF;",
	"putln(1 2);",
	"if (c) { a; } else { b; }",
	"match (x) { 1 => { a; }, 2 => { b; } }",
	"for (let i = 0; i < 3; i++) { putln(i); }",
	"for k, v in h { putln(k); }",
	"struct P { x; y; }",
	"enum C { RED, GREEN }",
	"import p \"lib/p.mut\";",
	"let f = fn(a) { return a; }; let g = macro(b) { quote(unquote(b)); };",
	"let h = {\"k\": 1, \"j\": [1, 2]};",
	strings.Repeat("(", 64) + "1" + strings.Repeat(")", 64) + ";",
	"${",
	"\"unterminated",
	"",
}

func addParserSeeds(f *testing.F) {
	for _, seed := range parserSeeds {
		f.Add(seed)
	}
}

// FuzzTheParserAlwaysTerminatesAndStaysWithinItsCaps. A panic is the obvious
// failure and the one a fuzzer is usually run for; the caps are the interesting
// part. maxParseErrors exists so a confused parse stops talking, and
// maxNestingDepth so a deep one stops before the stack does -- both are bounds
// on what the parser does with input nobody wrote, which is what this generates.
func FuzzTheParserAlwaysTerminatesAndStaysWithinItsCaps(f *testing.F) {
	addParserSeeds(f)

	f.Fuzz(func(t *testing.T, src string) {
		p := New(lexer.New(src))
		program := p.ParseProgram()
		if program == nil {
			t.Fatal("ParseProgram returned no program; it returns a tree even for input it refuses")
		}

		// One past the cap: the note that reporting stopped is itself an entry.
		if len(p.Errors()) > maxParseErrors+1 {
			t.Fatalf("%d errors for %d bytes, above the cap of %d", len(p.Errors()), len(src), maxParseErrors)
		}
		if len(p.TypedErrors()) > maxParseErrors {
			t.Fatalf("%d typed errors, above the cap of %d", len(p.TypedErrors()), maxParseErrors)
		}
	})
}

// FuzzEveryRecordedRangeIsARangeInTheSource is M26-LEX-018's invariant, widened
// from the interpolation holes it was filed against to every node the parser
// records. A range is what an editor slices the file by, so a position that is
// merely plausible -- the right line and column over an offset from somewhere
// else -- is wrong in the way that is hardest to see.
func FuzzEveryRecordedRangeIsARangeInTheSource(f *testing.F) {
	addParserSeeds(f)

	f.Fuzz(func(t *testing.T, src string) {
		p := New(lexer.New(src))
		program := p.ParseProgram()

		for node, rng := range program.NodePositions {
			if rng.Start.Offset < 0 || rng.End.Offset > len(src) {
				t.Fatalf("%T has the range %d:%d, outside a %d-byte source",
					node, rng.Start.Offset, rng.End.Offset, len(src))
			}
			if rng.Start.Offset > rng.End.Offset {
				t.Fatalf("%T starts at %d and ends at %d", node, rng.Start.Offset, rng.End.Offset)
			}
			if want := offsetOf(src, rng.Start.Line, rng.Start.Column); want != rng.Start.Offset {
				t.Fatalf("%T starts at offset %d, but its own line %d column %d is offset %d",
					node, rng.Start.Offset, rng.Start.Line, rng.Start.Column, want)
			}
		}

		// The same for the errors, which is where the defect actually was.
		for _, e := range p.TypedErrors() {
			if e.Range.Start.Offset < 0 || e.Range.End.Offset > len(src) {
				t.Fatalf("the error %q has the range %d:%d, outside a %d-byte source",
					e.Msg, e.Range.Start.Offset, e.Range.End.Offset, len(src))
			}
			if want := offsetOf(src, e.Range.Start.Line, e.Range.Start.Column); want != e.Range.Start.Offset {
				t.Fatalf("the error %q starts at offset %d, but its own line %d column %d is offset %d",
					e.Msg, e.Range.Start.Offset, e.Range.Start.Line, e.Range.Start.Column, want)
			}
		}
	})
}

// FuzzACloneEqualsItsOriginal pairs the two walks against each other, so neither
// can be wrong on its own without being caught. A Clone arm that shares a child
// instead of copying it, and an Equal arm that does not read one, both make this
// fail -- and a node type that neither handles fails it too, because Clone
// returns the node unchanged and Equal reports an unhandled kind as different.
func FuzzACloneEqualsItsOriginal(f *testing.F) {
	addParserSeeds(f)

	f.Fuzz(func(t *testing.T, src string) {
		program := New(lexer.New(src)).ParseProgram()

		cloned, ok := ast.Clone(program).(*ast.Program)
		if !ok {
			t.Fatalf("cloning a program gave a %T", ast.Clone(program))
		}
		if !ast.Equal(program, cloned) {
			t.Fatalf("a clone of %q is not equal to it", trim(src))
		}
	})
}

// FuzzEveryMessageTheTerminalPrintsIsPlaced is M26-LEX-015's invariant. Every
// message but the cap note has to carry its own position, and the pairing
// between the plain and typed lists has to hold, because a drift would place a
// message at another message's position -- which sends a reader to a line that
// is correct, and is worse than placing nothing.
func FuzzEveryMessageTheTerminalPrintsIsPlaced(f *testing.F) {
	addParserSeeds(f)

	f.Fuzz(func(t *testing.T, src string) {
		p := New(lexer.New(src))
		p.ParseProgram()

		plain, typed := p.Errors(), p.TypedErrors()
		if len(typed) > len(plain) {
			t.Fatalf("%d typed errors for %d plain ones; the typed list cannot be the longer", len(typed), len(plain))
		}
		for i := range typed {
			if !strings.HasSuffix(plain[i], typed[i].Msg) {
				t.Fatalf("error %d has drifted -- plain %q, typed %q", i, plain[i], typed[i].Msg)
			}
		}

		placed := p.PositionedErrors("w.mut")
		if len(placed) != len(plain) {
			t.Fatalf("%d placed messages for %d errors", len(placed), len(plain))
		}
		for i, msg := range placed {
			if i >= len(typed) {
				// Past the typed list is the cap note, which has no range.
				continue
			}
			if !strings.HasPrefix(msg, "w.mut:") {
				t.Fatalf("the message %q is not placed in a file", msg)
			}
			if !hasLineColumn(msg[len("w.mut:"):]) {
				t.Fatalf("the message %q names a file but no line and column", msg)
			}
		}
	})
}
