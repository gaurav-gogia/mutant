package sema

import (
	"path/filepath"
	"strings"
	"testing"
)

// M26-SEM-033: `mutant lint .` run inside a project files every module by its
// path relative to the working directory, so a top-level module is filed as
// a.mut, with no directory in its path. module.Resolver joins a relative import
// onto the importer's directory, which for a.mut is ".", and finds the file
// beside it; the workspace gave such an importer no own-directory candidate, so
// the import came back unresolved -- or as a search path's copy -- and the cycle
// the build refuses went unreported. No source file is written or opened: every
// module is filed from source held in the test. Every module path is relative,
// except the search path's copy in the second test, which is filed under an
// absolute path below an empty temporary directory; neither that file nor its
// lib directory is ever created.

// bareWorkspaceSEM033 files each source under its relative path exactly as
// written, which is how api.NewProject files what `mutant lint .` collected.
func bareWorkspaceSEM033(t *testing.T, searchPaths []string, files map[string]string) *Workspace {
	t.Helper()
	w := NewWorkspace(searchPaths)
	for rel, src := range files {
		path := filepath.FromSlash(rel)
		w.PutFile("file:///"+filepath.ToSlash(path), path, parse(t, src))
	}
	return w
}

// A cycle through a module filed with no directory in its path is reported in
// the loader's words, at the module's own import -- both where the module is in
// the cycle and where the cycle is below it, reached through a subdirectory
// whose import climbs back up to the top level.
func TestBareImporterSEM033ReportsTheCycleTheBuildRefuses(t *testing.T) {
	w := bareWorkspaceSEM033(t, nil, map[string]string{
		"a.mut":     `import b "b.mut"; let fa = 1;`,
		"b.mut":     `import a "a.mut"; let fb = 1;`,
		"m.mut":     `import s "sub/s.mut"; let m = 1;`,
		"sub/s.mut": `import a "../a.mut"; let s = 1;`,
	})
	sub := filepath.Join("sub", "s.mut")
	for _, c := range []struct {
		entry string
		chain []string
	}{
		{"a.mut", []string{"a.mut", "b.mut", "a.mut"}},
		{"b.mut", []string{"b.mut", "a.mut", "b.mut"}},
		{"m.mut", []string{"m.mut", sub, "a.mut", "b.mut", "a.mut"}},
	} {
		key := CanonicalKey(c.entry)
		facts, known := w.Facts(key)
		if !known {
			t.Fatalf("%s was never filed", c.entry)
		}
		want := "import cycle: " + strings.Join(c.chain, " -> ")
		got := w.Diagnostics(key)
		if len(got) != 1 || got[0].Code != DiagImportCycle || got[0].Module != key ||
			got[0].Range != facts.Imports[0].Range || got[0].Message != want {
			t.Errorf("Diagnostics(%s) = %+v\nwant one import cycle at its own import, saying %q", c.entry, got, want)
		}
	}
}

// The file beside the importer comes before every search path, as it does for
// module.Resolver: a search path's copy of b.mut is not what a.mut's import
// means when a b.mut is filed beside it.
func TestBareImporterSEM033PrefersTheFileBesideItOverASearchPath(t *testing.T) {
	lib := filepath.Join(t.TempDir(), "lib")
	w := bareWorkspaceSEM033(t, []string{lib}, map[string]string{
		"a.mut": `import b "b.mut"; let fa = 1;`,
		"b.mut": `let fb = 1;`,
	})
	copyPath := filepath.Join(lib, "b.mut")
	w.PutFile("file:///"+filepath.ToSlash(copyPath), copyPath, parse(t, `let fb = 2;`))

	w.mu.RLock()
	key, path, ok := w.resolveImportFrom("a.mut", "b.mut")
	w.mu.RUnlock()
	if !ok || key != CanonicalKey("b.mut") || path != "b.mut" {
		t.Fatalf("resolveImportFrom(a.mut, b.mut) = (%q, %q, %v), want the b.mut beside it, (%q, %q, true)",
			key, path, ok, CanonicalKey("b.mut"), "b.mut")
	}
	if bound := w.Namespaces(CanonicalKey("a.mut"))["b"]; bound != CanonicalKey("b.mut") {
		t.Fatalf("a.mut's alias b is bound to %q, want %q", bound, CanonicalKey("b.mut"))
	}
}

// A document with no file path has no directory, not the working directory: an
// import in it stays unresolved even when a file of that name is filed at the
// top level, and only a search path can answer it.
func TestBareImporterSEM033LeavesADocumentWithNoPathUnresolved(t *testing.T) {
	w := bareWorkspaceSEM033(t, nil, map[string]string{
		"b.mut": `let fb = 1;`,
	})
	w.PutFile("untitled:Untitled-1", "", parse(t, `import b "b.mut"; let x = 1;`))

	w.mu.RLock()
	key, path, ok := w.resolveImportFrom("", "b.mut")
	w.mu.RUnlock()
	if ok {
		t.Fatalf("resolveImportFrom(\"\", b.mut) = (%q, %q, true), want no answer for a document with no path", key, path)
	}
	diags := w.Diagnostics(CanonicalKey(""))
	if len(diags) != 1 || diags[0].Code != DiagUnresolvedImport {
		t.Fatalf("Diagnostics of the pathless document = %+v, want one unresolved import", diags)
	}
}
