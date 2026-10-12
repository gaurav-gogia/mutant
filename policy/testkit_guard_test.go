package policy

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// testkitImport is the package that holds test-only helpers.
const testkitImport = "mutant/testkit"

// scanImports maps every non-test .go file under root, outside testkit/, to the
// import paths it names.
//
// testkit/ is excluded because it is the one package whose job is to hold
// test-only helpers, so it is allowed to import them and to import "testing"
// itself. Every other production file is scanned.
//
// Imports are read with go/parser rather than matched as text, which is not
// fussiness: "testing" appears as a plain string in builtin/metadata.go,
// builtin/goresym_builtins.go and cmd/gendocs/sections.go, none of which import
// it, so a grep would report three violations that are not there.
func scanImports(root string) (map[string][]string, error) {
	out := map[string][]string{}
	fset := token.NewFileSet()
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skippedDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".go") || strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		rel = filepath.ToSlash(rel)
		if rel == "testkit" || strings.HasPrefix(rel, "testkit/") {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		paths := make([]string, 0, len(file.Imports))
		for _, imp := range file.Imports {
			p, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				return err
			}
			paths = append(paths, p)
		}
		out[rel] = paths
		return nil
	})
	return out, err
}

// productionImports is scanImports over this repository.
func productionImports(t *testing.T) map[string][]string {
	t.Helper()

	root, err := filepath.Abs(repositoryRoot)
	if err != nil {
		t.Fatal(err)
	}
	out, err := scanImports(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) == 0 {
		t.Fatal("scanned no files; the walk is looking in the wrong place")
	}
	return out
}

// TestTestkitIsImportedOnlyByTests: a helper written for tests -- a goroutine
// snapshot, a settle loop -- has no business in a binary an examiner runs.
func TestTestkitIsImportedOnlyByTests(t *testing.T) {
	for file, imports := range productionImports(t) {
		for _, path := range imports {
			if path == testkitImport {
				t.Errorf("%s imports %s, which only tests may import", file, testkitImport)
			}
		}
	}
}

// TestTestingIsImportedOnlyByTests is the same rule for the package the testing
// helpers are built from, and it is the stricter of the two: importing
// "testing" does not merely pull test-only code into a package, it links the
// harness and its flag registrations into every binary built from it.
//
// Two production files did: the parser's helper file and the evaluator's, which
// held nothing but test helpers -- checkParserErrors, testEval, testLetStmt --
// and were named so that neither looked like a test file to the toolchain or to
// a reader, so the parser and the evaluator both carried "testing" into the
// shipped package. The evaluator's copy also made mutant/parser a production
// dependency of mutant/evaluator, which it is not. They are
// parser/parser_helpers_test.go and evaluator/eval_conds_test.go now
// (M26-LEX-019), which is the whole of the fix: the code was already test code.
// Their old names are not written here, because a comment citing a path that a
// clone of this repository does not have fails TestCitedPathsExist -- which is
// how this sentence was caught naming them.
//
// This is a guard and not a one-off cleanup because the cost of the mistake is
// invisible. A file of helpers compiles, vets, and passes every other gate;
// nothing says it is in the shipped package until someone reads an import list.
func TestTestingIsImportedOnlyByTests(t *testing.T) {
	for file, imports := range productionImports(t) {
		for _, path := range imports {
			if path == "testing" || strings.HasPrefix(path, "testing/") {
				t.Errorf("%s imports %q: a file holding test helpers is named _test.go, "+
					"so the harness does not reach the shipped package", file, path)
			}
		}
	}
}

// TestTheImportGuardCatchesEachViolation is the non-vacuity check the two tests
// above need, because both of them pass by finding nothing and a walk that
// scanned the wrong tree would pass the same way.
//
// The two bad files reproduce the import blocks that were really in the tree:
// the parser's helper file, and the evaluator's, which named mutant/parser as
// well. They are given neutral names rather than the names they had, so that
// nothing here reads as a path this repository still contains. The allowed
// files beside them are two that hold "testing" as a plain string and import no
// such thing, a _test.go file that imports it legitimately, and testkit's own,
// which is exempt.
func TestTheImportGuardCatchesEachViolation(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"shipped/parser_helpers.go":   "package parser\n\nimport (\n\t\"fmt\"\n\t\"mutant/ast\"\n\t\"testing\"\n)\n",
		"shipped/eval_helpers.go":     "package evaluator\n\nimport (\n\t\"mutant/parser\"\n\t\"testing\"\n)\n",
		"shipped/quiet.go":            "package shipped\n\nimport \"mutant/testkit\"\n",
		"shipped/metadata.go":         "package shipped\n\nvar names = [][2]string{{\"assert\", \"testing\"}}\n",
		"shipped/sections.go":         "package shipped\n\nvar category = \"testing\"\n",
		"shipped/parser_real_test.go": "package parser\n\nimport \"testing\"\n\nfunc TestX(t *testing.T) {}\n",
		"testkit/goroutines.go":       "package testkit\n\nimport \"testing\"\n",
	}
	for name, source := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	imports, err := scanImports(root)
	if err != nil {
		t.Fatal(err)
	}

	var sawTesting, sawTestkit []string
	for file, paths := range imports {
		for _, path := range paths {
			switch path {
			case "testing":
				sawTesting = append(sawTesting, file)
			case testkitImport:
				sawTestkit = append(sawTestkit, file)
			}
		}
	}

	if want := []string{"shipped/eval_helpers.go", "shipped/parser_helpers.go"}; !sameSet(sawTesting, want) {
		t.Errorf("the guard reports %v as importing testing, want %v", sorted(sawTesting), want)
	}
	if want := []string{"shipped/quiet.go"}; !sameSet(sawTestkit, want) {
		t.Errorf("the guard reports %v as importing testkit, want %v", sorted(sawTestkit), want)
	}
	// The _test.go file and testkit's own file must not even be scanned, so a
	// later reader cannot mistake their absence above for the rule allowing
	// them somewhere else.
	for _, skipped := range []string{"shipped/parser_real_test.go", "testkit/goroutines.go"} {
		if _, scanned := imports[skipped]; scanned {
			t.Errorf("%s was scanned; it is outside the rule", skipped)
		}
	}
}

func sorted(in []string) []string {
	out := append([]string(nil), in...)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

func sameSet(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	g := sorted(got)
	for i := range want {
		if g[i] != want[i] {
			return false
		}
	}
	return true
}
