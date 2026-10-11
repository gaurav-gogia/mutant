package analyzer

import (
	"sort"
	"strings"

	mast "mutant/ast"
	"mutant/sema"

	lsp "github.com/tliron/glsp/protocol_3_16"
)

// The cross-module half of name resolution, which the language server has never
// had. It asks sema exactly what the compiler asks it, so an answer the editor
// gives is an answer the build will agree with.

// moduleMemberSource marks these as the compiler's own refusals rather than a
// lint: they are things the build will reject, not style advice.
const moduleMemberSource = "mutant-modules"

// localScopeAt is what the workspace cannot know: which names this file has
// bound at pos, and which enums it declares.
//
// The split is the whole design. Everything file-local comes from this file's
// graph, everything cross-module from the workspace, and one resolver puts them
// together with the precedence the compiler uses.
//
// The graph answers rather than this package, because the question is not an
// editor question. Assembling it here from a name match over VisibleBindingsAt
// is what this used to do, and it counted a struct name as a binding -- which
// VisibleAt is right to return, since a bare `Point` has to resolve to its
// struct, and which the compiler's Bound has never counted, since a type name
// is not in the symbol table. The two disagreed about every file that declares
// a struct named after a builtin family.
func (s *Snapshot) localScopeAt(pos lsp.Position) sema.LocalScope {
	return s.Graph().LocalScopeAt(s.TokenPosition(pos))
}

// localScopeAtNode is localScopeAt for a caller holding a node rather than a
// cursor: what the file has bound where the node begins.
//
// A walker asks this instead of keeping a scope chain of its own. Keeping one
// means deciding, again and separately, which AST nodes declare a name -- and
// the walkers that did decided it differently from the compiler, counting a
// struct name as a binding. A node with no recorded range gets the zero
// LocalScope, which binds nothing: the same answer the chain gave when it had
// not reached that part of the file.
func (s *Snapshot) localScopeAtNode(node mast.Node) sema.LocalScope {
	if s == nil || s.Program == nil || node == nil {
		return sema.LocalScope{}
	}
	rng, ok := s.Program.RangeOf(node)
	if !ok {
		return sema.LocalScope{}
	}
	return s.localScopeAtRange(rng)
}

// localScopeAtRange is localScopeAt for a caller that already holds a range in
// the AST's own coordinates.
//
// It exists so such a caller does not convert the range to a protocol position
// and immediately back again. That round trip is what startOf was for, and a
// conversion that is undone a line later is one that can be wrong in both
// directions at once -- which is exactly what M26-LSP-027 was.
func (s *Snapshot) localScopeAtRange(rng mast.Range) sema.LocalScope {
	return s.Graph().LocalScopeAt(rng.Start.Line, rng.Start.Column)
}

// namespaceField is one `ns.member` in the document, with the range to report
// at. Left is always an identifier: `a.b.c` is a field read on a value, not a
// reach into a module, and the module rules have nothing to say about it.
type namespaceField struct {
	node  *mast.FieldExpression
	rng   mast.Range
	left  string
	field string
}

// namespaceFields collects every `ident.member` in the document, in source
// order.
//
// Source order matters: NodePositions is a map, and diagnostics built by
// ranging over it would arrive in a different order on every keystroke, which
// clients render as the list flickering.
func (s *Snapshot) namespaceFields() []namespaceField {
	if s == nil || s.Program == nil || s.Program.NodePositions == nil {
		return nil
	}

	fields := make([]namespaceField, 0, 8)
	for node, rng := range s.Program.NodePositions {
		field, isField := node.(*mast.FieldExpression)
		if !isField || field == nil || field.Field == nil {
			continue
		}
		left, isIdent := field.Left.(*mast.Identifier)
		if !isIdent || left == nil {
			continue
		}
		fields = append(fields, namespaceField{node: field, rng: rng, left: left.Value, field: field.Field.Value})
	}
	sort.Slice(fields, func(i, j int) bool {
		return fields[i].rng.Start.Offset < fields[j].rng.Start.Offset
	})
	return fields
}

// fieldExpressionAt returns the innermost `ident.member` the position falls
// inside, so `a.b.c` answers about `.c` when the cursor is on `c`.
func (s *Snapshot) fieldExpressionAt(pos lsp.Position) (namespaceField, bool) {
	var (
		best  namespaceField
		found bool
	)
	line, column := s.TokenPosition(pos)
	for _, candidate := range s.namespaceFields() {
		if !rangeContains(candidate.rng, line, column) {
			continue
		}
		span := candidate.rng.End.Offset - candidate.rng.Start.Offset
		if !found || span < best.rng.End.Offset-best.rng.Start.Offset {
			best, found = candidate, true
		}
	}
	return best, found
}

// ModuleMemberTarget names the declaration `ns.member` refers to: the module it
// lives in, its name there, and where it is written.
//
// Go-to-definition wants only the last of those. Find-references and rename want
// the identity, because "every use of this" is a question about a declaration
// and not about a place -- the same declaration is reached through a different
// alias in every file that imports it, and under two aliases in a file that
// imports it twice.
//
// It answers only for a Certain resolution. A module the workspace has not read
// yet yields Provisional, and the editor must then show nothing: a jump to a
// guess is worse than no jump, because the user cannot tell the difference until
// they are already looking at the wrong file.
func (s *Snapshot) ModuleMemberTarget(pos lsp.Position) (moduleKey, name string, declaration lsp.Location, ok bool) {
	if s == nil || s.workspace == nil || s.ModuleKey == "" {
		return "", "", lsp.Location{}, false
	}
	field, found := s.fieldExpressionAt(pos)
	if !found {
		return "", "", lsp.Location{}, false
	}

	owner, uri, declRange, resolved := s.workspace.DefinitionOf(
		s.ModuleKey, s.localScopeAt(pos), field.left, field.field,
	)
	if !resolved || uri == "" || !declRange.IsValid() {
		return "", "", lsp.Location{}, false
	}
	return owner, field.field, lsp.Location{
		URI:   lsp.DocumentUri(uri),
		Range: s.Range(declRange),
	}, true
}

// ModuleMemberDefinition locates the declaration `ns.member` refers to, when the
// position sits on such an expression and ns is an import namespace.
func (s *Snapshot) ModuleMemberDefinition(pos lsp.Position) (lsp.Location, bool) {
	_, _, declaration, ok := s.ModuleMemberTarget(pos)
	if !ok {
		return lsp.Location{}, false
	}
	return declaration, true
}

// ModuleMemberNameAt reports whether the position is on the member half of an
// `ns.member` that this file's resolver reads as a reach into another module,
// because ns is an alias one of this file's imports binds -- whether or not the
// module behind it has been read.
//
// To that reading the name belongs to the module: what it refers to is
// declared there or nowhere, and never in this file. ModuleMemberDefinition
// and ModuleMemberHover decline when nothing has read the module for this
// file, when the build refuses the reach because the module keeps the name
// private or does not declare it, and when this file declares an enum of the
// alias's name below the use, which they count for the whole file. A caller
// must then show nothing rather than fall through to the file-local answers,
// which answer as though the member were a bare name in this file:
// go-to-definition lands on the use itself, and hover calls it an identifier,
// or shows the card of the builtin that shares its spelling.
//
// A value bound under the alias's name changes none of this, wherever it is
// bound. The compiler asks about the import before it asks about any binding,
// and binds a file's aliases for the whole file, so behind
// `let stats = {"mean": 1};` in a function body `stats.mean` is still a reach
// into the module. Resolving `stats` through this file's graph would find the
// let, and would answer for a field the build never reads.
//
// An enum of the alias's name does change it, from the enum's declaration on.
// The compiler asks about an enum before the import, as ResolveField does, and
// knows the enums whose statements it has compiled so far: `stats.mean`
// written above `enum stats { mean };` still reads the module, and below it
// reads the enum (parity's TestAnEnumDeclaredBelowTheUseDoesNotHideTheImport
// runs both). So the enum is asked about with sema's EnumDeclaredBefore, at
// the start of the use, and not with LocalScopeAt's Enums, which counts it for
// the whole file and which the other resolutions in this package still use.
//
// The reading is this file's, and the build's can differ from it. An enum of
// the alias's name declared by a module compiled before this one is one the
// compiler knows, because its enums are the program's and not the file's, so
// the build reads `stats.mean` as that enum's variant, while this file's graph
// holds only the enums this file declares. Where the module behind the alias
// declares no mean, or has not been read, this reports true there and a
// caller shows nothing where the build has an answer: an answer missing rather
// than a wrong one. Where the module does declare mean, ModuleMemberDefinition
// and ModuleMemberHover answer with that declaration, which is not what the
// build reads: the same gap, seen from the other side. Source order and the
// compiler's order also part in three places, where this file's reading is
// not the build's (M26-LSP-050). The compiler compiles a for loop's post
// section after the loop's body: a use in the post reads an enum of the
// alias's name that the body declares, where in source order the enum comes
// after the use and this reads the module, and a use in the body reads the
// module where an enum declared in the post comes first in source and this
// reports false. It never compiles a top-level macro's body, whose enum this
// still counts. And it compiles a macro's template where the macro is called,
// which can be above an enum the template is written below.
//
// Only the member half counts. `stats` in `stats.mean` names the alias, which
// this file declares, so a caller still asks the file-local answers about it.
func (s *Snapshot) ModuleMemberNameAt(pos lsp.Position) bool {
	if s == nil || s.Program == nil {
		return false
	}
	field, found := s.fieldExpressionAt(pos)
	if !found {
		return false
	}
	line, column := s.TokenPosition(pos)
	if rng, ok := s.Program.RangeOf(field.node.Field); !ok || !rangeContains(rng, line, column) {
		return false
	}
	graph, use := s.Graph(), field.rng.Start
	local := s.localScopeAt(pos)
	local.Enums = func(name string) bool {
		return graph.EnumDeclaredBefore(name, use.Line, use.Column)
	}
	if s.workspace != nil && s.ModuleKey != "" {
		switch s.workspace.ResolveField(s.ModuleKey, local, field.left, field.field).Kind {
		case sema.FieldModuleMember, sema.FieldRefused:
			return true
		}
		return false
	}
	// With no workspace or no module key -- an untitled buffer, which has no
	// path -- nothing resolves this file's imports, so nothing has read the
	// module behind an alias. Which aliases the imports bind is written here
	// all the same, and that is all ResolveField's rule asks: an alias names
	// its module unless an enum of that name is declared before the use.
	if local.Enums(field.left) {
		return false
	}
	_, imported := importNamespaces(s.Program.Statements)[field.left]
	return imported
}

// ModuleMemberDiagnostics reports the reaches into another module that the
// compiler will refuse: a private name, and a name the module does not declare.
//
// The sentence is sema's, unchanged, so the squiggle and the build error are
// the same words rather than two phrasings of one rule.
//
// Nothing is reported for a module the workspace has not read. The underscore
// rule is about the name and holds regardless, but "declares no such name"
// about an unread file would be a refusal invented out of ignorance -- sema
// marks that case Provisional and it never reaches here.
func (s *Snapshot) ModuleMemberDiagnostics() []lsp.Diagnostic {
	if s == nil || s.workspace == nil || s.ModuleKey == "" {
		return nil
	}

	var diagnostics []lsp.Diagnostic
	for _, field := range s.namespaceFields() {
		// The scope is taken at the expression's own start. That is exact for
		// the question being asked: whether `ns` is an import namespace is a
		// property of the file, not of the point inside it.
		resolved := s.workspace.ResolveField(
			s.ModuleKey, s.localScopeAtRange(field.rng), field.left, field.field,
		)
		if resolved.Kind != sema.FieldRefused || resolved.Refusal == nil {
			continue
		}

		severity := lsp.DiagnosticSeverityError
		source := moduleMemberSource
		diagnostics = append(diagnostics, lsp.Diagnostic{
			Range:    s.Range(field.rng),
			Severity: &severity,
			Source:   &source,
			Message:  resolved.Refusal.Error(),
		})
	}
	return diagnostics
}

// ModuleMemberHover describes the declaration `ns.member` refers to.
//
// It names the module, because that is the thing the reader cannot see from
// where they are standing -- the point of an import is that the code is
// elsewhere.
//
// A function shows its parameter names and no types. Mutant has no parameter
// annotations, and this package's inferred kinds are per-snapshot and
// explicitly heuristic; one file's guess about another file's function would be
// a guess presented as a fact. Nor is the declaration's leading comment shown:
// carrying it would mean holding text from a file this document does not own,
// and it is not in ModuleFacts today.
func (s *Snapshot) ModuleMemberHover(pos lsp.Position) (string, mast.Range, bool) {
	if s == nil || s.workspace == nil || s.ModuleKey == "" {
		return "", mast.Range{}, false
	}
	field, ok := s.fieldExpressionAt(pos)
	if !ok {
		return "", mast.Range{}, false
	}

	export, declaredIn, found := s.workspace.MemberFact(
		s.ModuleKey, s.localScopeAt(pos), field.left, field.field,
	)
	if !found {
		return "", mast.Range{}, false
	}

	signature := export.Name
	kind := "value"
	if export.Kind == sema.SymFunction {
		kind = "function"
		signature += "(" + strings.Join(export.Params, ", ") + ")"
	}

	text := "```mutant\n" + signature + "\n```\n\n" +
		kind + " `" + export.Name + "`, declared in `" + declaredIn + "`"
	return text, field.rng, true
}

// ImportDiagnostics reports what module.Load will refuse about this file's
// imports, in the loader's own words: a second import binding an alias the
// first already bound, and an import through which the loader reaches an
// import cycle.
//
// Like ModuleMemberDiagnostics these are the build's refusals and not lints, so
// they take no LintConfig, and without a workspace they say nothing. An import
// the workspace has not indexed is not reported at all: not indexed is not
// missing -- the scan is bounded and the server sets no --module-path -- and
// "no such file" about a file that exists is a refusal the build never makes.
func (s *Snapshot) ImportDiagnostics() []lsp.Diagnostic {
	if s == nil || s.workspace == nil || s.ModuleKey == "" {
		return nil
	}
	var diagnostics []lsp.Diagnostic
	for _, refused := range s.workspace.Diagnostics(s.ModuleKey) {
		if refused.Code == sema.DiagUnresolvedImport || refused.Module != s.ModuleKey {
			continue
		}
		severity := lsp.DiagnosticSeverityError
		source := moduleMemberSource
		diagnostics = append(diagnostics, lsp.Diagnostic{
			Range:    s.Range(refused.Range),
			Severity: &severity,
			Source:   &source,
			Message:  refused.Message,
		})
	}
	return diagnostics
}
