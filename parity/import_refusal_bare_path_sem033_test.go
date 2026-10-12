package parity

import (
	"errors"
	"path/filepath"
	"sort"
	"testing"

	"mutant/lsp/api"
	"mutant/module"
)

// M26-SEM-033: `mutant lint .` run inside a project hands api.NewProject every
// file by its path relative to the working directory, so a top-level module is
// a.mut, with no directory in its path. The build of a.mut, entered by that
// same relative path, joins a.mut's import onto "." and refuses the cycle; the
// editor gave a.mut no own-directory candidate, resolved nothing, and refused
// nothing. Here the working directory is the tree, every file is named
// relative to it, and each entry's refusal must be the loader's error byte for
// byte -- the same check TestTheEditorsImportRefusalIsTheLoaders makes over
// absolute paths.
func TestImportRefusalSEM033FromInsideTheTreeIsTheLoaders(t *testing.T) {
	tree := map[string]string{
		"a.mut":     "import b \"b.mut\";\nlet fa = 1;\n",
		"b.mut":     "import a \"a.mut\";\nlet fb = 1;\n",
		"m.mut":     "import s \"sub/s.mut\";\nlet m = 1;\n",
		"sub/s.mut": "import a \"../a.mut\";\nlet s = 1;\n",
		"leaf.mut":  "let l = 1;\n",
	}
	t.Chdir(writeTree(t, tree))

	paths := make([]string, 0, len(tree))
	for rel := range tree {
		paths = append(paths, filepath.FromSlash(rel))
	}
	sort.Strings(paths)
	project := api.NewProject(paths)

	for _, c := range []struct {
		file  string
		cycle bool
	}{
		{"a.mut", true},
		{"b.mut", true},
		{"m.mut", true},
		{filepath.Join("sub", "s.mut"), true},
		{"leaf.mut", false},
	} {
		_, err := module.Load(c.file, nil)
		var cycle *module.CycleError
		if c.cycle != errors.As(err, &cycle) || (!c.cycle && err != nil) {
			t.Fatalf("the loader's verdict on %s is %v, but the case is written for cycle=%v", c.file, err, c.cycle)
		}

		var said []string
		for _, d := range project.Lint(c.file, tree[filepath.ToSlash(c.file)]) {
			if d.Source == "mutant-modules" {
				said = append(said, d.Message)
			}
		}
		if err == nil {
			if len(said) != 0 {
				t.Errorf("the loader accepts %s, and the editor refuses it: %q", c.file, said)
			}
			continue
		}
		if len(said) != 1 || said[0] != err.Error() {
			t.Errorf("the loader refuses %s with\n  %q\nand the editor says\n  %q", c.file, err.Error(), said)
		}
	}
}
