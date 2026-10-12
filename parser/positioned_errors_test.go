package parser

import (
	"strings"
	"testing"

	"mutant/lexer"
)

// Parser.Errors carries no position, and it is what every path to a terminal
// printed: the command line through module.ParseError, the REPL, the browser
// REPL and net_serve's handler loader. So `mutant run` reported `expected next
// token to be ), but got IDENT instead` and left the author to find it, while
// the language server showed the same mistake underlined in place, because it
// renders TypedErrors (M26-LEX-015).
//
// PositionedErrors is Errors with each message placed. These tests cover the
// rendering, the invariant it rests on, and the two cases where it declines to
// place a message rather than inventing a position for it.

func TestEveryParseErrorTheTerminalPrintsCarriesItsPosition(t *testing.T) {
	for _, src := range []string{
		"let = ;\n",
		"let x = (1 + ;\n",
		"let a = 1;\nlet b = ;\n",
		"putln(1 2);\n",
		"let s = \"${ x + }\";\n",
		"let y = 6 \\ 3;\n",
		"fn(a, a) { return a; };\n",
		"let z = 0xFF;\n",
		"let q = 1;\nlet w = 2;\n   let e = ;\n",
	} {
		p := New(lexer.New(src))
		p.ParseProgram()

		placed := p.PositionedErrors("w.mut")
		if len(placed) == 0 {
			t.Errorf("%q produced no errors, so there is nothing to place", src)
			continue
		}
		if len(placed) != len(p.Errors()) {
			t.Errorf("%q: %d placed messages for %d errors", src, len(placed), len(p.Errors()))
		}
		for _, msg := range placed {
			if !strings.HasPrefix(msg, "w.mut:") {
				t.Errorf("%q: the message %q is not placed in a file", src, msg)
				continue
			}
			if !hasLineColumn(msg[len("w.mut:"):]) {
				t.Errorf("%q: the message %q names a file but no line and column", src, msg)
			}
		}
	}
}

// An empty path is a REPL line, which has no file to name.
func TestWithNoPathTheMessageIsPlacedByLineAndColumnAlone(t *testing.T) {
	p := New(lexer.New("let x = ;\n"))
	p.ParseProgram()

	placed := p.PositionedErrors("")
	if len(placed) == 0 {
		t.Fatal("no errors")
	}
	for _, msg := range placed {
		if strings.HasPrefix(msg, ":") {
			t.Errorf("the message %q begins with an empty path", msg)
		}
		if !hasLineColumn(msg) {
			t.Errorf("the message %q carries no line and column", msg)
		}
	}
}

// The position is the mistake's own, not the statement's or the line's.
func TestThePositionIsWhereTheMistakeIs(t *testing.T) {
	// `let a = 1;` is line 1. The `;` with nothing before it is line 3, and it
	// is indented, so the column is neither 1 nor the statement's.
	p := New(lexer.New("let a = 1;\nlet b = 2;\n      let c = ;\n"))
	p.ParseProgram()

	placed := p.PositionedErrors("w.mut")
	if len(placed) == 0 {
		t.Fatal("no errors")
	}
	if !strings.HasPrefix(placed[0], "w.mut:3:15:") {
		t.Errorf("the first message is %q, want it placed at w.mut:3:15", placed[0])
	}
}

// TestThePlainAndTypedErrorListsStayPaired is the invariant PositionedErrors
// rests on: appendError appends to both lists, one entry each, in order, so the
// two pair by index.
//
// A drift would put a position on the wrong message, which is worse than
// putting none on any -- it would send a reader to a line that is correct. The
// two known asymmetries are both checked rather than assumed: the typed list is
// never longer than the plain one, and each typed message is a suffix of its
// twin, which is what lets a hole's message carry an "inside ${...}" prefix in
// the plain list alone.
func TestThePlainAndTypedErrorListsStayPaired(t *testing.T) {
	for _, src := range []string{
		"let = ;\n",
		"let s = \"${ x + }\";\n",
		"let s = \"a${ x + }b${ y.. }c\";\n",
		"putln(1 2); putln(3 4);\n",
		"let a = ;\nlet b = ;\nlet c = ;\n",
		"let y = 6 \\ 3;\n",
		strings.Repeat("let a = ;\n", 40),
	} {
		p := New(lexer.New(src))
		p.ParseProgram()

		plain, typed := p.Errors(), p.TypedErrors()
		if len(typed) > len(plain) {
			t.Errorf("%q: %d typed errors for %d plain ones; the typed list cannot be the longer",
				trim(src), len(typed), len(plain))
			continue
		}
		for i := range typed {
			if !strings.HasSuffix(plain[i], typed[i].Msg) {
				t.Errorf("%q: error %d has drifted -- plain %q, typed %q",
					trim(src), i, plain[i], typed[i].Msg)
			}
		}
	}
}

// Past the cap the parse says so once and stops, and that note is appended to
// the plain list alone: a complaint about the number of diagnostics has no
// range to sit at. PositionedErrors has to print it unplaced rather than drop
// it or place it at 0:0.
func TestTheTooManyErrorsNoteIsPrintedWithoutAPosition(t *testing.T) {
	p := New(lexer.New(strings.Repeat("let a = ;\n", maxParseErrors+10)))
	p.ParseProgram()

	plain := p.Errors()
	if len(plain) <= len(p.TypedErrors()) {
		t.Fatalf("the cap note is missing: %d plain errors, %d typed", len(plain), len(p.TypedErrors()))
	}

	placed := p.PositionedErrors("w.mut")
	if len(placed) != len(plain) {
		t.Fatalf("%d placed messages for %d errors", len(placed), len(plain))
	}
	last := placed[len(placed)-1]
	if !strings.Contains(last, "too many parse errors") {
		t.Fatalf("the last message is %q, want the cap note", last)
	}
	if strings.HasPrefix(last, "w.mut:") {
		t.Errorf("the cap note is placed in the file, at %q; it has no position to be at", last)
	}
}

// A hole's message says it is in a hole and is placed at the mistake, not at
// the hole's opening brace. Both halves used to come from one hand-written
// prefix inside parseHole, which named the hole's start -- so the only plain
// message in the parser that carried a position carried the wrong one.
func TestAHoleMessageSaysSoAndIsPlacedAtTheMistake(t *testing.T) {
	p := New(lexer.New("let a = 1;\nlet s = \"xx${ a + }yy\";\n"))
	p.ParseProgram()

	placed := p.PositionedErrors("w.mut")
	if len(placed) == 0 {
		t.Fatal("a hole with a dangling operator parsed clean")
	}
	var hole string
	for _, msg := range placed {
		if strings.Contains(msg, "inside ${...}") {
			hole = msg
		}
	}
	if hole == "" {
		t.Fatalf("no message says it is inside a hole: %v", placed)
	}
	// `let s = "xx${ a + }yy";` puts the `$` at column 12 and the `}` at column
	// 19. The mistake is the missing operand after the `+`, and where it would
	// have gone is where the hole's text runs out, which is the `}`. So 19 is
	// the answer and 12 is what a message placed at the hole's opening would
	// say -- and 12 is what the hand-written prefix used to say for every
	// mistake anywhere inside the hole, however far in.
	if !strings.HasPrefix(hole, "w.mut:2:19:") {
		t.Errorf("the message is %q, want it placed at w.mut:2:19, where the missing operand would go", hole)
	}
}

// hasLineColumn reports whether s begins with "<digits>:<digits>:".
func hasLineColumn(s string) bool {
	for range 2 {
		seen := 0
		for seen < len(s) && s[seen] >= '0' && s[seen] <= '9' {
			seen++
		}
		if seen == 0 || seen >= len(s) || s[seen] != ':' {
			return false
		}
		s = s[seen+1:]
	}
	return true
}
