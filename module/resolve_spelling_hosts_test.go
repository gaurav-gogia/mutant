package module

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// M26-RUN-012: one import line must mean one thing on every host. These tests
// assert the same outcome whatever GOOS runs them, which is the point: before
// the fix, `lib\util.mut` and `util.mut ` resolved on Windows (Win32 treats the
// backslash as a separator and drops a trailing space) and were "cannot find
// module" on Linux. The one test whose outcome depends on GOOS,
// TestResolveSpellingHostsRootedSpellingIsNeverJoined, says why.

func TestResolveSpellingHostsBackslashIsASeparator(t *testing.T) {
	root := writeTree(t, map[string]string{"lib/util.mut": "let u = 1;\n"})
	r := NewResolver(nil)

	want, err := r.Resolve("lib/util.mut", root)
	if err != nil {
		t.Fatalf("forward-slash spelling: %v", err)
	}
	got, err := r.Resolve(`lib\util.mut`, root)
	if err != nil {
		t.Fatalf(`lib\util.mut must resolve on every host, as Namespace already reads it: %v`, err)
	}
	if got != want {
		t.Fatalf(`lib\util.mut resolved to %q, lib/util.mut to %q: one file, two answers`, got, want)
	}
}

func TestResolveSpellingHostsLoadsOneModuleForBothSeparators(t *testing.T) {
	root := writeTree(t, map[string]string{
		"main.mut":     `import a "lib\\util.mut";` + "\n" + `import b "lib/util.mut";` + "\n",
		"lib/util.mut": "let u = 1;\n",
	})

	g, err := Load(filepath.Join(root, "main.mut"), nil)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(g.Modules) != 2 {
		t.Fatalf("expected 2 modules, got %v", displayNames(g))
	}
	if g.Entry.Imports[0].Path != g.Entry.Imports[1].Path {
		t.Fatalf("the two separators reached two modules: %q and %q",
			g.Entry.Imports[0].Path, g.Entry.Imports[1].Path)
	}
}

// Every spelling below names an existing file once a segment's trailing spaces
// or dots are dropped. Windows drops some of them (measured on Windows 11
// before the fix: "util.mut " and "lib./util.mut" resolved there) and Linux
// drops none, so each must be refused, on every host, as an invalid path --
// not resolved on one host and "cannot find module" on the other -- and the
// reason must name the Windows cause.
func TestResolveSpellingHostsRefusesWhatWindowsWouldTrim(t *testing.T) {
	root := writeTree(t, map[string]string{
		"util.mut":     "let u = 1;\n",
		"lib/util.mut": "let l = 1;\n",
	})
	r := NewResolver(nil)

	for _, spelling := range []string{
		"util.mut ",
		"util.mut.",
		"lib /util.mut",
		`lib \util.mut`,
		"lib./util.mut",
		"lib../util.mut",
	} {
		got, err := r.Resolve(spelling, root)
		var invalid *InvalidPathError
		if !errors.As(err, &invalid) {
			t.Errorf("Resolve(%q) = %q, %v; want *InvalidPathError on every host", spelling, got, err)
			continue
		}
		if !strings.Contains(invalid.Reason, "Windows") {
			t.Errorf("Resolve(%q): the reason does not say why: %q", spelling, invalid.Reason)
		}
	}
}

// Whitespace at either end of a path, other than a trailing U+0020, is refused
// too, but no host drops it, so the reason must not blame Windows. Before the
// repair each of these was refused as whitespace "which Windows drops from the
// end of a name", which is false for leading whitespace and for a trailing
// tab, newline or no-break space.
func TestResolveSpellingHostsEdgeWhitespaceReasonBlamesNoHost(t *testing.T) {
	root := writeTree(t, map[string]string{"util.mut": "let u = 1;\n"})
	r := NewResolver(nil)

	for _, spelling := range []string{
		" util.mut",
		"\tutil.mut",
		"util.mut\t",
		"util.mut\n",
		"util.mut\xc2\xa0", // U+00A0 NO-BREAK SPACE
		" util.mut ",
	} {
		got, err := r.Resolve(spelling, root)
		var invalid *InvalidPathError
		if !errors.As(err, &invalid) {
			t.Errorf("Resolve(%q) = %q, %v; want *InvalidPathError on every host", spelling, got, err)
			continue
		}
		if strings.Contains(invalid.Reason, "Windows") {
			t.Errorf("Resolve(%q): no host drops this whitespace, yet the reason blames Windows: %q",
				spelling, invalid.Reason)
		}
		if !strings.Contains(invalid.Reason, "whitespace") {
			t.Errorf("Resolve(%q): the reason does not name the whitespace: %q", spelling, invalid.Reason)
		}
	}
}

// A spelling that begins with a separator is absolute outside Windows and is
// looked up as written. On Windows it names no drive, so it is rooted on the
// current drive, not absolute; joining it onto the importer's directory made
// `\lib\util.mut` open lib/util.mut there while Linux looked at /lib/util.mut.
// It must never be joined onto the importer's directory: Windows refuses it,
// and every other host searches the one absolute path.
func TestResolveSpellingHostsRootedSpellingIsNeverJoined(t *testing.T) {
	root := writeTree(t, map[string]string{"lib/util.mut": "let l = 1;\n"})
	r := NewResolver(nil)

	for _, spelling := range []string{`\lib\util.mut`, "/lib/util.mut"} {
		got, err := r.Resolve(spelling, root)
		if runtime.GOOS == "windows" {
			var invalid *InvalidPathError
			if !errors.As(err, &invalid) {
				t.Errorf("Resolve(%q) = %q, %v; want *InvalidPathError on Windows", spelling, got, err)
				continue
			}
			if !strings.Contains(invalid.Reason, "separator") {
				t.Errorf("Resolve(%q): the reason does not say why: %q", spelling, invalid.Reason)
			}
			continue
		}
		absolute := filepath.Clean("/lib/util.mut")
		if _, statErr := os.Stat(absolute); statErr == nil {
			t.Skipf("%s exists on this host, so a lookup there cannot be told from a join", absolute)
		}
		var missing *NotFoundError
		if !errors.As(err, &missing) {
			t.Errorf("Resolve(%q) = %q, %v; want *NotFoundError for the absolute path", spelling, got, err)
			continue
		}
		if len(missing.Searched) != 1 || missing.Searched[0] != absolute {
			t.Errorf("Resolve(%q) searched %q; want only %q", spelling, missing.Searched, absolute)
		}
	}
}

// The refusal must not catch spellings that already mean one file everywhere:
// a dot-segment, a leading dot, a space inside a name.
func TestResolveSpellingHostsKeepsPortableSpellings(t *testing.T) {
	root := writeTree(t, map[string]string{
		"lib/util.mut":     "let l = 1;\n",
		".hidden/util.mut": "let h = 1;\n",
		"my lib/util.mut":  "let m = 1;\n",
	})
	r := NewResolver(nil)

	for _, spelling := range []string{
		"./lib/util.mut",
		"lib/../lib/util.mut",
		`.\lib\util.mut`,
		".hidden/util.mut",
		"my lib/util.mut",
	} {
		if _, err := r.Resolve(spelling, root); err != nil {
			t.Errorf("Resolve(%q): %v", spelling, err)
		}
	}
}

func TestLoadSpellingHostsRefusesATrailingSpace(t *testing.T) {
	root := writeTree(t, map[string]string{
		"main.mut": `import u "util.mut ";` + "\n",
		"util.mut": "let u = 1;\n",
	})

	_, err := Load(filepath.Join(root, "main.mut"), nil)
	var invalid *InvalidPathError
	if !errors.As(err, &invalid) {
		t.Fatalf("expected *InvalidPathError on every host, got %T: %v", err, err)
	}
	if !strings.Contains(err.Error(), "main.mut") {
		t.Fatalf("the refusal does not name the importing file: %s", err)
	}
}
