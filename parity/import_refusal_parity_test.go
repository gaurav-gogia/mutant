package parity

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"testing"

	"mutant/lexer"
	"mutant/lsp/api"
	"mutant/module"
	"mutant/parser"
	"mutant/sema"
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
// loader names every file by its absolute path, as the workspace does. Every
// import in it spells its file in the file's own letter case: one that spells it
// in another case, which the workspace resolves only on Windows, where
// sema.CanonicalKey folds letter case, is the business of
// TestTheEditorsImportRefusalNamesAFileAsItsImportSpellsIt (M26-SEM-032).
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

// M26-SEM-032: on Windows, where a module's identity folds letter case, an
// import may spell a file in another case than the file on disk. module.Load
// names every file by the path its walk reached it through, which keeps the
// letters each import wrote in the case it wrote them, and the editor named
// every file below the entry as the workspace had filed it, so the two
// sentences differed in exactly the letters an import respelled.
//
// The editor is asked through api.Project, as above. Each case also lists the
// files the loader's sentence names, so that it cannot pass by the loader
// ceasing to name a file as its import spelled it. A build of b.mut, c.mut or
// e.mut is refused by a cycle whose closing import respells the entry, so the
// loader names that file two ways, and the editor has to as well. p.mut, q.mut,
// d.mut and sub/s.mut reach nothing through a respelled import: they are the
// controls, which held before.
func TestTheEditorsImportRefusalNamesAFileAsItsImportSpellsIt(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("sema.CanonicalKey folds letter case only on Windows, so only there does an import of B.mut name the module b.mut")
	}
	tree := map[string]string{
		// a.mut spells b.mut as B.mut and closes a cycle; main.mut walks into
		// it. Both imports of the c.mut and e.mut cycle respell their file.
		"a.mut":    "import b \"B.mut\";\nlet fa = 1;\n",
		"b.mut":    "import a \"a.mut\";\nlet fb = 1;\n",
		"main.mut": "import a \"a.mut\";\nlet m = 1;\n",
		"c.mut":    "import e \"E.mut\";\nlet fc = 1;\n",
		"e.mut":    "import c \"C.mut\";\nlet fe = 1;\n",
		// d.mut binds util twice and is reached as D.mut; sub/s.mut binds it
		// twice and is reached through the directory spelled SUB.
		"usesdup.mut": "import dep \"D.mut\";\nlet u = 1;\n",
		"d.mut":       "import \"x/util.mut\";\nimport \"y/util.mut\";\nlet d = 1;\n",
		"usessub.mut": "import s \"SUB/s.mut\";\nlet v = 1;\n",
		"sub/s.mut":   "import \"../x/util.mut\";\nimport \"../y/util.mut\";\nlet s = 1;\n",
		"x/util.mut":  "let one = 1;\n",
		"y/util.mut":  "let two = 2;\n",
		// A cycle whose imports spell their files as they are on disk.
		"p.mut": "import q \"q.mut\";\nlet fp = 1;\n",
		"q.mut": "import p \"p.mut\";\nlet fq = 1;\n",
	}
	root := writeTree(t, tree)
	paths := make([]string, 0, len(tree))
	for rel := range tree {
		paths = append(paths, filepath.Join(root, filepath.FromSlash(rel)))
	}
	sort.Strings(paths)
	project := api.NewProject(paths)

	for _, c := range []struct {
		file string
		// verdict is what the loader refuses the file for.
		verdict string
		// named are the files the loader's sentence names, relative to the
		// tree and spelled as its walk reached them: a cycle's chain, or the
		// file that binds a name twice.
		named []string
	}{
		{"a.mut", "cycle", []string{"a.mut", "B.mut", "a.mut"}},
		{"b.mut", "cycle", []string{"b.mut", "a.mut", "B.mut"}},
		{"main.mut", "cycle", []string{"main.mut", "a.mut", "B.mut", "a.mut"}},
		{"c.mut", "cycle", []string{"c.mut", "E.mut", "C.mut"}},
		{"e.mut", "cycle", []string{"e.mut", "C.mut", "E.mut"}},
		{"usesdup.mut", "duplicate", []string{"D.mut"}},
		{"usessub.mut", "duplicate", []string{"SUB/s.mut"}},
		{"p.mut", "cycle", []string{"p.mut", "q.mut", "p.mut"}},
		{"q.mut", "cycle", []string{"q.mut", "p.mut", "q.mut"}},
		{"d.mut", "duplicate", []string{"d.mut"}},
		{"sub/s.mut", "duplicate", []string{"sub/s.mut"}},
	} {
		t.Run(c.file, func(t *testing.T) {
			entry := filepath.Join(root, filepath.FromSlash(c.file))
			_, err := module.Load(entry, nil)
			var (
				cycle     *module.CycleError
				duplicate *module.DuplicateNamespaceError
				verdict   string
				named     []string
			)
			switch {
			case errors.As(err, &cycle):
				verdict, named = "cycle", cycle.Chain
			case errors.As(err, &duplicate):
				verdict, named = "duplicate", []string{duplicate.Importer}
			}
			want := make([]string, len(c.named))
			for i, rel := range c.named {
				want[i] = filepath.Join(root, filepath.FromSlash(rel))
			}
			if verdict != c.verdict || !slices.Equal(named, want) {
				t.Fatalf("the loader's verdict on %s is %q naming %q (%v), but the case is written for %q naming %q",
					c.file, verdict, named, err, c.verdict, want)
			}

			var said []string
			for _, d := range project.Lint(entry, tree[c.file]) {
				if d.Source == "mutant-modules" {
					said = append(said, d.Message)
				}
			}
			if len(said) != 1 || said[0] != err.Error() {
				t.Errorf("the loader refuses %s with\n  %q\nand the editor says\n  %q", c.file, err.Error(), said)
			}
		})
	}
}

// An entry filed under an uncleaned path -- a.mut/., or the same with a trailing
// separator or a last element of ".." -- has a.mut's key, but the path itself is
// not a.mut's: its filepath.Dir is a.mut itself or a directory below it.
// module.Load cleans the path it is given before it names the entry or resolves
// an import from its directory, and the editor has to as well. Resolving from
// the uncleaned path's directory finds no import, so the cycle a build of a.mut
// is refused for goes unsaid; naming the entry by that path gives a sentence
// the loader never prints, which is what the editor said of a.mut and d.mut
// before M26-SEM-032.
//
// The workspace is asked directly, filed the way the language server files a
// document opened under such a path, which it does on every system. api.Project
// is asked as well wherever os.ReadFile reads the path, because NewProject
// skips a file it cannot read: on Windows it reads one whose last element is
// "." or "..", and on Linux none of them.
func TestTheEditorsImportRefusalNamesAnUncleanEntryAsTheLoaderDoes(t *testing.T) {
	tree := map[string]string{
		"a.mut":      "import b \"b.mut\";\nlet fa = 1;\n",
		"b.mut":      "import a \"a.mut\";\nlet fb = 1;\n",
		"d.mut":      "import \"x/util.mut\";\nimport \"y/util.mut\";\nlet d = 1;\n",
		"x/util.mut": "let one = 1;\n",
		"y/util.mut": "let two = 2;\n",
	}
	root := writeTree(t, tree)
	sep := string(filepath.Separator)
	windows := runtime.GOOS == "windows"
	for _, c := range []struct {
		file string
		// verdict is what the loader refuses the file for.
		verdict string
		// named are the files the loader's sentence names, relative to the
		// tree: a cycle's chain, or the file that binds a name twice.
		named []string
	}{
		{"a.mut", "cycle", []string{"a.mut", "b.mut", "a.mut"}},
		{"d.mut", "duplicate", []string{"d.mut"}},
	} {
		for _, unclean := range []struct {
			name, suffix string
			// read is whether os.ReadFile reads the entry by this path.
			read bool
		}{
			{"dot", sep + ".", windows},
			{"separator", sep, false},
			{"parent", sep + "x" + sep + "..", windows},
		} {
			t.Run(c.file+"-"+unclean.name, func(t *testing.T) {
				entry := filepath.Join(root, c.file) + unclean.suffix
				_, err := module.Load(entry, nil)
				var (
					cycle     *module.CycleError
					duplicate *module.DuplicateNamespaceError
					verdict   string
					named     []string
				)
				switch {
				case errors.As(err, &cycle):
					verdict, named = "cycle", cycle.Chain
				case errors.As(err, &duplicate):
					verdict, named = "duplicate", []string{duplicate.Importer}
				}
				want := make([]string, len(c.named))
				for i, rel := range c.named {
					want[i] = filepath.Join(root, rel)
				}
				if verdict != c.verdict || !slices.Equal(named, want) {
					t.Fatalf("the loader's verdict on %s is %q naming %q (%v), but the case is written for %q naming %q",
						entry, verdict, named, err, c.verdict, want)
				}

				workspace := sema.NewWorkspace(nil)
				paths := make([]string, 0, len(tree))
				for rel, src := range tree {
					path := filepath.Join(root, filepath.FromSlash(rel))
					if rel == c.file {
						path = entry
					}
					workspace.PutFile("file:///"+filepath.ToSlash(path), path, parser.New(lexer.New(src)).ParseProgram())
					paths = append(paths, path)
				}
				sort.Strings(paths)
				var said []string
				for _, d := range workspace.Diagnostics(sema.CanonicalKey(entry)) {
					said = append(said, d.Message)
				}
				if len(said) != 1 || said[0] != err.Error() {
					t.Errorf("the loader refuses %s with\n  %q\nand the workspace says\n  %q", entry, err.Error(), said)
				}

				if _, readErr := os.ReadFile(entry); (readErr == nil) != unclean.read {
					t.Fatalf("os.ReadFile(%q) = %v, but the case is written for read = %v", entry, readErr, unclean.read)
				}
				if !unclean.read {
					return
				}
				said = nil
				for _, d := range api.NewProject(paths).Lint(entry, tree[c.file]) {
					if d.Source == "mutant-modules" {
						said = append(said, d.Message)
					}
				}
				if len(said) != 1 || said[0] != err.Error() {
					t.Errorf("the loader refuses %s with\n  %q\nand api.Project says\n  %q", entry, err.Error(), said)
				}
			})
		}
	}
}
