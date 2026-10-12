package analyzer

import (
	"strings"

	mast "mutant/ast"

	lsp "github.com/tliron/glsp/protocol_3_16"
)

// DeclarationRemovalRange is the range an edit deletes to remove the statement
// that declares the name written at pos, and whether such an edit is safe.
//
// The statement is a `let` binding exactly that one name, or a `struct` or an
// `enum`, standing in a statement list -- the file's own, or a block's. The
// range is the statement's own extent, widened to its whole lines when nothing
// else is written on them, so `let a = 1; putln(a);` loses only the `let` and a
// declaration spread over several lines goes all at once.
//
// It declines rather than guesses:
//
//   - when the file does not parse, because a range next to a parse error is
//     where the parser stopped rather than where the statement ends;
//   - when the name at pos is not an identifier spelled name, which is how a
//     diagnostic measured in an older copy of the text is told apart;
//   - for a `let` binding several names, where deleting the statement deletes
//     the others too;
//   - for the `let` in a counted loop's header, which is not a statement of
//     its own: deleting it would leave `for ( cond; post)`;
//   - for a `struct` or an `enum` declared again under the same name with a
//     different body, because the build accepts both and the later one is
//     the one in effect: removing it from `struct P { x }; struct P { y };
//     P{y: 1};` turned a program that builds into one the compiler refuses.
func (s *Snapshot) DeclarationRemovalRange(pos lsp.Position, name string) (lsp.Range, bool) {
	if s == nil || s.Program == nil || s.Program.NodePositions == nil || len(s.ParseErrors) > 0 {
		return lsp.Range{}, false
	}

	node, identRange, ok := s.NodeAt(pos)
	ident, isIdent := node.(*mast.Identifier)
	if !ok || !isIdent || ident == nil || ident.Value != name || s.Position(identRange.Start) != pos {
		return lsp.Range{}, false
	}

	var declaring mast.Statement
	for candidate := range s.Program.NodePositions {
		switch stmt := candidate.(type) {
		case *mast.LetStatement:
			if stmt != nil && bindsOnly(stmt, ident) && inertInitializer(stmt.Value) {
				declaring = stmt
			}
		case *mast.StructStatement:
			if stmt != nil && stmt.Name == ident {
				declaring = stmt
			}
		case *mast.EnumStatement:
			if stmt != nil && stmt.Name == ident {
				declaring = stmt
			}
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
	if !removesNothingInEffect(s.Program.NodePositions, declaring, name) {
		return lsp.Range{}, false
	}

	rng, ok := s.Program.RangeOf(declaring)
	if !ok {
		return lsp.Range{}, false
	}
	start, end := rng.Start.Offset, rng.End.Offset
	if start < 0 || end <= start || end > len(s.Source) {
		return lsp.Range{}, false
	}
	return s.deletionRange(start, end), true
}

// inertInitializer reports whether evaluating expr can do nothing a program
// could observe: no call, no assignment, no index or field read that can fail,
// no arithmetic that can fail. Deleting a `let` whose initializer is inert
// deletes a name and nothing else; deleting any other `let` also deletes
// whatever its initializer runs.
//
// The set is deliberately small. Anything not listed -- a call, an infix (a
// division can fail at run time), an index, a template with holes -- answers
// false, and a quick fix built on this then declines rather than delete code
// that runs.
func inertInitializer(expr mast.Expression) bool {
	switch e := expr.(type) {
	case *mast.IntegerLiteral, *mast.FloatLiteral, *mast.StringLiteral, *mast.Boolean,
		*mast.Identifier, *mast.FunctionLiteral, *mast.MacroLiteral:
		return e != nil
	case *mast.PrefixExpression:
		if e == nil {
			return false
		}
		// The operator decides as much as the operand. `!` is defined for
		// every value, but `-true` halts the VM ("unsupported object type for
		// negation: BOOLEAN") and so does `~` on anything but an integer, and
		// deleting such a let deletes the failure with it.
		switch e.Right.(type) {
		case *mast.IntegerLiteral:
			return e.Operator == "!" || e.Operator == "-" || e.Operator == "~"
		case *mast.FloatLiteral:
			return e.Operator == "!" || e.Operator == "-"
		case *mast.Boolean:
			return e.Operator == "!"
		}
		return false
	case *mast.ArrayLiteral:
		if e == nil {
			return false
		}
		for _, element := range e.Elements {
			if !inertInitializer(element) {
				return false
			}
		}
		return true
	}
	return false
}

// removesNothingInEffect reports whether deleting declaring leaves the program
// meaning what it meant. Only a type can fail that: a `struct` or an `enum`
// declared twice under one name builds, and the declarations are not dead
// copies of each other unless they say the same thing. Two identical ones are,
// so either may go; two different ones are not, and the fix declines rather
// than guess which one the program's uses were written against.
func removesNothingInEffect(nodes map[mast.Node]mast.Range, declaring mast.Statement, name string) bool {
	switch declaring.(type) {
	case *mast.StructStatement, *mast.EnumStatement:
	default:
		return true
	}
	for candidate := range nodes {
		var other mast.Statement
		switch stmt := candidate.(type) {
		case *mast.StructStatement:
			if stmt != nil && stmt.Name != nil && stmt.Name.Value == name {
				other = stmt
			}
		case *mast.EnumStatement:
			if stmt != nil && stmt.Name != nil && stmt.Name.Value == name {
				other = stmt
			}
		}
		if other != nil && other != declaring && !mast.Equal(other, declaring) {
			return false
		}
	}
	return true
}

// bindsOnly reports whether a let binds ident and nothing else.
func bindsOnly(stmt *mast.LetStatement, ident *mast.Identifier) bool {
	names := stmt.Names
	if len(names) == 0 && stmt.Name != nil {
		names = []*mast.Identifier{stmt.Name}
	}
	return len(names) == 1 && names[0] == ident
}

// deletionRange widens the byte span [start, end) of one statement to whole
// lines when the lines hold nothing else, and otherwise takes the statement
// and the blanks separating it from its neighbour on the line.
func (s *Snapshot) deletionRange(start, end int) lsp.Range {
	src := s.Source
	lineStart := strings.LastIndexByte(src[:start], '\n') + 1
	lineEnd := len(src)
	if i := strings.IndexByte(src[end:], '\n'); i >= 0 {
		lineEnd = end + i
	}
	before, after := src[lineStart:start], src[end:lineEnd]

	switch {
	case isBlankRun(before) && isBlankRun(after):
		if lineEnd < len(src) {
			return s.offsetRange(lineStart, lineEnd+1)
		}
		return s.offsetRange(lineStart, lineEnd)
	case isBlankRun(after):
		return s.offsetRange(lineStart+len(strings.TrimRight(before, " \t")), end)
	default:
		next := end
		for next < lineEnd && (src[next] == ' ' || src[next] == '\t') {
			next++
		}
		return s.offsetRange(start, next)
	}
}

func isBlankRun(text string) bool {
	return strings.Trim(text, " \t\r") == ""
}

func (s *Snapshot) offsetRange(start, end int) lsp.Range {
	return lsp.Range{Start: s.offsetPosition(start), End: s.offsetPosition(end)}
}

// offsetPosition is the protocol position of a byte offset into Source.
func (s *Snapshot) offsetPosition(offset int) lsp.Position {
	src := s.Source[:offset]
	line := strings.Count(src, "\n")
	column := offset - (strings.LastIndexByte(src, '\n') + 1)
	return s.PositionAt(line, column)
}
