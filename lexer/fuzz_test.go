package lexer

import (
	"strconv"
	"strings"
	"testing"

	"mutant/token"
)

// The release gate's fuzz step discovered no targets at all until 2026-10-11 --
// `go test -list '^Fuzz' ./...` returned nothing, so the step reported
// "SKIP: no fuzz targets" and passing it meant nothing. These are the lexer's
// share of that.
//
// What is fuzzed here is the one property this package promises and has broken
// before: that the token stream ends. NextToken's own doc comment records the
// cost of it being untrue -- a rune a dispatch arm claims but no reader consumes
// yields a token of zero width, which the next call produces again, and nothing
// notices, because a lexer is never asked whether it is finished. That stall
// took down a whole test run once. A property no example happens to exercise is
// exactly what a fuzzer is for.

// seeds are the shapes a stall has come from: a character no rule begins with, a
// literal that is never closed, a NUL, a hole left open, a superscript after
// digits, and a raw literal ending in a backslash.
var seeds = []string{
	"let x = 1;",
	"let s = \"a${ b }c\";",
	"let y = 2\u00b2;",
	"r\"C:\\evidence\\\"",
	"\"unterminated",
	"\"\"\"triple",
	"${",
	"let z = 6 \\ 3;",
	"// comment with no newline",
	"0xFF 1_000 1.5e3 255u8",
	"\x00",
	"",
}

// FuzzTheLexerAlwaysReachesEOF drains the stream under a bound. Reaching the
// bound is the failure: it means a call produced a token without moving, so an
// unbounded caller -- which is every caller, including the parser's recovery
// loop -- would never stop.
func FuzzTheLexerAlwaysReachesEOF(f *testing.F) {
	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, src string) {
		l := New(src)
		// Every token but EOF consumes at least one rune, so the stream cannot
		// be longer than the input plus the EOF marker. The slack is for the
		// zero-width EOF and nothing else.
		bound := len(src) + 2
		for i := 0; i <= bound; i++ {
			tok := l.NextToken()
			if tok.Type == token.EOF {
				return
			}
			if tok.End.Offset <= tok.Start.Offset {
				t.Fatalf("token %d (%s %q) has width %d, so the lexer did not move",
					i, tok.Type, tok.Literal, tok.End.Offset-tok.Start.Offset)
			}
		}
		t.Fatalf("the lexer produced more than %d tokens for %d bytes without reaching EOF", bound, len(src))
	})
}

// FuzzEveryTokenLiteralMatchesItsType is the invariant that caught the isFloat
// flag: a FLOAT whose literal holds no decimal point is a number that was
// marked a float by a dot it does not own. The deterministic cases are in
// float_flag_test.go; this is the same statement over input nobody wrote.
func FuzzEveryTokenLiteralMatchesItsType(f *testing.F) {
	for _, seed := range seeds {
		f.Add(seed)
	}
	f.Add("0.A")
	f.Add("3.to_s()")
	f.Add("1..4")

	f.Fuzz(func(t *testing.T, src string) {
		l := New(src)
		for i := 0; i <= len(src)+2; i++ {
			tok := l.NextToken()
			if tok.Type == token.EOF {
				return
			}
			switch tok.Type {
			case token.FLOAT:
				if !strings.Contains(tok.Literal, ".") {
					t.Fatalf("FLOAT %q holds no decimal point", tok.Literal)
				}
				if _, err := strconv.ParseFloat(tok.Literal, 64); err != nil {
					t.Fatalf("FLOAT %q does not parse as a float: %v", tok.Literal, err)
				}
			case token.INT:
				for _, r := range tok.Literal {
					if r < '0' || r > '9' {
						t.Fatalf("INT %q holds %q, which is not a digit", tok.Literal, r)
					}
				}
			}
		}
	})
}

// FuzzEveryTokenPositionIsInsideTheSource is the other half: a position the
// lexer reports has to be a position in the text it was given, in all three
// coordinates and consistently between them. The offsets are what an editor
// slices by, and a position that is merely plausible is how an off-by-one
// survives -- M26-LEX-018 was exactly that, correct lines and columns over
// offsets taken from somewhere else.
func FuzzEveryTokenPositionIsInsideTheSource(f *testing.F) {
	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, src string) {
		l := New(src)
		for i := 0; i <= len(src)+2; i++ {
			tok := l.NextToken()

			for what, pos := range map[string]token.Position{"start": tok.Start, "end": tok.End} {
				if pos.Line < 1 || pos.Column < 1 {
					t.Fatalf("%s of %s is line %d column %d; both are 1-based",
						what, tok.Type, pos.Line, pos.Column)
				}
				if pos.Offset < 0 || pos.Offset > len(src) {
					t.Fatalf("%s of %s is offset %d, outside a %d-byte source",
						what, tok.Type, pos.Offset, len(src))
				}
				if want := offsetIn(src, pos.Line, pos.Column); want != pos.Offset {
					t.Fatalf("%s of %s (%q) is offset %d, but its own line %d column %d is offset %d",
						what, tok.Type, tok.Literal, pos.Offset, pos.Line, pos.Column, want)
				}
			}
			if tok.Start.Offset > tok.End.Offset {
				t.Fatalf("%s starts at %d and ends at %d", tok.Type, tok.Start.Offset, tok.End.Offset)
			}
			if tok.Type == token.EOF {
				return
			}
		}
	})
}
