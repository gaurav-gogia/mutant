package server

import (
	"errors"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/tliron/glsp"
	lsp "github.com/tliron/glsp/protocol_3_16"

	"mutant/module"
)

// M26-SEM-006: the build refuses an import cycle and two imports binding one
// name, and the editor said nothing about either.
//
// sema computed both and nothing published them: publishDiagnostics sent the
// analyzer's diagnostics, and the analyzer had no import-level refusal. A file
// in a cycle showed nothing at all. A file with a duplicated alias showed only
// "unused import", on the first of the two imports -- the one the loader binds
// and the use reaches. A file in no cycle that imports one, which the build
// refuses just the same, showed nothing either.
//
// Each file below is opened in a server that has scanned the tree, and what the
// server publishes for it is held against what module.Load -- the build's
// first step -- says of a build entered at that file. The tree is under
// t.TempDir(), outside the directory the test runs in, so the loader names
// every file by its absolute path, as the editor does. Every import in it spells
// its file in the file's own letter case: one that spells it in another case,
// which the workspace resolves only on Windows, where sema.CanonicalKey folds
// letter case, is the business of
// TestImportRefusalsNameAFileAsItsImportSpellsIt (M26-SEM-032).

var importRefusalFixture = map[string]string{
	// a.mut and b.mut import each other. main.mut is in no cycle, but a build
	// of it walks into one.
	"a.mut":    "import b \"b.mut\";\nlet fa = fn() { return b.fb(); };\nputln(fa());\n",
	"b.mut":    "import a \"a.mut\";\nlet fb = fn() { return 1; };\n",
	"main.mut": "import a \"a.mut\";\nputln(a.fa());\n",
	// Two unaliased imports both bind util, and two aliased ones both bind m.
	// spare is bound once and read nowhere.
	"dup.mut":     "import \"x/util.mut\";\nimport \"y/util.mut\";\nputln(util.one());\n",
	"aliased.mut": "import m \"x/util.mut\";\nimport m \"y/util.mut\";\nimport spare \"x/util.mut\";\nputln(m.one());\n",
	// The build accepts these: a diamond, one module under two names, and an
	// import of a file the scan never indexes, because it skips dot-directories.
	"diamond.mut":      "import p \"p.mut\";\nimport q \"q.mut\";\nputln(p.fp() + q.fq());\n",
	"twice.mut":        "import one \"x/util.mut\";\nimport two \"x/util.mut\";\nputln(one.one() + two.one());\n",
	"hidden.mut":       "import h \".hidden/util.mut\";\nputln(1);\n",
	"p.mut":            "import util \"x/util.mut\";\nlet fp = fn() { return util.one(); };\n",
	"q.mut":            "import util \"x/util.mut\";\nlet fq = fn() { return util.one(); };\n",
	"x/util.mut":       "let one = fn() { return 1; };\n",
	"y/util.mut":       "let two = fn() { return 2; };\n",
	".hidden/util.mut": "let one = fn() { return 1; };\n",
}

// publishedOnOpen scans root, opens rel with text in it the way an editor does,
// and returns what the server published for it.
func publishedOnOpen(t *testing.T, root, rel, text string) []lsp.Diagnostic {
	t.Helper()
	return publishedOnOpenURI(t, root, pathToURI(filepath.Join(root, filepath.FromSlash(rel))), text)
}

// publishedOnOpenURI is publishedOnOpen for a document the editor names by uri.
func publishedOnOpenURI(t *testing.T, root string, uri lsp.DocumentUri, text string) []lsp.Diagnostic {
	t.Helper()
	s := New(false)
	s.setRoots([]string{root})
	s.scanWorkspace()

	var published []lsp.PublishDiagnosticsParams
	ctx := &glsp.Context{Notify: func(_ string, params any) {
		if p, ok := params.(lsp.PublishDiagnosticsParams); ok {
			published = append(published, p)
		}
	}}
	if err := s.didOpen(ctx, &lsp.DidOpenTextDocumentParams{TextDocument: lsp.TextDocumentItem{
		URI: uri, LanguageID: "mutant", Version: 1, Text: text,
	}}); err != nil {
		t.Fatal(err)
	}
	if len(published) != 1 {
		t.Fatalf("opening %s published %d times, want once", uri, len(published))
	}
	return published[0].Diagnostics
}

// loaderVerdict is what module.Load says of a build entered at path: which
// refusal it raises and the error itself, or "" and nil when it accepts.
func loaderVerdict(t *testing.T, path string) (string, error) {
	t.Helper()
	_, err := module.Load(path, nil)
	var (
		cycle     *module.CycleError
		duplicate *module.DuplicateNamespaceError
	)
	switch {
	case err == nil:
		return "", nil
	case errors.As(err, &cycle):
		return "cycle", err
	case errors.As(err, &duplicate):
		return "duplicate", err
	}
	t.Fatalf("the loader refuses %s for a reason this test is not about: %v", path, err)
	return "", err
}

func TestImportRefusalsArePublished(t *testing.T) {
	root := writeModules(t, importRefusalFixture)
	for _, c := range []struct {
		file string
		// refusal is what the loader refuses the file for, or "" when it
		// accepts it.
		refusal string
		// line is the import statement the refusal is published at.
		line int
		// shared is a name two imports bind. It is not unused: the build
		// refuses the file, and the refusal says why.
		shared string
		// unused is a name bound once and never read, which is still said to
		// be unused in a file that also binds another name twice.
		unused string
	}{
		{file: "a.mut", refusal: "cycle", line: 0},
		{file: "b.mut", refusal: "cycle", line: 0},
		{file: "main.mut", refusal: "cycle", line: 0},
		{file: "dup.mut", refusal: "duplicate", line: 1, shared: "util"},
		{file: "aliased.mut", refusal: "duplicate", line: 1, shared: "m", unused: "spare"},
		{file: "diamond.mut"},
		{file: "twice.mut"},
		// Not indexed is not missing: the file is there and the build accepts
		// it, so "no such file" would be a refusal the build never makes.
		{file: "hidden.mut"},
	} {
		t.Run(c.file, func(t *testing.T) {
			refusal, loaded := loaderVerdict(t, filepath.Join(root, filepath.FromSlash(c.file)))
			if refusal != c.refusal {
				t.Fatalf("the loader's verdict on %s is %q (%v), but the case is written for %q", c.file, refusal, loaded, c.refusal)
			}

			published := publishedOnOpen(t, root, c.file, importRefusalFixture[c.file])
			for _, d := range published {
				if c.shared != "" && strings.HasPrefix(d.Message, "unused import `"+c.shared+"`") {
					t.Errorf("%s: %q -- two imports bind %s, and the build refuses the file rather than leaving one unread",
						c.file, d.Message, c.shared)
				}
			}
			if c.unused != "" {
				said := false
				for _, d := range published {
					said = said || strings.HasPrefix(d.Message, "unused import `"+c.unused+"`")
				}
				if !said {
					t.Errorf("%s: nothing says %s is unused, among %q", c.file, c.unused, diagnosticMessages(published))
				}
			}

			var refusals []lsp.Diagnostic
			for _, d := range published {
				if d.Source != nil && *d.Source == "mutant-modules" {
					refusals = append(refusals, d)
				}
			}
			if loaded == nil {
				if len(refusals) != 0 {
					t.Errorf("the build accepts %s, and the server published %q", c.file, diagnosticMessages(refusals))
				}
				return
			}
			if len(refusals) != 1 {
				t.Errorf("%s: published %q from mutant-modules, want the loader's %q alone",
					c.file, diagnosticMessages(refusals), loaded.Error())
				return
			}
			got := refusals[0]
			if got.Message != loaded.Error() {
				t.Errorf("%s: published\n  %q\nand the loader says\n  %q", c.file, got.Message, loaded.Error())
			}
			if got.Severity == nil || *got.Severity != lsp.DiagnosticSeverityError {
				t.Errorf("%s: published with severity %v, want Error: the build refuses the file", c.file, got.Severity)
			}
			statement := strings.Split(importRefusalFixture[c.file], "\n")[c.line]
			want := lsp.Range{
				Start: lsp.Position{Line: lsp.UInteger(c.line)},
				End:   lsp.Position{Line: lsp.UInteger(c.line), Character: lsp.UInteger(len(statement))},
			}
			if got.Range != want {
				t.Errorf("%s: published at %+v, want %+v, the import %q", c.file, got.Range, want, statement)
			}
		})
	}
}

// M26-SEM-032: on Windows, where a module's identity folds letter case, an
// import may spell a file in another case than the file on disk, and the build
// names the file as the import spelled it. module.Load names every file by the
// path its walk reached it through: the entry as it was given, made absolute
// and cleaned, and each file below it at the path module.Resolver.Resolve
// returns for the import that reached it -- the import's absolute spelling, or
// its spelling joined onto the directory of the file that wrote it or onto the
// --module-path directory it was found in, cleaned either way. The editor named
// every file below the entry as the workspace had filed it -- here, as the scan
// found it on disk -- so the two sentences differed in exactly the letters an
// import respelled, and the editor's was not the build's.
//
// Each case also lists the files the loader's sentence names -- a cycle's
// chain, or the file that binds a name twice -- so that it cannot pass by the
// loader ceasing to name a file as its import spelled it. A build of b.mut,
// c.mut or e.mut is refused by a cycle whose closing import respells the entry,
// so the loader names that file two ways, and the editor has to as well. p.mut,
// q.mut, d.mut and sub/s.mut reach nothing through a respelled import: they are
// the controls, which held before.
var respelledImportFixture = map[string]string{
	// a.mut spells b.mut as B.mut and closes a cycle; main.mut walks into it.
	"a.mut":    "import b \"B.mut\";\nlet fa = fn() { return b.fb(); };\nputln(fa());\n",
	"b.mut":    "import a \"a.mut\";\nlet fb = fn() { return 1; };\n",
	"main.mut": "import a \"a.mut\";\nputln(a.fa());\n",
	// Both imports of this cycle respell their file.
	"c.mut": "import e \"E.mut\";\nlet fc = 1;\n",
	"e.mut": "import c \"C.mut\";\nlet fe = 1;\n",
	// d.mut binds util twice and is reached as D.mut; sub/s.mut binds it twice
	// and is reached through the directory spelled SUB.
	"usesdup.mut": "import dep \"D.mut\";\nputln(1);\n",
	"d.mut":       "import \"x/util.mut\";\nimport \"y/util.mut\";\nputln(util.one());\n",
	"usessub.mut": "import s \"SUB/s.mut\";\nputln(1);\n",
	"sub/s.mut":   "import \"../x/util.mut\";\nimport \"../y/util.mut\";\nputln(util.one());\n",
	"x/util.mut":  "let one = fn() { return 1; };\n",
	"y/util.mut":  "let two = fn() { return 2; };\n",
	// A cycle whose imports spell their files as they are on disk.
	"p.mut": "import q \"q.mut\";\nlet fp = 1;\n",
	"q.mut": "import p \"p.mut\";\nlet fq = 1;\n",
}

func TestImportRefusalsNameAFileAsItsImportSpellsIt(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("sema.CanonicalKey folds letter case only on Windows, so only there does an import of B.mut name the module b.mut")
	}
	root := writeModules(t, respelledImportFixture)
	for _, c := range []struct {
		file string
		// refusal is what the loader refuses the file for.
		refusal string
		// named are the files the loader's sentence names, relative to the
		// tree and spelled as its walk reached them: a cycle's chain, or the
		// file that binds a name twice.
		named []string
	}{
		{"a.mut", "cycle", []string{"a.mut", "B.mut", "a.mut"}},
		{"b.mut", "cycle", []string{"b.mut", "a.mut", "B.mut"}},
		{"main.mut", "cycle", []string{"main.mut", "a.mut", "B.mut", "a.mut"}},
		{"c.mut", "cycle", []string{"c.mut", "E.mut", "C.mut"}},
		{"e.mut", "cycle", []string{"e.mut", "C.mut", "E.mut"}},
		{"usesdup.mut", "duplicate", []string{"D.mut"}},
		{"usessub.mut", "duplicate", []string{"SUB/s.mut"}},
		{"p.mut", "cycle", []string{"p.mut", "q.mut", "p.mut"}},
		{"q.mut", "cycle", []string{"q.mut", "p.mut", "q.mut"}},
		{"d.mut", "duplicate", []string{"d.mut"}},
		{"sub/s.mut", "duplicate", []string{"sub/s.mut"}},
	} {
		t.Run(c.file, func(t *testing.T) {
			refusal, loaded := loaderVerdict(t, filepath.Join(root, filepath.FromSlash(c.file)))
			if refusal != c.refusal {
				t.Fatalf("the loader's verdict on %s is %q (%v), but the case is written for %q", c.file, refusal, loaded, c.refusal)
			}
			var (
				cycle     *module.CycleError
				duplicate *module.DuplicateNamespaceError
				named     []string
			)
			switch {
			case errors.As(loaded, &cycle):
				named = cycle.Chain
			case errors.As(loaded, &duplicate):
				named = []string{duplicate.Importer}
			}
			want := make([]string, len(c.named))
			for i, rel := range c.named {
				want[i] = filepath.Join(root, filepath.FromSlash(rel))
			}
			if !slices.Equal(named, want) {
				t.Fatalf("refusing %s, the loader names %q, but the case is written for %q", c.file, named, want)
			}

			var refusals []string
			for _, d := range publishedOnOpen(t, root, c.file, respelledImportFixture[c.file]) {
				if d.Source != nil && *d.Source == "mutant-modules" {
					refusals = append(refusals, d.Message)
				}
			}
			if len(refusals) != 1 || refusals[0] != loaded.Error() {
				t.Errorf("%s: published %q from mutant-modules, want the loader's\n  %q\nalone", c.file, refusals, loaded.Error())
			}
		})
	}
}

// A document opened under an uncleaned path -- file:///.../a.mut/., or the same
// with a trailing slash or a last segment of ".." -- is filed under a.mut's key,
// but the path itself is not a.mut's: its filepath.Dir is a.mut itself or a
// directory below it. module.Load cleans the path it is given before it names
// the entry or resolves an import from its directory, and the editor has to as
// well. Resolving from the uncleaned path's directory finds no import, and the
// cycle a build of a.mut is refused for goes unpublished; naming the entry by
// that path gives a sentence the loader never prints, which is what the editor
// published for a.mut and d.mut before M26-SEM-032.
func TestImportRefusalsOfADocumentOpenedUnderAnUncleanPath(t *testing.T) {
	files := map[string]string{
		"a.mut":      "import b \"b.mut\";\nlet fa = 1;\n",
		"b.mut":      "import a \"a.mut\";\nlet fb = 1;\n",
		"d.mut":      "import \"x/util.mut\";\nimport \"y/util.mut\";\nlet d = 1;\n",
		"x/util.mut": "let one = 1;\n",
		"y/util.mut": "let two = 2;\n",
	}
	root := writeModules(t, files)
	for _, c := range []struct {
		file string
		// refusal is what the loader refuses the file for.
		refusal string
	}{
		{"a.mut", "cycle"},
		{"d.mut", "duplicate"},
	} {
		for _, unclean := range []struct{ name, suffix string }{
			{"dot", "/."},
			{"slash", "/"},
			{"parent", "/x/.."},
		} {
			t.Run(c.file+"-"+unclean.name, func(t *testing.T) {
				uri := pathToURI(filepath.Join(root, c.file)) + lsp.DocumentUri(unclean.suffix)
				path, ok := uriToPath(uri)
				if !ok {
					t.Fatalf("the server reads no path from %s", uri)
				}
				refusal, loaded := loaderVerdict(t, path)
				if refusal != c.refusal {
					t.Fatalf("the loader's verdict on %s is %q (%v), but the case is written for %q", path, refusal, loaded, c.refusal)
				}

				var refusals []string
				for _, d := range publishedOnOpenURI(t, root, uri, files[c.file]) {
					if d.Source != nil && *d.Source == "mutant-modules" {
						refusals = append(refusals, d.Message)
					}
				}
				if len(refusals) != 1 || refusals[0] != loaded.Error() {
					t.Errorf("%s: published %q from mutant-modules, want the loader's\n  %q\nalone", uri, refusals, loaded.Error())
				}
			})
		}
	}
}

// The compiler's refusals hang on the same answer. The editor runs the compiler
// on a document only when Diagnostics says nothing about any module of its
// closure, and c.mut's import, resolved from an uncleaned path's directory,
// comes back unresolved: a document opened under such a path with a refusal
// the compiler makes -- break outside a loop, which `mutant gen` refuses in
// these words -- would show nothing, where the same document opened under its
// own path shows the refusal.
func TestACompilerRefusalOfADocumentOpenedUnderAnUncleanPath(t *testing.T) {
	files := map[string]string{
		"c.mut":      "import u \"x/util.mut\";\nputln(u.one);\nbreak;\n",
		"x/util.mut": "let one = 1;\n",
	}
	root := writeModules(t, files)
	compilerSays := func(uri lsp.DocumentUri) []string {
		var said []string
		for _, d := range publishedOnOpenURI(t, root, uri, files["c.mut"]) {
			if d.Source != nil && *d.Source == "mutant-compiler" {
				said = append(said, d.Message)
			}
		}
		return said
	}
	plain := pathToURI(filepath.Join(root, "c.mut"))
	want := compilerSays(plain)
	if len(want) != 1 {
		t.Fatalf("opened as %s, the compiler's refusals are %q, want one: break outside a loop", plain, want)
	}
	for _, suffix := range []string{"/.", "/", "/x/.."} {
		uri := plain + lsp.DocumentUri(suffix)
		if got := compilerSays(uri); !slices.Equal(got, want) {
			t.Errorf("opened as %s, the compiler's refusals are %q, want %q, as under its own path", uri, got, want)
		}
	}
}
