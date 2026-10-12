package analyzer

import (
	"strings"

	mast "mutant/ast"

	lsp "github.com/tliron/glsp/protocol_3_16"
)

// StringLiteralRef is a string literal's inner-text range (quotes excluded) and
// decoded value. The server uses these to turn string literals that name
// existing files into clickable document links.
type StringLiteralRef struct {
	Range lsp.Range
	Value string
}

// StringLiteralRefs returns every string literal in the document.
func (s *Snapshot) StringLiteralRefs() []StringLiteralRef {
	if s == nil || s.Program == nil || s.Program.NodePositions == nil {
		return nil
	}
	var refs []StringLiteralRef
	for node, rng := range s.Program.NodePositions {
		lit, ok := node.(*mast.StringLiteral)
		if !ok || lit == nil || lit.Value == "" || !rng.IsValid() {
			continue
		}
		converted := s.Range(rng)
		start, end := converted.Start, converted.End
		// Trim the delimiters so the underline covers just the text. They are
		// one character only for an ordinary "..." literal; trimming one from
		// each end of every literal left two quotes inside the underline of a
		// """...""" one and the opening quote inside an r"..." one
		// (M26-SEM-018). Every delimiter character is ASCII, so a count of them
		// is a count of UTF-16 units, and the start and end each lie on their
		// own delimiter's line, so a literal over several lines trims the same.
		open, closing := stringDelimiters(lit)
		if end.Character < closing || (start.Line == end.Line && end.Character < start.Character+open+closing) {
			continue // narrower than its own delimiters: not a range this literal's token produced
		}
		start.Character += open
		end.Character -= closing
		refs = append(refs, StringLiteralRef{Range: lsp.Range{Start: start, End: end}, Value: lit.Value})
	}
	return refs
}

// stringDelimiters returns how many characters open and close a string
// literal's spelling: `"`, `"""`, `r"` or `r"""` before the text, and `"` or
// `"""` after it. The lexer keeps the spelling only for the raw and the
// triple-quoted forms, so an empty one is an ordinary "..." literal.
func stringDelimiters(lit *mast.StringLiteral) (open, closing uint32) {
	spelling := lit.Token.Raw
	if rest, raw := strings.CutPrefix(spelling, "r"); raw {
		open, spelling = 1, rest
	}
	if strings.HasPrefix(spelling, `"""`) {
		return open + 3, 3
	}
	return open + 1, 1
}
