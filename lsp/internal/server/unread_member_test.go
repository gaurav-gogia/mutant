package server

// M26-LSP-048: definition and hover on the member half of `stats.mean`
// answered from the importer itself when the module behind `stats` could not
// answer for it.
//
// As the importer's resolver reads it, what a member of an import namespace
// refers to is declared in that module or nowhere, and never in the file that
// reaches for it. When the module was unread -- its file missing from disk, or
// open and desynced -- or when the build refused the reach because the module
// does not declare the name or keeps it private, the cross-module answer
// declined and both requests fell through to the importer's file-local ones.
// Definition returned the use's own range as the member's declaration. Hover
// called the member a bare identifier of the importer, and a member spelled
// like a builtin, `stats.sum`, hovered as that builtin's card: wrong
// information, presented as fact. An untitled buffer, whose imports nothing
// resolves, did the same, and so did a use above an enum named like the
// alias: the compiler has not reached the enum there and reads the module,
// while the answers counted the enum for the whole file.
//
// Both now answer nothing there. The controls are the answers that must not go
// with them: a module that is read and in step still answers for its member,
// the alias still answers from the import that declares it, an enum named
// like the alias still answers as the enum below its declaration, a bare
// builtin still hovers as itself, and `p.x` on a value keeps the field
// fallback it has always had.
//
// Every request goes through s.handler, which the stdio router hands each of
// these methods to, on a server taken through initialize and initialized.

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/tliron/glsp"
	lsp "github.com/tliron/glsp/protocol_3_16"
)

// valueFieldLines follow mainImportingStats in the fixtures that need a field
// read on a value: nothing can say which struct p holds, so `p.x` gets the
// field fallback, which is the answer the fix must leave alone.
const valueFieldLines = "let make = fn() { return 1; };\nlet p = make();\nputln(p.x);\n"

var (
	// The alias in mainImportingStats's `stats.mean`, and the alias's own name
	// in its import statement.
	onStatsInMain   = lsp.Position{Line: 1, Character: 10}
	statsAliasRange = lsp.Range{Start: lsp.Position{Line: 0, Character: 7}, End: lsp.Position{Line: 0, Character: 12}}

	// x in `putln(p.x);`, the last of valueFieldLines after mainImportingStats.
	fieldXRange = lsp.Range{Start: lsp.Position{Line: 4, Character: 8}, End: lsp.Position{Line: 4, Character: 9}}

	// mean's name in statsSource.
	meanInStats = lsp.Range{Start: lsp.Position{Line: 0, Character: 4}, End: lsp.Position{Line: 0, Character: 8}}

	// mean in enumNamedLikeTheAlias's `stats.mean`, and the variant it names.
	onEnumMean      = lsp.Position{Line: 2, Character: 15}
	meanVariantDecl = lsp.Range{Start: lsp.Position{Line: 1, Character: 13}, End: lsp.Position{Line: 1, Character: 17}}
)

// enumNamedLikeTheAlias declares an enum under the name an import binds, above
// the use. An enum declared before a use comes first there, in the compiler
// as in ResolveField -- `Colour.Red` predates modules -- so `stats.mean` here
// is the enum's variant, and the file-local answers for it are the right ones.
const enumNamedLikeTheAlias = "import stats \"lib/stats.mut\";\nenum stats { mean };\nlet m = stats.mean;\n"

// initializedServerOver scans root synchronously, the way the background scan
// would, then takes the server through initialize and initialized. initialize
// carries no root, so the scan that initialized starts walks nothing and
// cannot race the requests a test sends.
func initializedServerOver(t *testing.T, root string) *Server {
	t.Helper()
	s := New(false)
	s.setRoots([]string{root})
	s.scanWorkspace()
	initializeServer(t, s)
	return s
}

// askThroughTheHandler sends one request the way a client does and returns its
// result.
func askThroughTheHandler(t *testing.T, s *Server, method string, params any) any {
	t.Helper()
	result, validMethod, validParams, err := s.handler.Handle(&glsp.Context{
		Method: method,
		Params: mustJSON(t, params),
		Notify: func(string, any) {},
	})
	if err != nil || !validMethod || !validParams {
		t.Fatalf("%s: err %v, valid method %t, valid params %t", method, err, validMethod, validParams)
	}
	return result
}

func definitionAt(t *testing.T, s *Server, uri lsp.DocumentUri, pos lsp.Position) any {
	t.Helper()
	return askThroughTheHandler(t, s, string(lsp.MethodTextDocumentDefinition), lsp.DefinitionParams{
		TextDocumentPositionParams: lsp.TextDocumentPositionParams{TextDocument: lsp.TextDocumentIdentifier{URI: uri}, Position: pos},
	})
}

func hoverAt(t *testing.T, s *Server, uri lsp.DocumentUri, pos lsp.Position) any {
	t.Helper()
	return askThroughTheHandler(t, s, string(lsp.MethodTextDocumentHover), lsp.HoverParams{
		TextDocumentPositionParams: lsp.TextDocumentPositionParams{TextDocument: lsp.TextDocumentIdentifier{URI: uri}, Position: pos},
	})
}

// locationOf is the location a definition answer names. The handler answers
// a reach into another module with a Location and its other paths with a
// *Location, and either is a jump.
func locationOf(result any) (lsp.Location, bool) {
	switch location := result.(type) {
	case lsp.Location:
		return location, true
	case *lsp.Location:
		if location != nil {
			return *location, true
		}
	}
	return lsp.Location{}, false
}

// hoverTextOf is a hover answer's text, and false when it answered nothing.
func hoverTextOf(result any) (string, bool) {
	hover, ok := result.(*lsp.Hover)
	if !ok || hover == nil {
		return "", false
	}
	if markup, ok := hover.Contents.(lsp.MarkupContent); ok {
		return markup.Value, true
	}
	return describeAnswer(hover.Contents), true
}

// requireSilence fails unless definition and hover both answer nothing at pos.
func requireSilence(t *testing.T, s *Server, uri lsp.DocumentUri, pos lsp.Position, what string) {
	t.Helper()
	if said := describeAnswer(definitionAt(t, s, uri, pos)); said != "" {
		t.Errorf("definition on %s answered %s; what it refers to is not in this file", what, said)
	}
	if text, ok := hoverTextOf(hoverAt(t, s, uri, pos)); ok {
		t.Errorf("hover on %s answered %q; nothing in this file can describe it", what, text)
	}
}

// requireMemberAnswer checks that definition and hover at pos answer out of
// the module: definition at the member's declaration, hover with its card.
func requireMemberAnswer(t *testing.T, s *Server, uri lsp.DocumentUri, pos lsp.Position, moduleURI lsp.DocumentUri, declaration lsp.Range, what string) {
	t.Helper()
	if location, ok := locationOf(definitionAt(t, s, uri, pos)); !ok || location.URI != moduleURI || location.Range != declaration {
		t.Errorf("definition on %s = %+v (answered %t), want %+v in %s", what, location, ok, declaration, moduleURI)
	}
	if text, ok := hoverTextOf(hoverAt(t, s, uri, pos)); !ok || !strings.Contains(text, "declared in") {
		t.Errorf("hover on %s = %q (answered %t), want the member's card", what, text, ok)
	}
}

// requireNothingFromThisFile checks that definition and hover at pos do not
// answer out of the file they are asked in. Each may answer out of the module,
// as requireMemberAnswer wants, or not at all.
func requireNothingFromThisFile(t *testing.T, s *Server, uri lsp.DocumentUri, pos lsp.Position, moduleURI lsp.DocumentUri, declaration lsp.Range, what string) {
	t.Helper()
	definition := definitionAt(t, s, uri, pos)
	if said := describeAnswer(definition); said != "" {
		if location, ok := locationOf(definition); !ok || location.URI != moduleURI || location.Range != declaration {
			t.Errorf("definition on %s answered %s; want nothing, or %+v in %s", what, said, declaration, moduleURI)
		}
	}
	if text, ok := hoverTextOf(hoverAt(t, s, uri, pos)); ok && !strings.Contains(text, "declared in") {
		t.Errorf("hover on %s answered %q; want nothing, or the member's card", what, text)
	}
}

// requireAliasAnswer checks that the alias in `stats.mean` still answers:
// definition at the import that declares it, and a hover naming it. It is the
// importer's own declaration, whatever became of the module behind it.
func requireAliasAnswer(t *testing.T, s *Server, uri lsp.DocumentUri) {
	t.Helper()
	if location, ok := locationOf(definitionAt(t, s, uri, onStatsInMain)); !ok || location.URI != uri || location.Range != statsAliasRange {
		t.Errorf("definition on the alias stats = %+v (answered %t), want the import's alias, %+v in %s", location, ok, statsAliasRange, uri)
	}
	if text, ok := hoverTextOf(hoverAt(t, s, uri, onStatsInMain)); !ok || !strings.Contains(text, "`stats`") {
		t.Errorf("hover on the alias stats = %q (answered %t), want it described", text, ok)
	}
}

// requireFieldFallback checks the answers `p.x` has always had where nothing
// can say which struct p holds: definition names the field itself and hover
// describes it. No import is involved.
func requireFieldFallback(t *testing.T, s *Server, uri lsp.DocumentUri) {
	t.Helper()
	if location, ok := locationOf(definitionAt(t, s, uri, fieldXRange.Start)); !ok || location.URI != uri || location.Range != fieldXRange {
		t.Errorf("definition on p.x = %+v (answered %t), want the field itself, %+v", location, ok, fieldXRange)
	}
	if text, ok := hoverTextOf(hoverAt(t, s, uri, fieldXRange.Start)); !ok || !strings.Contains(text, "`x`") {
		t.Errorf("hover on p.x = %q (answered %t), want the field described", text, ok)
	}
}

// requireEnumAnswer checks that `stats.mean` in enumNamedLikeTheAlias still
// answers as the enum's variant: definition at the variant, hover naming it.
func requireEnumAnswer(t *testing.T, s *Server, uri lsp.DocumentUri) {
	t.Helper()
	if location, ok := locationOf(definitionAt(t, s, uri, onEnumMean)); !ok || location.URI != uri || location.Range != meanVariantDecl {
		t.Errorf("definition on the variant stats.mean = %+v (answered %t), want the variant, %+v in %s", location, ok, meanVariantDecl, uri)
	}
	if text, ok := hoverTextOf(hoverAt(t, s, uri, onEnumMean)); !ok || !strings.Contains(text, "enum member `mean`") {
		t.Errorf("hover on the variant stats.mean = %q (answered %t), want the enum member", text, ok)
	}
}

// lib/stats.mut was never written, so nothing has read the module behind
// stats: sema answers stats.mean provisionally. In enum.mut beside it, an enum
// declared under the alias's name still answers as the enum.
func TestAMemberOfAMissingModuleAnswersNothing(t *testing.T) {
	text := mainImportingStats + valueFieldLines
	root := writeModules(t, map[string]string{"main.mut": text, "enum.mut": enumNamedLikeTheAlias})
	s := initializedServerOver(t, root)
	uri := pathToURI(filepath.Join(root, "main.mut"))
	openDocument(t, s, uri, text)

	requireSilence(t, s, uri, insideStatsMean, "stats.mean, with lib/stats.mut missing")
	requireAliasAnswer(t, s, uri)
	requireFieldFallback(t, s, uri)

	enumURI := pathToURI(filepath.Join(root, "enum.mut"))
	openDocument(t, s, enumURI, enumNamedLikeTheAlias)
	requireEnumAnswer(t, s, enumURI)
}

// A member spelled like a builtin was the worst of it: hover on stats.sum
// showed the card of the builtin sum, which describes another function
// entirely.
func TestAMemberNamedLikeABuiltinIsNotDescribedAsTheBuiltin(t *testing.T) {
	const text = "import stats \"lib/stats.mut\";\nlet m = stats.sum([1]);\nlet n = sum([1]);\n"
	root := writeModules(t, map[string]string{"main.mut": text})
	s := initializedServerOver(t, root)
	uri := pathToURI(filepath.Join(root, "main.mut"))
	openDocument(t, s, uri, text)

	requireSilence(t, s, uri, insideStatsMean, "stats.sum, with lib/stats.mut missing")

	// Written bare, sum is the builtin, and still hovers as one.
	if hover, ok := hoverTextOf(hoverAt(t, s, uri, lsp.Position{Line: 2, Character: 9})); !ok || !strings.Contains(hover, "builtin `sum") {
		t.Errorf("hover on the bare builtin sum = %q (answered %t), want its card", hover, ok)
	}
}

// The module is read and in step, and the build refuses the reach: it declares
// no median, and _total is private to it. The member it does offer still
// answers out of it.
func TestAMemberTheBuildRefusesAnswersNothing(t *testing.T) {
	const library = "let _total = fn(xs) { return 1; };\nlet mean = fn(xs) { return _total(xs); };\n"
	const text = "import stats \"lib/stats.mut\";\n" +
		"let a = stats.median([1]);\n" +
		"let b = stats._total([1]);\n" +
		"let c = stats.mean([1]);\n"
	root := writeModules(t, map[string]string{"main.mut": text, "lib/stats.mut": library})
	s := initializedServerOver(t, root)
	uri := pathToURI(filepath.Join(root, "main.mut"))
	openDocument(t, s, uri, text)

	requireSilence(t, s, uri, lsp.Position{Line: 1, Character: 15}, "stats.median, which lib/stats.mut does not declare")
	requireSilence(t, s, uri, lsp.Position{Line: 2, Character: 15}, "stats._total, which lib/stats.mut keeps private")
	requireMemberAnswer(t, s, uri, lsp.Position{Line: 3, Character: 15},
		pathToURI(filepath.Join(root, "lib", "stats.mut")),
		lsp.Range{Start: lsp.Position{Line: 1, Character: 4}, End: lsp.Position{Line: 1, Character: 8}},
		"stats.mean, which lib/stats.mut declares")
}

// lib/stats.mut is open and falls out of step: the store refuses a change it
// cannot apply, and the module's facts leave the shared workspace with
// everything else derived from it. To main.mut it is then a module nobody has
// read. A whole-document change brings the answers back.
func TestAMemberOfADesyncedModuleAnswersNothing(t *testing.T) {
	root := writeModules(t, map[string]string{"main.mut": mainImportingStats, "lib/stats.mut": statsSource})
	s := initializedServerOver(t, root)
	statsURI := pathToURI(filepath.Join(root, "lib", "stats.mut"))
	mainURI := pathToURI(filepath.Join(root, "main.mut"))
	openDocument(t, s, statsURI, statsSource)
	openDocument(t, s, mainURI, mainImportingStats)

	requireMemberAnswer(t, s, mainURI, insideStatsMean, statsURI, meanInStats, "stats.mean while lib/stats.mut is in step")
	if t.Failed() {
		t.FailNow()
	}

	changeThroughTheHandler(t, s, unusableChange(statsURI, 2))
	requireDesynced(t, s, statsURI)
	requireSilence(t, s, mainURI, insideStatsMean, "stats.mean, with lib/stats.mut out of step")
	requireAliasAnswer(t, s, mainURI)

	changeThroughTheHandler(t, s, wholeDocumentChange(statsURI, 3, statsSource))
	requireInStep(t, s, statsURI)
	requireMemberAnswer(t, s, mainURI, insideStatsMean, statsURI, meanInStats, "stats.mean once lib/stats.mut is back in step")
}

// A buffer with no file behind it has no module key, so nothing resolves its
// imports and no module can answer for a member. Which aliases its imports
// bind is written in it all the same, and so is whether an enum of an alias's
// name is declared above a use.
func TestAMemberInAnUntitledBufferAnswersNothing(t *testing.T) {
	s := New(false)
	initializeServer(t, s)
	const untitled = lsp.DocumentUri("untitled:Untitled-1")
	openDocument(t, s, untitled, mainImportingStats+valueFieldLines)

	requireSilence(t, s, untitled, insideStatsMean, "stats.mean in an untitled buffer")
	requireAliasAnswer(t, s, untitled)
	requireFieldFallback(t, s, untitled)

	const another = lsp.DocumentUri("untitled:Untitled-2")
	openDocument(t, s, another, enumNamedLikeTheAlias)
	requireEnumAnswer(t, s, another)
}

// A value bound under the alias's name does not hide the module. The compiler
// asks about the import before it asks about any binding, and binds a file's
// aliases for the whole file, so `stats.mean` behind a local `let stats` in a
// function body is lib/stats.mut's mean: parity's
// TestALocalNamedLikeAnImportAliasDoesNotHideIt runs such a program, and it
// returns the module's 42 rather than the local's 7. The editor answers out of
// the module while it is read, and nothing once it is not -- never with the
// local's field, which the build never reads.
func TestALocalNamedLikeTheAliasDoesNotHideTheModule(t *testing.T) {
	const text = "import stats \"lib/stats.mut\";\n" +
		"let f = fn() { let stats = {\"mean\": 7}; return stats.mean; };\n"
	line := strings.Split(text, "\n")[1]
	onMean := lsp.Position{Line: 1, Character: uint32(strings.Index(line, "stats.mean") + len("stats.") + 1)}

	t.Run("module read and in step", func(t *testing.T) {
		root := writeModules(t, map[string]string{"main.mut": text, "lib/stats.mut": statsSource})
		s := initializedServerOver(t, root)
		uri := pathToURI(filepath.Join(root, "main.mut"))
		openDocument(t, s, uri, text)
		requireMemberAnswer(t, s, uri, onMean, pathToURI(filepath.Join(root, "lib", "stats.mut")), meanInStats,
			"stats.mean behind a local stats")
	})

	t.Run("module missing", func(t *testing.T) {
		root := writeModules(t, map[string]string{"main.mut": text})
		s := initializedServerOver(t, root)
		uri := pathToURI(filepath.Join(root, "main.mut"))
		openDocument(t, s, uri, text)
		requireSilence(t, s, uri, onMean, "stats.mean behind a local stats, with lib/stats.mut missing")
	})

	t.Run("untitled buffer", func(t *testing.T) {
		s := New(false)
		initializeServer(t, s)
		const untitled = lsp.DocumentUri("untitled:Untitled-1")
		openDocument(t, s, untitled, text)
		requireSilence(t, s, untitled, onMean, "stats.mean behind a local stats in an untitled buffer")
	})
}

// enumBelowTheUses reaches through stats three times above an enum named like
// the alias, which has a variant of each member's name: mean; sum, spelled
// like a builtin; and _total, which a module keeps private.
const enumBelowTheUses = "import stats \"lib/stats.mut\";\n" +
	"let m = stats.mean([1]);\n" +
	"let n = stats.sum([1]);\n" +
	"let o = stats._total;\n" +
	"enum stats { mean, sum, _total };\n"

// libraryDeclaringTheMembers declares all three of enumBelowTheUses's members.
const libraryDeclaringTheMembers = "let mean = fn(xs) { return 1; };\nlet sum = fn(xs) { return 9; };\nlet _total = 1;\n"

var (
	// The members in enumBelowTheUses.
	onMeanAboveTheEnum  = lsp.Position{Line: 1, Character: 15}
	onSumAboveTheEnum   = lsp.Position{Line: 2, Character: 15}
	onTotalAboveTheEnum = lsp.Position{Line: 3, Character: 15}

	// mean's and sum's names in libraryDeclaringTheMembers.
	meanInLibrary = lsp.Range{Start: lsp.Position{Line: 0, Character: 4}, End: lsp.Position{Line: 0, Character: 8}}
	sumInLibrary  = lsp.Range{Start: lsp.Position{Line: 1, Character: 4}, End: lsp.Position{Line: 1, Character: 7}}
)

// An enum named like the alias comes first only from its declaration on. The
// compiler knows the enums whose statements it has compiled, so at a use
// above `enum stats { ... };` stats is still the import: the build cannot
// find lib/stats.mut, or refuses a member lib/stats.mut does not declare or
// keeps private, or reads the member as lib/stats.mut's declaration. parity's
// TestAnEnumDeclaredBelowTheUseDoesNotHideTheImport runs such programs.
//
// The answers counted the enum for the whole file, so both requests fell
// through to the importer's own: definition on the use's own range, hover
// "identifier `mean`", and on stats.sum the builtin sum's card.
func TestAnEnumDeclaredBelowTheUseDoesNotHideTheModule(t *testing.T) {
	t.Run("module missing", func(t *testing.T) {
		root := writeModules(t, map[string]string{"main.mut": enumBelowTheUses})
		s := initializedServerOver(t, root)
		uri := pathToURI(filepath.Join(root, "main.mut"))
		openDocument(t, s, uri, enumBelowTheUses)

		requireSilence(t, s, uri, onMeanAboveTheEnum, "stats.mean above enum stats, with lib/stats.mut missing")
		requireSilence(t, s, uri, onSumAboveTheEnum, "stats.sum above enum stats, with lib/stats.mut missing")
		requireSilence(t, s, uri, onTotalAboveTheEnum, "stats._total above enum stats, with lib/stats.mut missing")
	})

	t.Run("module refusing the members", func(t *testing.T) {
		root := writeModules(t, map[string]string{"main.mut": enumBelowTheUses, "lib/stats.mut": "let median = 1;\nlet _total = 1;\n"})
		s := initializedServerOver(t, root)
		uri := pathToURI(filepath.Join(root, "main.mut"))
		openDocument(t, s, uri, enumBelowTheUses)

		requireSilence(t, s, uri, onMeanAboveTheEnum, "stats.mean above enum stats, which lib/stats.mut does not declare")
		requireSilence(t, s, uri, onSumAboveTheEnum, "stats.sum above enum stats, which lib/stats.mut does not declare")
		requireSilence(t, s, uri, onTotalAboveTheEnum, "stats._total above enum stats, which lib/stats.mut keeps private")
	})

	// The build reads stats.mean and stats.sum as lib/stats.mut's declarations
	// here, so an answer out of lib/stats.mut would be right. The cross-module
	// answers still count the enum for the whole file, through LocalScopeAt,
	// and decline, so nothing comes back; main.mut's answer must not.
	t.Run("module declaring the members", func(t *testing.T) {
		root := writeModules(t, map[string]string{"main.mut": enumBelowTheUses, "lib/stats.mut": libraryDeclaringTheMembers})
		s := initializedServerOver(t, root)
		uri := pathToURI(filepath.Join(root, "main.mut"))
		statsURI := pathToURI(filepath.Join(root, "lib", "stats.mut"))
		openDocument(t, s, uri, enumBelowTheUses)

		requireNothingFromThisFile(t, s, uri, onMeanAboveTheEnum, statsURI, meanInLibrary, "stats.mean above enum stats, which lib/stats.mut declares")
		requireNothingFromThisFile(t, s, uri, onSumAboveTheEnum, statsURI, sumInLibrary, "stats.sum above enum stats, which lib/stats.mut declares")
		requireSilence(t, s, uri, onTotalAboveTheEnum, "stats._total above enum stats, which lib/stats.mut keeps private")
	})

	t.Run("untitled buffer", func(t *testing.T) {
		s := New(false)
		initializeServer(t, s)
		const untitled = lsp.DocumentUri("untitled:Untitled-1")
		openDocument(t, s, untitled, enumBelowTheUses)

		requireSilence(t, s, untitled, onMeanAboveTheEnum, "stats.mean above enum stats in an untitled buffer")
		requireSilence(t, s, untitled, onSumAboveTheEnum, "stats.sum above enum stats in an untitled buffer")
		requireSilence(t, s, untitled, onTotalAboveTheEnum, "stats._total above enum stats in an untitled buffer")
	})

	// Below its declaration the enum comes first, and the variant still
	// answers, with lib/stats.mut read and declaring a mean of its own.
	t.Run("enum above the use", func(t *testing.T) {
		root := writeModules(t, map[string]string{"main.mut": enumNamedLikeTheAlias, "lib/stats.mut": statsSource})
		s := initializedServerOver(t, root)
		uri := pathToURI(filepath.Join(root, "main.mut"))
		openDocument(t, s, uri, enumNamedLikeTheAlias)

		requireEnumAnswer(t, s, uri)
	})
}
