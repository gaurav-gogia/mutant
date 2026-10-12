package analyzer

import (
	"strings"
	"testing"
	"unicode/utf16"

	lsp "github.com/tliron/glsp/protocol_3_16"
)

// coveredText returns the source text a protocol range covers, counting columns
// in UTF-16 code units, as Position.character is.
func coveredText(src string, rng lsp.Range) string {
	lines := strings.Split(src, "\n")
	at := func(p lsp.Position) int {
		offset := 0
		for i := 0; i < int(p.Line); i++ {
			offset += len(lines[i]) + 1
		}
		units := 0
		for i, r := range lines[p.Line] {
			if units >= int(p.Character) {
				return offset + i
			}
			units += len(utf16.Encode([]rune{r}))
		}
		return offset + len(lines[p.Line])
	}
	return src[at(rng.Start):at(rng.End)]
}

// TestAStringLinkCoversOnlyTheTextInsideTheDelimiters pins the underline of a
// path in a string to the path. StringLiteralRefs trimmed one character from
// each end of a single-line literal, as if every literal were "...": a
// triple-quoted one kept two quotes inside the underline on each side, and a
// raw one its opening r" (M26-SEM-018).
func TestAStringLinkCoversOnlyTheTextInsideTheDelimiters(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"an ordinary literal", "let p = \"data/x.log\";\n", "data/x.log"},
		{"a triple-quoted literal", "let p = \"\"\"data/x.log\"\"\";\n", "data/x.log"},
		{"a raw literal", "let p = r\"data/x.log\";\n", "data/x.log"},
		{"a raw triple-quoted literal", "let p = r\"\"\"data/x.log\"\"\";\n", "data/x.log"},
		{"after a name that is not ASCII", "let é = \"\"\"data/x.log\"\"\";\n", "data/x.log"},
		{"an import path", "import lib \"\"\"data/x.log\"\"\";\n", "data/x.log"},
		// Over three lines the delimiters are out, and the block between them,
		// newline and indentation included, is the literal's text.
		{"a literal over three lines", "let p = \"\"\"\n    data/x.log\n    \"\"\";\n", "\n    data/x.log\n    "},
	} {
		t.Run(c.name, func(t *testing.T) {
			refs := New().Analyze(c.src).StringLiteralRefs()
			if len(refs) != 1 {
				t.Fatalf("%d string literal refs, want 1\n%s", len(refs), c.src)
			}
			if got := coveredText(c.src, refs[0].Range); got != c.want {
				t.Errorf("the link covers %q, want %q\n%s", got, c.want, c.src)
			}
		})
	}
}
