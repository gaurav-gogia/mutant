package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A directory the user names is walked whatever it is called. WalkDir hands the
// root to the callback first and the root's d.Name() is its last element, so a
// dot-name on the root used to match the skip rule meant for build state below
// it: `mutant test .hidden` reported "no *_test.mut files found" and exited 0
// while the same test file named directly failed, and `mutant lint .hidden`
// printed nothing and exited 0 while the file named directly reported two
// diagnostics. (M26-TOOL-019)
func TestCollectFilesWalksAnExplicitDotDirectory(t *testing.T) {
	root := t.TempDir()
	hidden := filepath.Join(root, ".hidden")
	if err := os.MkdirAll(hidden, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		"bad_test.mut": "let a = 1;\n",
		"lintme.mut":   "let b = 2;\n",
	} {
		if err := os.WriteFile(filepath.Join(hidden, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("source files", func(t *testing.T) {
		files, problems := collectMutantSourceFiles([]string{hidden})
		if len(problems) != 0 {
			t.Fatalf("problems on a readable directory: %v", problems)
		}
		if len(files) != 2 {
			t.Fatalf("expected both .mut files in an explicitly named dot-directory, got %d: %v", len(files), files)
		}
	})

	t.Run("test files", func(t *testing.T) {
		files, problems := collectMutantTestFiles([]string{hidden})
		if len(problems) != 0 {
			t.Fatalf("problems on a readable directory: %v", problems)
		}
		if len(files) != 1 {
			t.Fatalf("expected the one *_test.mut file in an explicitly named dot-directory, got %d: %v", len(files), files)
		}
	})
}

// The skip rule still applies below the root, which is the half the fix must
// not have cost: a dot-directory nested inside the tree being walked is build
// or tool state and is not source anybody asked for.
func TestCollectFilesStillSkipsADotDirectoryBelowTheRoot(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{".git", "node_modules", "vendor"} {
		path := filepath.Join(root, dir)
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(path, "buried.mut"), []byte("let a = 1;\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "real.mut"), []byte("let a = 1;\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	files, problems := collectMutantSourceFiles([]string{root})
	if len(problems) != 0 {
		t.Fatalf("problems on a readable tree: %v", problems)
	}
	if len(files) != 1 {
		t.Fatalf("expected only real.mut, got %d: %v", len(files), files)
	}
	if !strings.HasSuffix(files[0], "real.mut") {
		t.Fatalf("collected %q, want real.mut", files[0])
	}
}

// `mutant test .` is how the documented invocation is written, and the root's
// name is then literally ".". It must not be read as a dot-directory.
func TestCollectFilesAcceptsTheCurrentDirectoryAsRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a_test.mut"), []byte("let a = 1;\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	restore, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(restore)

	files, problems := collectMutantTestFiles([]string{"."})
	if len(problems) != 0 {
		t.Fatalf("problems on a readable directory: %v", problems)
	}
	if len(files) != 1 {
		t.Fatalf("expected the one test file in \".\", got %d: %v", len(files), files)
	}
}

// A file argument that is not Mutant source is refused and named. Every
// non-directory argument used to be appended unchecked, so `mutant fmt
// notes.md` ran the Mutant formatter over Markdown and wrote the result back.
// (M26-TOOL-020)
func TestCollectFilesRefusesANonMutantFile(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"notes.md", "config.json", "script.sh", "noextension"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	for _, name := range []string{"notes.md", "config.json", "script.sh", "noextension"} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(root, name)
			files, problems := collectMutantSourceFiles([]string{path})
			if len(files) != 0 {
				t.Fatalf("%s was collected as Mutant source: %v", name, files)
			}
			if len(problems) != 1 {
				t.Fatalf("expected one problem naming %s, got %v", name, problems)
			}
			if !strings.Contains(problems[0].Error(), name) {
				t.Fatalf("the problem does not name the file: %v", problems[0])
			}
		})
	}
}

// The refusal is per argument and not per run: the .mut files given alongside
// a refused one are still collected, which is what makes `mutant fmt *` in a
// mixed directory format the source and leave everything else alone.
func TestARefusedArgumentDoesNotDiscardTheOthers(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"a.mut", "notes.md", "b.mut"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("let a = 1;\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	files, problems := collectMutantSourceFiles([]string{
		filepath.Join(root, "a.mut"),
		filepath.Join(root, "notes.md"),
		filepath.Join(root, "b.mut"),
	})
	if len(files) != 2 {
		t.Fatalf("expected both .mut files, got %d: %v", len(files), files)
	}
	if len(problems) != 1 {
		t.Fatalf("expected the one refusal, got %v", problems)
	}
}

// A path that does not exist is reported rather than returned as the only
// problem: the arguments after it are still examined, so one typo does not
// hide a second.
func TestEveryBadArgumentIsReported(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.mut"), []byte("let a = 1;\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	files, problems := collectMutantSourceFiles([]string{
		filepath.Join(root, "nope.mut"),
		filepath.Join(root, "a.mut"),
		filepath.Join(root, "alsonope.mut"),
	})
	if len(files) != 1 {
		t.Fatalf("expected the one real file, got %d: %v", len(files), files)
	}
	if len(problems) != 2 {
		t.Fatalf("expected both missing paths to be reported, got %v", problems)
	}
}

// A run that skipped something must not exit 0. Both halves are checked: fmt
// formats the file it was given and still reports the one it refused, and the
// file it refused is left byte for byte as it was.
func TestFmtRefusesANonMutantFileAndLeavesItAlone(t *testing.T) {
	root := t.TempDir()
	markdown := filepath.Join(root, "notes.md")
	// The trailing double space is a line break in Markdown and the CRLF
	// endings are the file's own; the formatter stripped both.
	original := []byte("# Title\r\nline one  \r\n\r\n| a | b |   \r\n")
	if err := os.WriteFile(markdown, original, 0o644); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "messy.mut")
	if err := os.WriteFile(source, []byte("let   x=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if code := handleFmtCommand([]string{"mutant", "fmt", markdown, source}); code == 0 {
		t.Fatal("mutant fmt exited 0 having skipped a file")
	}

	after, err := os.ReadFile(markdown)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(original) {
		t.Fatalf("the Markdown file was rewritten:\n%q\nwant\n%q", after, original)
	}

	formatted, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(formatted), "let   x=1") {
		t.Fatalf("the .mut file beside it was not formatted: %q", formatted)
	}
}

func TestLintRefusesANonMutantFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notes.md")
	if err := os.WriteFile(path, []byte("# Title\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if code := handleLintCommand([]string{"mutant", "lint", path}); code == 0 {
		t.Fatal("mutant lint exited 0 having skipped a file")
	}
}
