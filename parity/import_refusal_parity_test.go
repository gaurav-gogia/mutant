package parity

import (
	"errors"
	"path/filepath"
	"sort"
	"testing"

	"mutant/lsp/api"
	"mutant/module"
)

// M26-SEM-006: the build refuses an import cycle and two imports binding one
// name, and the editor said nothing about either. It says so now, and this
// holds it to the build's sentence.
//
// Every file of the tree below is taken as the entry of a build, and
// module.Load -- the build's first step, whose error is what `mutant` prints --
// gives its verdict. Where it refuses the file for a cycle or a duplicate
// namespace, the editor's one import refusal for that file is the loader's
// error byte for byte; where it accepts the file, the editor refuses nothing.
// The editor is asked through api.Project, which runs the analyzer the language
// server publishes from and is what `mutant lint` reports.
//
// The tree is under t.TempDir(), outside the directory a test runs in, so the
// loader names every file by its absolute path, as the workspace does.
func TestTheEditorsImportRefusalIsTheLoaders(t *testing.T) {
	tree := map[string]string{
		// A cycle, a file in no cycle that imports one, and a file that
		// imports itself.
		"a.mut":    "import b \"b.mut\";\nlet fa = 1;\n",
		"b.mut":    "import a \"a.mut\";\nlet fb = 1;\n",
		"main.mut": "import a \"a.mut\";\nlet m = 1;\n",
		"self.mut": "import me \"self.mut\";\nlet s = 1;\n",
		// Two imports binding util, an importer of that file, and an import
		// back into it from the module the second import names: a cycle the
		// loader never reaches, because it refuses a repeated name before it
		// resolves the import.
		"dup.mut":     "import \"x/util.mut\";\nimport \"y/util.mut\";\nlet d = 1;\n",
		"usesdup.mut": "import dep \"dup.mut\";\nlet u = 1;\n",
		"y/util.mut":  "import back \"../dup.mut\";\nlet two = 2;\n",
		// Accepted: a leaf, one module under two names, a diamond, and an
		// import of a file that is on disk but was never given to the
		// project. Not indexed is not missing, so it is no refusal.
		"x/util.mut":       "let one = 1;\n",
		"twice.mut":        "import one \"x/util.mut\";\nimport two \"x/util.mut\";\nlet c = one.one + two.one;\n",
		"diamond.mut":      "import t \"twice.mut\";\nimport o \"x/util.mut\";\nlet z = t.c + o.one;\n",
		"unindexed.mut":    "import kept \"outside/kept.mut\";\nlet w = kept.k;\n",
		"outside/kept.mut": "let k = 1;\n",
	}
	root := writeTree(t, tree)
	paths := make([]string, 0, len(tree))
	for rel := range tree {
		if rel != "outside/kept.mut" {
			paths = append(paths, filepath.Join(root, filepath.FromSlash(rel)))
		}
	}
	sort.Strings(paths)
	project := api.NewProject(paths)

	for _, c := range []struct {
		file string
		// verdict is what the loader refuses the file for, or "" when it
		// accepts it.
		verdict string
	}{
		{"a.mut", "cycle"},
		{"b.mut", "cycle"},
		{"main.mut", "cycle"},
		{"self.mut", "cycle"},
		{"dup.mut", "duplicate"},
		{"usesdup.mut", "duplicate"},
		{"y/util.mut", "duplicate"},
		{"x/util.mut", ""},
		{"twice.mut", ""},
		{"diamond.mut", ""},
		{"unindexed.mut", ""},
	} {
		t.Run(c.file, func(t *testing.T) {
			entry := filepath.Join(root, filepath.FromSlash(c.file))
			_, err := module.Load(entry, nil)
			var (
				cycle     *module.CycleError
				duplicate *module.DuplicateNamespaceError
				verdict   string
			)
			switch {
			case err == nil:
			case errors.As(err, &cycle):
				verdict = "cycle"
			case errors.As(err, &duplicate):
				verdict = "duplicate"
			default:
				t.Fatalf("the loader refuses %s for a reason this test is not about: %v", c.file, err)
			}
			if verdict != c.verdict {
				t.Fatalf("the loader's verdict on %s is %q (%v), but the case is written for %q", c.file, verdict, err, c.verdict)
			}

			var said []string
			for _, d := range project.Lint(entry, tree[c.file]) {
				if d.Source != "mutant-modules" {
					continue
				}
				if d.Severity != api.SeverityError {
					t.Errorf("%s: %q is reported as %s, want error: the build refuses the file", c.file, d.Message, d.Severity)
				}
				said = append(said, d.Message)
			}

			if err == nil {
				if len(said) != 0 {
					t.Errorf("the loader accepts %s, and the editor refuses it: %q", c.file, said)
				}
				return
			}
			if len(said) != 1 || said[0] != err.Error() {
				t.Errorf("the loader refuses %s with\n  %q\nand the editor says\n  %q", c.file, err.Error(), said)
			}
		})
	}
}
