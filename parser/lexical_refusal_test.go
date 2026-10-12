package parser

import (
	"strings"
	"testing"
)

// The parser half of lexer/malformed_number_test.go and lexer/backslash_test.go,
// in the spirit of lexer_fault_test.go beside it: what the lexer refuses has to
// reach the author as ONE positioned error carrying the lexer's own sentence,
// not as the parser's guess at one.
//
// Both halves matter because a statement boundary is not where most of these
// appear. `let m = 0xFF;` was two statements; `putln(0xFF)` was inside an
// argument list, where no `;` is missing and the author got
// `expected next token to be , or ), but got IDENT instead` -- a sentence about
// punctuation for someone who wrote a hex literal. Every test here fails at
// 4360eec (M26-LEX-007, M26-LEX-016).

func TestAHexLiteralInAnExpressionCostsOneErrorAndNamesHex(t *testing.T) {
	fault := oneFault(t, "putln(0xFF + 1);")

	if !strings.Contains(fault.Msg, "no hex literal") {
		t.Fatalf("the error does not name hex: %q", fault.Msg)
	}
	if strings.Contains(fault.Msg, "expected next token") {
		t.Errorf("the error is the parser's sentence about punctuation: %q", fault.Msg)
	}
	// Column 7 is the 0 of 0xFF, which is the whole malformed spelling's start.
	if got := fault.Range.Start.Column; got != 7 {
		t.Errorf("the error is at column %d, want 7 (the 0 of 0xFF)", got)
	}
}

// The `+ 1` is what made this two errors rather than one: the element was
// refused, and the argument list then complained about the token left over.
func TestAHexLiteralInAnArrayCostsOneError(t *testing.T) {
	fault := oneFault(t, "let a = [0xFF, 2];")

	if !strings.Contains(fault.Msg, "no hex literal") {
		t.Fatalf("the error does not name hex: %q", fault.Msg)
	}
}

func TestABackslashInAnArgumentListCarriesTheLexersSentence(t *testing.T) {
	fault := oneFault(t, `putln(6 \ 3);`)

	if !strings.Contains(fault.Msg, "not an operator") {
		t.Fatalf("the error does not say a backslash is not an operator: %q", fault.Msg)
	}
	if strings.Contains(fault.Msg, "expected next token") {
		t.Errorf("the error is the parser's sentence about punctuation: %q", fault.Msg)
	}
}

func TestABackslashInsideALongerExpressionStillCostsOneError(t *testing.T) {
	fault := oneFault(t, `putln(6 \ 3 + 1);`)

	if !strings.Contains(fault.Msg, "not an operator") {
		t.Fatalf("the error does not say a backslash is not an operator: %q", fault.Msg)
	}
}

// The control for suppressing the punctuation complaint: a list that really is
// missing a comma still says so, because nothing else reported that position.
func TestAListMissingACommaStillSaysSo(t *testing.T) {
	fault := oneFault(t, "putln(1 2);")

	if !strings.Contains(fault.Msg, "expected next token") {
		t.Fatalf("a list missing a comma no longer says so: %q", fault.Msg)
	}
}

// And the control for the refusals themselves: the shapes they are modelled on
// still parse with nothing said.
func TestWellFormedArithmeticStillParses(t *testing.T) {
	sources := []string{
		"putln(6 / 3);",
		"let a = [255, 2];",
		"putln(255 + 1);",
		`putln("C:\\evidence");`,
		`putln(r"C:\evidence\");`,
	}

	for _, src := range sources {
		t.Run(src, func(t *testing.T) {
			plain, _ := faults(src)
			if len(plain) != 0 {
				t.Fatalf("%s produced %v", src, plain)
			}
		})
	}
}
