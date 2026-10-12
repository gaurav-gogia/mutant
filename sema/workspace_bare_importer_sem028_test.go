package sema

import (
	"path/filepath"
	"testing"
)

// M26-SEM-028: `mutant lint .` and `mutant lint main.mut`, run from the file's
// own directory, file each module under a path relative to the working
// directory, so a top-level module is "main.mut" -- a path whose directory is
// ".". module.Resolver joins that module's imports onto ".", and so must the
// workspace: before the fix every import of such a module came back unresolved,
// and every cross-module refusal lint would have reported went missing.
func TestSEM028AnImporterInTheWorkingDirectoryResolvesBesideItself(t *testing.T) {
	stats := filepath.Join("lib", "stats.mut")
	w := NewWorkspace(nil)
	w.PutFile("file:///main.mut", "main.mut", parse(t, `import stats "lib/stats.mut";`))
	w.PutFile("file:///lib/stats.mut", stats, parse(t, `let mean = 1;`))

	main := CanonicalKey("main.mut")
	w.mu.RLock()
	key, path, ok := w.resolveImportFrom("main.mut", "lib/stats.mut")
	byKey, okByKey := w.resolveImport(main, "lib/stats.mut")
	w.mu.RUnlock()
	if !ok || key != CanonicalKey(stats) || path != stats {
		t.Errorf("resolveImportFrom(%q, %q) = %q, %q, %v; want %q, %q, true -- the file beside the importer, as module.Resolver finds it",
			"main.mut", "lib/stats.mut", key, path, ok, CanonicalKey(stats), stats)
	}
	if !okByKey || byKey != CanonicalKey(stats) {
		t.Errorf("resolveImport(%q, %q) = %q, %v; want %q, true", main, "lib/stats.mut", byKey, okByKey, CanonicalKey(stats))
	}
	if got := w.Namespaces(main)["stats"]; got != CanonicalKey(stats) {
		t.Errorf("Namespaces(%q)[\"stats\"] = %q, want %q: the alias names a module the workspace has read", main, got, CanonicalKey(stats))
	}
}

// The search paths still come after the importer's own directory for such a
// module, as they do in module.Resolver: a copy on a search path must not win
// over the file beside the importer.
func TestSEM028TheFileBesideABareImporterBeatsASearchPathCopy(t *testing.T) {
	elsewhere := filepath.Join(t.TempDir(), "mods")
	w := NewWorkspace([]string{elsewhere})
	w.PutFile("file:///a.mut", "a.mut", parse(t, `import b "b.mut";`))
	w.PutFile("file:///b.mut", "b.mut", parse(t, `let one = 1;`))
	copyPath := filepath.Join(elsewhere, "b.mut")
	w.PutFile("file:///copy/b.mut", copyPath, parse(t, `let two = 2;`))

	w.mu.RLock()
	key, path, ok := w.resolveImportFrom("a.mut", "b.mut")
	w.mu.RUnlock()
	if !ok || key != CanonicalKey("b.mut") || path != "b.mut" {
		t.Errorf("resolveImportFrom(%q, %q) = %q, %q, %v; want %q, %q, true, not the search path's copy %q",
			"a.mut", "b.mut", key, path, ok, CanonicalKey("b.mut"), "b.mut", copyPath)
	}
}

// A document with no path -- an importer named "" or filed under the "." an
// empty path cleans to -- is beside nothing, so a relative import from it
// stays unresolved even when a file of that name sits in the working
// directory.
func TestSEM028AnImporterWithNoPathStaysUnresolved(t *testing.T) {
	w := NewWorkspace(nil)
	w.PutFile("untitled:1", "", parse(t, `import b "b.mut";`))
	w.PutFile("file:///b.mut", "b.mut", parse(t, `let one = 1;`))

	w.mu.RLock()
	defer w.mu.RUnlock()
	for _, from := range []string{"", "."} {
		if key, path, ok := w.resolveImportFrom(from, "b.mut"); ok {
			t.Errorf("resolveImportFrom(%q, %q) = %q, %q, true; want unresolved: an importer with no path has no directory", from, "b.mut", key, path)
		}
	}
	if key, ok := w.resolveImport(CanonicalKey(""), "b.mut"); ok {
		t.Errorf("resolveImport(%q, %q) = %q, true; want unresolved", CanonicalKey(""), "b.mut", key)
	}
}
