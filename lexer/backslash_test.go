package lexer

import (
	"strings"
	"testing"

	"mutant/token"
)

// A backslash outside a string lexed as the division token, carrying `\` as its
// literal. So `6 \ 3` was a well-formed infix expression as far as the parser
// could tell, the editor showed no diagnostic at all, and the only complaint
// came from the compiler, as `unknown operator \` with no position on it. These
// tests fail at 4360eec (M26-LEX-016).

func TestABackslashIsItsOwnIllegalTokenAndSaysWhy(t *testing.T) {
	tok := onlyIllegal(t, `putln(6 \ 3);`)

	if tok.Literal != `\` {
		t.Errorf("the illegal token is %q, want a single backslash", tok.Literal)
	}
	if !strings.Contains(tok.Err, "not an operator") {
		t.Errorf("the message does not say a backslash is not an operator: %q", tok.Err)
	}
	// Worth naming both remedies, because the two reasons an author writes one
	// here are integer division and a Windows path.
	for _, want := range []string{"/", "string literal"} {
		if !strings.Contains(tok.Err, want) {
			t.Errorf("the message does not mention %q: %q", want, tok.Err)
		}
	}
}

// The control: a forward slash is still the operator, which is the thing the
// backslash was being mistaken for.
func TestADivisionIsStillADivision(t *testing.T) {
	toks := allTokens(t, "putln(6 / 3);")

	var slashes int
	for _, tok := range toks {
		if tok.Type == token.ILLEGAL {
			t.Fatalf("a division produced an ILLEGAL (%q): %s", tok.Literal, types(toks))
		}
		if tok.Type == token.FSLASH {
			slashes++
			if tok.Literal != "/" {
				t.Errorf("the division token's literal is %q, want /", tok.Literal)
			}
		}
	}
	if slashes != 1 {
		t.Errorf("a division produced %d FSLASH tokens, want 1: %s", slashes, types(toks))
	}
}

// The other control, and the one that matters to this language's readers: a
// backslash inside a string literal is text and always was, in both spellings.
// The raw form ending in one is the shape a Windows path takes.
func TestABackslashInsideAStringIsStillText(t *testing.T) {
	sources := []string{
		`let p = "C:\\evidence";`,
		`let p = r"C:\evidence\";`,
		`let p = """C:\evidence""";`,
	}

	for _, src := range sources {
		t.Run(src, func(t *testing.T) {
			toks := allTokens(t, src)
			for _, tok := range toks {
				if tok.Type == token.ILLEGAL {
					t.Fatalf("%s produced an ILLEGAL (%q: %s): %s",
						src, tok.Literal, tok.Err, types(toks))
				}
			}
		})
	}
}
