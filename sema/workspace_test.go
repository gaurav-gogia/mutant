package sema

import (
	"path/filepath"
	"strings"
	"testing"

	"mutant/ast"
	"mutant/lexer"
	"mutant/parser"
)

// The workspace is what the language server has never had: knowledge of what an
// import means. These tests are written against parsed source rather than
// hand-built facts, because the thing being checked is the reading of a file as
// much as the answering of a question.

func parse(t *testing.T, src string) *ast.Program {
	t.Helper()
	p := parser.New(lexer.New(src))
	program := p.ParseProgram()
	if errs := p.Errors(); len(errs) > 0 {
		t.Fatalf("fixture did not parse: %v", errs)
	}
	return program
}

// workspaceOf builds a workspace from a map of relative path to source, rooted
// at a directory that does not exist. Nothing here touches the filesystem, and
// that is the point: PutFile and every query below run on the keystroke path.
func workspaceOf(t *testing.T, files map[string]string) (*Workspace, func(string) string) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "proj")

	w := NewWorkspace(nil)
	key := func(rel string) string { return CanonicalKey(filepath.Join(root, rel)) }
	for rel, src := range files {
		path := filepath.Join(root, rel)
		w.PutFile("file:///"+filepath.ToSlash(path), path, parse(t, src))
	}
	return w, key
}

// The shape of examples/modules, which is the golden fixture: a transitive
// import, an alias, a private name used inside its own module, and one name
// declared in two modules.
var moduleFixture = map[string]string{
	"main.mut": `
import report "lib/report.mut";
import numbers "lib/stats.mut";
let label = "main";
let main = fn() { report.render([1, 2]); };
`,
	"lib/report.mut": `
import stats "stats.mut";
let label = "report";
let render = fn(xs) { return stats.mean(xs); };
`,
	"lib/stats.mut": `
let _total = fn(xs) { return 0; };
let mean = fn(xs) { return _total(xs); };
let max = fn(xs) { return 0; };
`,
}

func TestAnImportBindsTheFileItNames(t *testing.T) {
	w, key := workspaceOf(t, moduleFixture)

	bound := w.Namespaces(key("main.mut"))
	if got := bound["report"]; got != key("lib/report.mut") {
		t.Fatalf("report is bound to %q, want %q", got, key("lib/report.mut"))
	}
	// An alias names the file, not the file's own name: `numbers` is stats.mut.
	if got := bound["numbers"]; got != key("lib/stats.mut") {
		t.Fatalf("numbers is bound to %q, want %q", got, key("lib/stats.mut"))
	}
}

// An import path is relative to the importing file, so report.mut's
// `import "stats.mut"` means lib/stats.mut and not a stats.mut beside main.
func TestAnImportIsRelativeToTheImporterAndNotToTheEntryPoint(t *testing.T) {
	w, key := workspaceOf(t, moduleFixture)
	if got := w.Namespaces(key("lib/report.mut"))["stats"]; got != key("lib/stats.mut") {
		t.Fatalf("stats resolved to %q, want %q", got, key("lib/stats.mut"))
	}
}

// This is the bug the whole step exists to fix. `stats.mean` had no code path
// at all in the editor.
func TestAModuleMemberResolvesToItsDeclaration(t *testing.T) {
	w, key := workspaceOf(t, moduleFixture)

	moduleKey, uri, declRange, found := w.DefinitionOf(key("lib/report.mut"), LocalScope{}, "stats", "mean")
	if !found {
		t.Fatal("stats.mean did not resolve")
	}
	if moduleKey != key("lib/stats.mut") {
		t.Fatalf("stats.mean resolved into %q, want %q", moduleKey, key("lib/stats.mut"))
	}
	if uri == "" {
		t.Fatal("resolution carries no URI, so the editor cannot open it")
	}
	if !declRange.IsValid() {
		t.Fatal("resolution carries no range, so there is nowhere to jump to")
	}
}

// docs/MODULES.md blesses two modules declaring one name, and examples/modules
// demonstrates it. Today that makes both unresolvable, because the index
// matches by name across every document and bails on the ambiguity.
func TestOneNameInTwoModulesResolvesToTheImportedOne(t *testing.T) {
	w, key := workspaceOf(t, moduleFixture)

	// `label` is declared in both main.mut and lib/report.mut.
	if _, known := w.Facts(key("main.mut")); !known {
		t.Fatal("main.mut is not indexed")
	}
	moduleKey, _, _, found := w.DefinitionOf(key("main.mut"), LocalScope{}, "report", "label")
	if !found {
		t.Fatal("report.label did not resolve")
	}
	if moduleKey != key("lib/report.mut") {
		t.Fatalf("report.label resolved into %q, want report.mut", moduleKey)
	}
}

func TestAPrivateMemberIsRefusedAcrossFiles(t *testing.T) {
	w, key := workspaceOf(t, moduleFixture)

	resolved := w.ResolveField(key("lib/report.mut"), LocalScope{}, "stats", "_total")
	if resolved.Kind != FieldRefused || resolved.Refusal == nil {
		t.Fatalf("stats._total resolved as %v, want a refusal", resolved.Kind)
	}
	if resolved.Refusal.Code != RefusePrivateMember {
		t.Fatalf("refusal code = %v, want RefusePrivateMember", resolved.Refusal.Code)
	}
	// And go-to-definition must decline, not jump to the private declaration.
	if _, _, _, found := w.DefinitionOf(key("lib/report.mut"), LocalScope{}, "stats", "_total"); found {
		t.Fatal("go-to-definition jumped to a name the compiler refuses")
	}
}

// A module is not in scope because it exists; it is in scope because this file
// imported it. stats.mut is indexed and main.mut does import it under an alias,
// but `report.mean` is still nothing.
func TestAMemberOfTheWrongModuleDoesNotResolve(t *testing.T) {
	w, key := workspaceOf(t, moduleFixture)

	resolved := w.ResolveField(key("main.mut"), LocalScope{}, "report", "mean")
	if resolved.Kind != FieldRefused {
		t.Fatalf("report.mean resolved as %v, want a refusal", resolved.Kind)
	}
	if resolved.Refusal.Code != RefuseNoSuchMember {
		t.Fatalf("refusal code = %v, want RefuseNoSuchMember", resolved.Refusal.Code)
	}
}

// An alias nobody imported is not a namespace, and must fall through to field
// access on a value rather than becoming a refusal.
func TestAnUnimportedNameIsNotANamespace(t *testing.T) {
	w, key := workspaceOf(t, moduleFixture)

	if got := w.ResolveField(key("main.mut"), LocalScope{}, "stats", "mean"); got.Kind != FieldValueAccess {
		t.Fatalf("stats.mean in main.mut (which does not import it as `stats`) = %v, want FieldValueAccess", got.Kind)
	}
}

func TestMembersOfferedBehindNamespaceDotExcludePrivateNames(t *testing.T) {
	w, key := workspaceOf(t, moduleFixture)

	var names []string
	for _, member := range w.MembersOf(key("lib/report.mut"), "stats") {
		names = append(names, member.Name)
	}
	want := []string{"max", "mean"}
	if len(names) != len(want) {
		t.Fatalf("stats. offers %v, want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("stats. offers %v, want %v", names, want)
		}
	}
}

// Nothing to say is the required answer when the target has not been read.
// Offering an empty list is a claim that the module declares nothing.
func TestAnUnindexedTargetOffersNothingRatherThanAnEmptyList(t *testing.T) {
	w, key := workspaceOf(t, map[string]string{
		"main.mut": `import gone "lib/gone.mut"; let x = 1;`,
	})
	if members := w.MembersOf(key("main.mut"), "gone"); members != nil {
		t.Fatalf("an unread module offered %v", members)
	}
	// And the resolution hedges rather than refusing: saying "declares no such
	// name" about a file nobody has read is a refusal invented out of ignorance.
	resolved := w.ResolveField(key("main.mut"), LocalScope{}, "gone", "anything")
	if resolved.Kind == FieldRefused {
		t.Fatalf("an unread module produced a refusal: %v", resolved.Refusal)
	}
}

func TestTheClosureIsEveryModuleReachableByImports(t *testing.T) {
	w, key := workspaceOf(t, moduleFixture)

	closure, diags := w.Closure(key("main.mut"))
	if len(diags) != 0 {
		t.Fatalf("a clean fixture produced diagnostics: %v", diags)
	}
	want := map[string]bool{
		key("main.mut"): true, key("lib/report.mut"): true, key("lib/stats.mut"): true,
	}
	if len(closure) != len(want) {
		t.Fatalf("closure = %v, want %d modules", closure, len(want))
	}
	for _, reached := range closure {
		if !want[reached] {
			t.Fatalf("closure reached %q, which nothing imports", reached)
		}
	}
}

// A cycle must be reported and walked through, never refused. An author creates
// them constantly while moving code between files, and an editor that stopped
// answering would be useless exactly then.
func TestACycleIsReportedAndDoesNotStopTheWalk(t *testing.T) {
	w, key := workspaceOf(t, map[string]string{
		"a.mut": `import b "b.mut"; let a = 1;`,
		"b.mut": `import a "a.mut"; let b = 1;`,
	})

	closure, diags := w.Closure(key("a.mut"))
	if len(closure) != 2 {
		t.Fatalf("closure = %v, want both modules despite the cycle", closure)
	}
	if len(diags) != 1 || diags[0].Code != DiagImportCycle {
		t.Fatalf("diagnostics = %v, want exactly one import cycle", diags)
	}
	// And members still resolve through it: the file is readable, it just may
	// not build.
	if got := w.ResolveField(key("a.mut"), LocalScope{}, "b", "b"); got.Kind != FieldModuleMember {
		t.Fatalf("b.b through a cycle = %v, want FieldModuleMember", got.Kind)
	}
}

// Two modules declaring `label` is a legal program. Having the information to
// notice it is not a reason to say anything about it.
func TestALegalProgramProducesNoDiagnostics(t *testing.T) {
	w, key := workspaceOf(t, moduleFixture)
	for _, rel := range []string{"main.mut", "lib/report.mut", "lib/stats.mut"} {
		if diags := w.Diagnostics(key(rel)); len(diags) != 0 {
			t.Fatalf("%s produced %v", rel, diags)
		}
	}
}

func TestTwoImportsBindingOneAliasAreReported(t *testing.T) {
	w, key := workspaceOf(t, map[string]string{
		"main.mut":   `import "a/util.mut"; import "b/util.mut"; let x = 1;`,
		"a/util.mut": `let f = fn() { return 1; };`,
		"b/util.mut": `let g = fn() { return 2; };`,
	})

	diags := w.Diagnostics(key("main.mut"))
	if len(diags) != 1 || diags[0].Code != DiagDuplicateNamespace {
		t.Fatalf("diagnostics = %v, want one duplicate namespace", diags)
	}
	// The first import wins, matching module.Load.
	if got := w.Namespaces(key("main.mut"))["util"]; got != key("a/util.mut") {
		t.Fatalf("util is bound to %q, want a/util.mut", got)
	}
}

// M26-SEM-006: the build refuses an import cycle, and Diagnostics -- what is
// wrong with one file's imports -- never reported one. Only Closure computed a
// cycle, and only at the import that closes it: in the last file of the loop,
// which is never the file asked about unless that file imports itself, with the
// chain spelled in canonical keys and trimmed to the module it re-enters.
//
// Diagnostics(key) reports, at each of key's imports, the first refusal a build
// entered at key meets through it: the cycle as module.CycleError prints it,
// every file from key down to the one that repeats, spelled as the files are
// spelled. A file in no cycle that imports one is refused by the build all the
// same, and a file with two imports that each lead into the cycle is told so at
// both.
func TestDiagnosticsReportsACycleAtTheFilesOwnImport(t *testing.T) {
	w, key := workspaceOf(t, map[string]string{
		"A.mut":    `import b "B.mut"; let a = 1;`,
		"B.mut":    `import a "A.mut"; let b = 1;`,
		"main.mut": `import a "A.mut"; let m = 1;`,
		"Via.mut":  `import a "A.mut"; let v = 1;`,
		"both.mut": `import a "A.mut"; import via "Via.mut"; let x = 1;`,
		"self.mut": `import me "self.mut"; let s = 1;`,
	})
	type cycleAt struct {
		imported int      // which of the entry's imports it is reported at
		chain    []string // the files the message names, in order
	}
	for _, c := range []struct {
		entry  string
		cycles []cycleAt
	}{
		{"A.mut", []cycleAt{{0, []string{"A.mut", "B.mut", "A.mut"}}}},
		{"B.mut", []cycleAt{{0, []string{"B.mut", "A.mut", "B.mut"}}}},
		{"main.mut", []cycleAt{{0, []string{"main.mut", "A.mut", "B.mut", "A.mut"}}}},
		{"both.mut", []cycleAt{
			{0, []string{"both.mut", "A.mut", "B.mut", "A.mut"}},
			{1, []string{"both.mut", "Via.mut", "A.mut", "B.mut", "A.mut"}},
		}},
		{"self.mut", []cycleAt{{0, []string{"self.mut", "self.mut"}}}},
	} {
		facts, known := w.Facts(key(c.entry))
		if !known {
			t.Fatalf("%s was never filed", c.entry)
		}
		got := w.Diagnostics(key(c.entry))
		if len(got) != len(c.cycles) {
			t.Errorf("Diagnostics(%s) = %+v, want %d import cycles", c.entry, got, len(c.cycles))
			continue
		}
		dir := filepath.Dir(facts.Path)
		for i, want := range c.cycles {
			named := make([]string, len(want.chain))
			for j, rel := range want.chain {
				named[j] = filepath.Join(dir, rel)
			}
			at := facts.Imports[want.imported].Range
			if got[i].Code != DiagImportCycle || got[i].Module != key(c.entry) || got[i].Range != at {
				t.Errorf("%s: diagnostic %d is code %d in %s at %v, want an import cycle at its own import %d, %v",
					c.entry, i, got[i].Code, got[i].Module, got[i].Range, want.imported, at)
			}
			if message := "import cycle: " + strings.Join(named, " -> "); got[i].Message != message {
				t.Errorf("%s: diagnostic %d says\n  %q\nwant\n  %q", c.entry, i, got[i].Message, message)
			}
		}
	}
}

// A build of main.mut is refused by dep.mut's two imports binding one name, and
// main.mut's import of dep.mut says so, in the loader's sentence naming dep.mut.
// dep.mut's own refusal is at its second import, and nothing is reported
// through that import: the loader refuses a repeated name before it resolves
// it, so the cycle y/util.mut would close back into dep.mut is never reached,
// and a build entered at y/util.mut is refused by the duplicate too.
func TestDiagnosticsReportsADuplicateBelowAtTheImportThatLeadsThere(t *testing.T) {
	w, key := workspaceOf(t, map[string]string{
		"main.mut":   `import dep "dep.mut"; let m = 1;`,
		"dep.mut":    `import "x/util.mut"; import "y/util.mut"; let d = 1;`,
		"x/util.mut": `let one = 1;`,
		"y/util.mut": `import dep "../dep.mut"; let two = 2;`,
	})
	depFacts, known := w.Facts(key("dep.mut"))
	if !known {
		t.Fatal("dep.mut was never filed")
	}
	want := depFacts.Path + `: two imports bind the name "util": "x/util.mut" and "y/util.mut"; ` +
		"give one an alias, as in `import other \"y/util.mut\"`"

	for _, c := range []struct {
		file     string
		imported int // which of the file's imports the duplicate is reported at
	}{
		{"main.mut", 0},
		{"dep.mut", 1},
		{"y/util.mut", 0},
	} {
		facts, known := w.Facts(key(c.file))
		if !known {
			t.Fatalf("%s was never filed", c.file)
		}
		got := w.Diagnostics(key(c.file))
		if len(got) != 1 || got[0].Code != DiagDuplicateNamespace || got[0].Module != key(c.file) ||
			got[0].Range != facts.Imports[c.imported].Range || got[0].Message != want {
			t.Errorf("Diagnostics(%s) = %+v\nwant one DiagDuplicateNamespace at its import %d, saying %q",
				c.file, got, c.imported, want)
		}
	}
}

// Typing inside a function body cannot change what another module sees, so it
// must not bump the generation. This is the keystroke-cost property: a caller
// that caches against Generation does no work at all for the common edit.
func TestEditingAFunctionBodyChangesNoFact(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "main.mut")
	uri := "file:///main.mut"

	w := NewWorkspace(nil)
	w.PutFile(uri, path, parse(t, `let f = fn() { return 1; };`))
	before := w.Generation()

	if changed := w.PutFile(uri, path, parse(t, `let f = fn() { return 1 + 2 + 3; };`)); changed {
		t.Fatal("editing a function body reported a change to what other modules see")
	}
	if w.Generation() != before {
		t.Fatal("editing a function body bumped the generation")
	}

	// Renaming the function, however, is exactly what importers must be told
	// about.
	if changed := w.PutFile(uri, path, parse(t, `let g = fn() { return 1; };`)); !changed {
		t.Fatal("renaming a top-level declaration reported no change")
	}
	if w.Generation() == before {
		t.Fatal("renaming a top-level declaration did not bump the generation")
	}
}

// Moving a declaration down the file changes its range but nothing another
// module can see, so no generation bump -- and the new range must still be
// served, because nothing caches another module's positions.
func TestMovingADeclarationKeepsTheGenerationAndUpdatesTheRange(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "lib.mut")
	uri := "file:///lib.mut"

	w := NewWorkspace(nil)
	w.PutFile(uri, path, parse(t, "let mean = fn() { return 1; };"))
	facts, _ := w.Facts(CanonicalKey(path))
	first := facts.Exports["mean"].DeclRange

	before := w.Generation()
	if changed := w.PutFile(uri, path, parse(t, "\n\n\nlet mean = fn() { return 1; };")); changed {
		t.Fatal("moving a declaration reported a change to what other modules see")
	}
	if w.Generation() != before {
		t.Fatal("moving a declaration bumped the generation")
	}

	facts, _ = w.Facts(CanonicalKey(path))
	if moved := facts.Exports["mean"].DeclRange; moved.Start.Line == first.Start.Line {
		t.Fatal("the range did not move, so go-to-definition would land on the old line")
	}
}

// Struct and enum names are program-global in Mutant -- claimTypeName refuses
// two modules declaring one struct name program-wide -- so they are written
// bare and are not members of a namespace. Offering `ns.Colour` would offer a
// spelling the compiler does not accept.
func TestTypesAreNotOfferedAsModuleMembers(t *testing.T) {
	w, key := workspaceOf(t, map[string]string{
		"main.mut": `import lib "lib.mut"; let x = 1;`,
		"lib.mut":  `enum Colour { Red, Green }; struct Point { x, y }; let mean = fn() { return 1; };`,
	})

	for _, member := range w.MembersOf(key("main.mut"), "lib") {
		if member.Name == "Colour" || member.Name == "Point" {
			t.Fatalf("lib. offered the type %q, which is written bare", member.Name)
		}
	}
	if got := w.ResolveField(key("main.mut"), LocalScope{}, "lib", "Colour"); got.Kind != FieldRefused {
		t.Fatalf("lib.Colour = %v, want a refusal", got.Kind)
	}

	// The types are still recorded -- they are program-global facts, just not
	// members.
	facts, _ := w.Facts(key("lib.mut"))
	if declared, ok := facts.Types["Colour"]; !ok || declared.Kind != SymEnum {
		t.Fatal("the enum was not recorded as a type")
	} else if len(declared.Members) != 2 || declared.Members[0] != "Red" {
		t.Fatalf("enum variants = %v, want [Red Green] in declaration order", declared.Members)
	}
}

// A file-local binding beats the fold, and it is the caller that knows about
// bindings -- the workspace never walks a function body.
func TestALocalBindingStillBeatsTheBuiltinFold(t *testing.T) {
	w, key := workspaceOf(t, map[string]string{"main.mut": `let x = 1;`})

	local := LocalScope{Bound: func(name string) bool { return name == "fs" }}
	if got := w.ResolveField(key("main.mut"), local, "fs", "read"); got.Kind != FieldValueAccess {
		t.Fatalf("fs.read with a bound fs = %v, want FieldValueAccess", got.Kind)
	}
	if got := w.ResolveField(key("main.mut"), LocalScope{}, "fs", "read"); got.Kind != FieldBuiltinFold {
		t.Fatalf("fs.read with nothing bound = %v, want FieldBuiltinFold", got.Kind)
	}
}

// The rule this pins is negative, and it is the one the workspace index breaks
// today: a file is never chosen because its name matches. Here the import
// spelling names a file that is not there, while a file with exactly that base
// name sits in another directory. Resolving to it would be the current
// go-to-definition bug -- a jump into a module nothing imported -- and the
// correct answer is silence.
func TestAFileIsNeverResolvedByItsName(t *testing.T) {
	w, key := workspaceOf(t, map[string]string{
		"main.mut":        `import stats "lib/stats.mut"; let x = 1;`,
		"other/stats.mut": `let mean = fn() { return 1; };`,
	})

	if bound, ok := w.Namespaces(key("main.mut"))["stats"]; ok {
		t.Fatalf("an import that names no indexed file resolved to %q", bound)
	}
	if members := w.MembersOf(key("main.mut"), "stats"); members != nil {
		t.Fatalf("a module chosen by name offered %v", members)
	}
	if _, _, _, found := w.DefinitionOf(key("main.mut"), LocalScope{}, "stats", "mean"); found {
		t.Fatal("go-to-definition jumped into a module nothing imported")
	}

	diags := w.Diagnostics(key("main.mut"))
	if len(diags) != 1 || diags[0].Code != DiagUnresolvedImport {
		t.Fatalf("diagnostics = %v, want one unresolved import", diags)
	}
}

// An import binds its alias the moment it is written, whether or not the
// workspace has read the file yet. What is unknown is the module's contents,
// not the fact that it is a module -- so the answer hedges rather than becoming
// an ordinary field read on a value called `stats`, which would invite the
// undefined-identifier rule to squiggle `stats` and completion to offer a
// value's members.
//
// This is what Confidence is for, and the case the editor hits on every cold
// start while the workspace scan is still running.
func TestAnAliasWhoseTargetIsUnreadHedgesRatherThanBecomingAValue(t *testing.T) {
	w, key := workspaceOf(t, map[string]string{
		"main.mut": `import stats "lib/stats.mut"; let x = 1;`,
	})

	resolved := w.ResolveField(key("main.mut"), LocalScope{}, "stats", "mean")
	if resolved.Kind != FieldModuleMember {
		t.Fatalf("stats.mean against an unread module = %v, want FieldModuleMember", resolved.Kind)
	}
	if resolved.Confidence != Provisional {
		t.Fatal("an unread module produced a Certain answer")
	}
	if resolved.Refusal != nil {
		t.Fatalf("an unread module produced a refusal: %v", resolved.Refusal)
	}
	// Provisional must render as nothing, never as a jump.
	if _, _, _, found := w.DefinitionOf(key("main.mut"), LocalScope{}, "stats", "mean"); found {
		t.Fatal("go-to-definition acted on a provisional answer")
	}

	// The private rule is about the name, not the file, so it still applies:
	// the editor can squiggle stats._total before it has read stats.mut.
	private := w.ResolveField(key("main.mut"), LocalScope{}, "stats", "_total")
	if private.Kind != FieldRefused || private.Confidence != Certain {
		t.Fatalf("stats._total against an unread module = %v/%v, want a certain refusal",
			private.Kind, private.Confidence)
	}

	// And it names the module by the import the author wrote. There is no key
	// to name it by, which is what unread means, and the sentence used to fill
	// that gap with "this program" -- the one module a name beginning with _ is
	// certainly not private to from here. A refusal the editor is right to show
	// early is also a refusal it has to word correctly early.
	if private.Refusal == nil {
		t.Fatal("the refusal carries no message")
	}
	text := private.Refusal.Error()
	if strings.Contains(text, "this program") {
		t.Fatalf("the refusal calls the imported module this program: %q", text)
	}
	if !strings.Contains(text, "lib/stats.mut") {
		t.Fatalf("the refusal names no module the author would recognise: %q", text)
	}
}
