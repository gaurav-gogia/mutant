package server

import (
	"errors"
	"path/filepath"
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
// every file by its absolute path, as the editor does.

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

// publishedOnOpen scans root, opens rel the way an editor does, and returns
// what the server published for it.
func publishedOnOpen(t *testing.T, root, rel string) []lsp.Diagnostic {
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
	uri := pathToURI(filepath.Join(root, filepath.FromSlash(rel)))
	if err := s.didOpen(ctx, &lsp.DidOpenTextDocumentParams{TextDocument: lsp.TextDocumentItem{
		URI: uri, LanguageID: "mutant", Version: 1, Text: importRefusalFixture[rel],
	}}); err != nil {
		t.Fatal(err)
	}
	if len(published) != 1 {
		t.Fatalf("opening %s published %d times, want once", rel, len(published))
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

			published := publishedOnOpen(t, root, c.file)
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
