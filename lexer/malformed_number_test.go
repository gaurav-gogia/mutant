package lexer

import (
	"strings"
	"testing"

	"mutant/token"
)

// A name written immediately after digits is one malformed number, and this
// lexer used to hand on two tokens for it without a word: `0xFF` became INT(0)
// IDENT(xFF), `1.5e3` became FLOAT(1.5) IDENT(e3), `1_000` became INT(1)
// IDENT(_000). Every test below fails at 4360eec, where each source lexes into
// two clean tokens and no layer afterwards can tell the split from two
// statements the author wrote (M26-LEX-007).
//
// The three spellings are named rather than merely refused because each arrives
// from an author who has written another language, and each has a different
// remedy. The controls at the bottom are the other half: what the check must
// not take over.

func TestAHexLiteralIsRefusedAndOffersItsDecimalValue(t *testing.T) {
	tok := onlyIllegal(t, "let m = 0xFF;")

	if tok.Literal != "0xFF" {
		t.Errorf("the illegal token is %q, want the whole spelling %q", tok.Literal, "0xFF")
	}
	for _, want := range []string{"no hex literal", "write 255", `parse_int("FF", 16)`} {
		if !strings.Contains(tok.Err, want) {
			t.Errorf("the message does not mention %q: %s", want, tok.Err)
		}
	}

	// The token spans the whole spelling, so the range a diagnostic shows is
	// already this fault's own position. Appending one printed it twice: a
	// diagnostic headed `1:9` that ended in "(line 1, column 9)".
	if strings.Contains(tok.Err, "line 1") {
		t.Errorf("the message repeats a position the token's range already gives: %s", tok.Err)
	}
}

func TestAnExponentIsRefusedAndOffersTheValueItSpells(t *testing.T) {
	tok := onlyIllegal(t, "let b = 1.5e3;")

	if tok.Literal != "1.5e3" {
		t.Errorf("the illegal token is %q, want the whole spelling %q", tok.Literal, "1.5e3")
	}
	for _, want := range []string{"no exponent notation", "write 1500"} {
		if !strings.Contains(tok.Err, want) {
			t.Errorf("the message does not mention %q: %s", want, tok.Err)
		}
	}
}

func TestADigitSeparatorIsRefusedAndOffersTheNumberWithoutIt(t *testing.T) {
	tok := onlyIllegal(t, "let n = 1_000;")

	if tok.Literal != "1_000" {
		t.Errorf("the illegal token is %q, want the whole spelling %q", tok.Literal, "1_000")
	}
	for _, want := range []string{"no digit separator", "write 1000"} {
		if !strings.Contains(tok.Err, want) {
			t.Errorf("the message does not mention %q: %s", want, tok.Err)
		}
	}
}

// Anything that is not one of the three has no remedy to offer, so the message
// names the split itself. That is the part the author cannot see: every message
// downstream used to talk about a name they never wrote.
func TestAnyOtherNameAfterDigitsNamesTheSplitItself(t *testing.T) {
	tok := onlyIllegal(t, "let v = 255u8;")

	if tok.Literal != "255u8" {
		t.Errorf("the illegal token is %q, want the whole spelling %q", tok.Literal, "255u8")
	}
	for _, want := range []string{"255u8", "the number 255", "the name u8"} {
		if !strings.Contains(tok.Err, want) {
			t.Errorf("the message does not mention %q: %s", want, tok.Err)
		}
	}
}

// A hex spelling whose digits are not hex has no value to offer, and inventing
// one would be worse than saying less.
func TestAHexSpellingWithNoValueOffersNoValue(t *testing.T) {
	tok := onlyIllegal(t, "let m = 0xZZ;")

	if !strings.Contains(tok.Err, "no hex literal") {
		t.Errorf("the message does not name hex: %s", tok.Err)
	}
	if strings.Contains(tok.Err, "write ") {
		t.Errorf("the message offers a value 0xZZ does not have: %s", tok.Err)
	}
}

// A superscript two is neither a letter nor '_', so the digits end before it
// and it is its own ILLEGAL, as it was. The check must not take that over: it
// would then report a hex literal for a character that is not a name at all.
func TestASuperscriptAfterDigitsIsStillItsOwnIllegalToken(t *testing.T) {
	toks := allTokens(t, "let x = 2²;")

	var sawInt bool
	for _, tok := range toks {
		if tok.Type == token.INT && tok.Literal == "2" {
			sawInt = true
		}
	}
	if !sawInt {
		t.Errorf("the 2 did not lex as its own INT: %s", types(toks))
	}

	tok := onlyIllegal(t, "let x = 2²;")
	if tok.Literal != "²" {
		t.Errorf("the illegal token is %q, want just the superscript", tok.Literal)
	}
}

// What the check must leave alone. A leading zero is refused too, but by
// parseIntegerLiteral, which needs the digits handed to it as an INT to do it.
func TestOrdinaryNumbersAreUntouched(t *testing.T) {
	tests := []struct {
		src     string
		literal string
		want    token.TokenType
	}{
		{"255;", "255", token.INT},
		{"0;", "0", token.INT},
		{"1.5;", "1.5", token.FLOAT},
		{"010.5;", "010.5", token.FLOAT},
		{"0755;", "0755", token.INT},
	}

	for _, tt := range tests {
		t.Run(tt.src, func(t *testing.T) {
			toks := allTokens(t, tt.src)
			for _, tok := range toks {
				if tok.Type == token.ILLEGAL {
					t.Fatalf("%q produced an ILLEGAL (%q: %s): %s",
						tt.src, tok.Literal, tok.Err, types(toks))
				}
			}
			if toks[0].Type != tt.want || toks[0].Literal != tt.literal {
				t.Errorf("%q lexed as %s(%q), want %s(%q)",
					tt.src, toks[0].Type, toks[0].Literal, tt.want, tt.literal)
			}
		})
	}
}
