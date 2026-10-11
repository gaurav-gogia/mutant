package analyzer

import (
	mast "mutant/ast"

	lsp "github.com/tliron/glsp/protocol_3_16"
)

// RedeclarationAssignmentRange is the range an edit deletes to turn the `let`
// that redeclares the name written at pos into an assignment: from the `let`
// keyword up to the name, so `let x = 2;` becomes `x = 2;`.
//
// That is the remedy the compiler's own refusal names -- "to change this one
// write x = ... instead" -- and it keeps the value. The fix this replaced
// deleted the whole line, so the program compiled and quietly went on using the
// first value (M26-LSP-029).
//
// It declines rather than guesses, for the reasons DeclarationRemovalRange
// gives: a file that does not parse, a name at pos that is not the identifier
// spelled name -- which is how a diagnostic measured in an older copy of the
// text is told apart -- and the `let` in a counted loop's header. It also
// declines for a let binding several names, because the language has no
// assignment to several names at once.
//
// The name check is not a formality. Without it, a refusal of `let x = 2;`
// still pending when the line had become `let y = 2;` offered the same fix,
// and left `y = 2;`: an assignment to a name nothing declares.
func (s *Snapshot) RedeclarationAssignmentRange(pos lsp.Position, name string) (lsp.Range, bool) {
	if s == nil || s.Program == nil || s.Program.NodePositions == nil || len(s.ParseErrors) > 0 {
		return lsp.Range{}, false
	}

	node, identRange, ok := s.NodeAt(pos)
	ident, isIdent := node.(*mast.Identifier)
	if !ok || !isIdent || ident == nil || ident.Value != name || s.Position(identRange.Start) != pos {
		return lsp.Range{}, false
	}

	var declaring *mast.LetStatement
	for candidate := range s.Program.NodePositions {
		if stmt, isLet := candidate.(*mast.LetStatement); isLet && stmt != nil && bindsOnly(stmt, ident) {
			declaring = stmt
		}
	}
	if declaring == nil {
		return lsp.Range{}, false
	}
	for candidate := range s.Program.NodePositions {
		if loop, isLoop := candidate.(*mast.ForStatement); isLoop && loop != nil && loop.Init == declaring {
			return lsp.Range{}, false
		}
	}

	rng, ok := s.Program.RangeOf(declaring)
	if !ok {
		return lsp.Range{}, false
	}
	start, end := rng.Start.Offset, identRange.Start.Offset
	if start < 0 || end <= start || end > len(s.Source) {
		return lsp.Range{}, false
	}
	return s.offsetRange(start, end), true
}
