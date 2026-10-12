package api

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// M26-SEM-028: `mutant lint .` run from a project's own directory hands
// NewProject paths relative to the working directory, so the top-level module
// is "main.mut". Its import must resolve beside it, as the build resolves it,
// or every cross-module refusal disappears from lint while linting the same
// project from its parent directory reports it.
//
// t.Chdir makes this test serial; the paths it files are relative to the
// directory it moves into, which is what `mutant lint .` does.
func TestSEM028ProjectLintFromTheModulesOwnDirectoryResolvesImports(t *testing.T) {
	root := t.TempDir()
	mainSrc := "import stats \"lib/stats.mut\";\nputln(stats.nope);\n"
	files := map[string]string{
		"main.mut":                        mainSrc,
		filepath.Join("lib", "stats.mut"): "let mean = 1;\n",
	}
	if err := os.MkdirAll(filepath.Join(root, "lib"), 0o755); err != nil {
		t.Fatal(err)
	}
	for rel, src := range files {
		if err := os.WriteFile(filepath.Join(root, rel), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(root)

	project := NewProject([]string{"main.mut", filepath.Join("lib", "stats.mut")})
	var messages []string
	found := false
	for _, d := range project.Lint("main.mut", mainSrc) {
		messages = append(messages, d.Message)
		if d.Severity == SeverityError && strings.Contains(d.Message, "declares no nope") {
			found = true
		}
	}
	if !found {
		t.Errorf("linting main.mut from its own directory gave %q; want the refusal that stats.mut declares no nope, as linting from the parent reports", messages)
	}
}
