package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// On Windows the sandbox detector reads one process, the parent mutant was
// started by, and matches it by image name (security/sandbox_windows.go). So Git
// Bash is detected when its bash.exe is that parent, and a program in between --
// timeout, env, sh -c, cmd /c -- hides it: measured 2026-10-11 under
// M26-SEC-012, which the owner keeps deferred past 2.6.0. The tutorial's note on
// the halt said that launching from Git Bash "is detected" with no condition, a
// guarantee the detector does not give (M26-DOC1-021). This holds every sentence
// of the page that promises the detection to naming the parent it depends on.
func TestTutorialGitBashDetectionNamesTheParent(t *testing.T) {
	const doc = "docs/TUTORIAL_30_MIN.md"
	raw, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(doc)))
	if err != nil {
		t.Fatal(err)
	}
	// A blockquote carries one sentence over lines that each begin with ">".
	text := strings.Join(strings.Fields(strings.ReplaceAll(string(raw), "\n>", "\n")), " ")
	for _, sentence := range strings.Split(text, ". ") {
		if !strings.Contains(sentence, "Git Bash") || !strings.Contains(sentence, "is detected") {
			continue
		}
		if !strings.Contains(sentence, "parent") {
			t.Errorf("%s says Git Bash is detected without naming the parent process that detection depends on: %q. The detector reads only the process that started mutant, so a program in between hides the shell.", doc, sentence)
		}
	}
}
