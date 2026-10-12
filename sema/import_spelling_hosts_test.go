package sema

import "testing"

// M26-RUN-012, the editor's half. Workspace.resolveImportFrom walks the
// candidates module.Resolver walks, so it must read a spelling the way the
// loader does: a backslash as a separator on every host, and a spelling the
// loader refuses as naming a different file on each host as naming nothing.

func TestImportSpellingHostsBackslashBindsTheModule(t *testing.T) {
	w, key := workspaceOf(t, map[string]string{
		"main.mut":     `import u "lib\\util.mut";`,
		"lib/util.mut": `let x = 1;`,
	})

	if got := w.Namespaces(key("main.mut"))["u"]; got != key("lib/util.mut") {
		t.Fatalf(`lib\util.mut bound u to %q, want %q on every host`, got, key("lib/util.mut"))
	}
}

// The target files are indexed under the very names the imports spell, so a
// resolver that read the spelling literally would find them on any host. The
// loader refuses each spelling, so the editor must not bind it either.
func TestImportSpellingHostsTrimmedSpellingsBindNothing(t *testing.T) {
	for _, tc := range []struct{ spelling, file string }{
		{"util.mut ", "util.mut "},
		{"lib./util.mut", "lib./util.mut"},
		{"lib /util.mut", "lib /util.mut"},
	} {
		w, key := workspaceOf(t, map[string]string{
			"main.mut": `import u "` + tc.spelling + `";`,
			tc.file:    `let x = 1;`,
		})
		if got, bound := w.Namespaces(key("main.mut"))["u"]; bound {
			t.Errorf("import %q bound u to %q; the loader refuses this spelling", tc.spelling, got)
		}
	}
}

// A spelling that begins with a separator is never joined onto the importer's
// directory: the loader refuses it on Windows, where it names no drive, and
// looks it up as an absolute path elsewhere. So the file beside the importer
// that a join would reach must not be bound, on any host.
func TestImportSpellingHostsRootedSpellingIsNotJoined(t *testing.T) {
	for _, source := range []string{
		`import u "/lib/util.mut";`,
		`import u "\\lib\\util.mut";`,
	} {
		w, key := workspaceOf(t, map[string]string{
			"main.mut":     source,
			"lib/util.mut": `let x = 1;`,
		})
		if got := w.Namespaces(key("main.mut"))["u"]; got == key("lib/util.mut") {
			t.Errorf("%s bound u to the file beside the importer, %q", source, got)
		}
	}
}
