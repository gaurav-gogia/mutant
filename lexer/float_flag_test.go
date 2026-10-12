package lexer

import (
	"strconv"
	"strings"
	"testing"

	"mutant/token"
)

// readNumber's isFloat flag used to be set the moment a '.' was seen, before the
// test that decides whether to consume it. So a number written immediately
// before a dot it does not own came back marked as a float, and the token's own
// literal gave it away: a FLOAT holding no decimal point.
//
// Found by FuzzFormattingPreservesTheTree in 28 seconds on the input `0 .A`,
// which the formatter rewrites to `0.A` -- the same program to a reader, a
// different one to the lexer. It took ast.Equal to see it, because
// Program.String() renders an IntegerLiteral and a FloatLiteral of the same
// spelling as the same text.

// TestAFloatTokenAlwaysHoldsADecimalPoint is the invariant, stated as the one
// thing that cannot be true of a correct FLOAT.
func TestAFloatTokenAlwaysHoldsADecimalPoint(t *testing.T) {
	for _, src := range []string{
		"0.A", "0 .A", "1.b", "1.", "10.", "1..4", "3.to_s()", "1.5", "0.5", "1.5.x",
		"let x = 1.5; let y = 2; y.field;", "255;", "010.5;",
	} {
		l := New(src)
		for i := 0; i <= len(src)+2; i++ {
			tok := l.NextToken()
			if tok.Type == token.EOF {
				break
			}
			if tok.Type != token.FLOAT {
				continue
			}
			if !strings.Contains(tok.Literal, ".") {
				t.Errorf("%q: FLOAT %q holds no decimal point, so the number does not own the dot that marked it one",
					src, tok.Literal)
			}
			if _, err := strconv.ParseFloat(tok.Literal, 64); err != nil {
				t.Errorf("%q: FLOAT %q does not parse as a float: %v", src, tok.Literal, err)
			}
		}
	}
}

// And the shape the defect produced, pinned directly: a number before a dot is
// an integer, and the two spellings of it agree.
func TestANumberBeforeADotItDoesNotOwnIsAnInteger(t *testing.T) {
	for _, tc := range []struct {
		src     string
		literal string
	}{
		{"0.A", "0"},
		{"1.b", "1"},
		{"3.to_s()", "3"},
		{"10.", "10"},
		{"1..4", "1"},
	} {
		first := allTokens(t, tc.src)[0]
		if first.Type != token.INT || first.Literal != tc.literal {
			t.Errorf("%q begins with (%s, %q), want (INT, %q)",
				tc.src, first.Type, first.Literal, tc.literal)
		}
	}

	// `0 .A` and `0.A` are the same program, and the formatter turns the first
	// into the second. They have to lex alike or formatting changes meaning.
	spaced, tight := allTokens(t, "0 .A"), allTokens(t, "0.A")
	if len(spaced) != len(tight) {
		t.Fatalf("`0 .A` gives %s and `0.A` gives %s", types(spaced), types(tight))
	}
	for i := range spaced {
		if spaced[i].Type != tight[i].Type || spaced[i].Literal != tight[i].Literal {
			t.Errorf("token %d: `0 .A` has (%s, %q), `0.A` has (%s, %q)",
				i, spaced[i].Type, spaced[i].Literal, tight[i].Type, tight[i].Literal)
		}
	}
}

// The control, because the fix narrows what counts as a float: a real float is
// still a float, in every spelling the language has.
func TestRealFloatsAreStillFloats(t *testing.T) {
	for _, tc := range []struct {
		src     string
		literal string
	}{
		{"1.5", "1.5"},
		{"0.5", "0.5"},
		{"010.5", "010.5"},
		{"123.456", "123.456"},
		{"1.5.x", "1.5"},
		{"0.0", "0.0"},
	} {
		first := allTokens(t, tc.src)[0]
		if first.Type != token.FLOAT || first.Literal != tc.literal {
			t.Errorf("%q begins with (%s, %q), want (FLOAT, %q)",
				tc.src, first.Type, first.Literal, tc.literal)
		}
	}

	// And an integer is still an integer, so the flag was not simply turned off.
	for _, tc := range []struct {
		src     string
		literal string
	}{
		{"255", "255"},
		{"0", "0"},
		{"0755", "0755"},
	} {
		first := allTokens(t, tc.src)[0]
		if first.Type != token.INT || first.Literal != tc.literal {
			t.Errorf("%q begins with (%s, %q), want (INT, %q)",
				tc.src, first.Type, first.Literal, tc.literal)
		}
	}
}
