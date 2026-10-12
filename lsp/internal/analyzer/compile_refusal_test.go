package analyzer

// The compiler's refusals, held against what the editor reports.
//
// The editor used to know the compiler's refusals only by having them written a
// second time, as rules of its own, and most were never written: a break outside
// a loop, an assignment to a builtin, a struct literal missing a field, a for-in
// binding one name twice and more drew nothing in the editor and a failed build
// from `mutant gen`. Measured at 2c5f24f against the table below, 28 of the 38
// programs the compiler refuses carried no Error at all (M26-LSP-028).
//
// So the property asserted is the one an author relies on, and every row is
// decided by compiling it rather than by assertion: a program the build refuses
// carries an Error, and a program the build accepts carries no refusal in the
// compiler's name. Which part of the editor raises the Error is not the point --
// a rule that already reports a refusal where the compiler raised it is the
// better copy, because it reports every occurrence where the compiler stops at
// the first -- but when the compiler's own copy is shown, it says what the
// compiler says.

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"mutant/compiler"
	"mutant/generator"
	"mutant/lexer"
	"mutant/parser"
	"mutant/sema"

	lsp "github.com/tliron/glsp/protocol_3_16"
)

// diagnosticsFrom returns the diagnostics whose source is one of sources. The
// sources are spelled out at each call rather than taken from the package's
// constants, because a source is what a client filters on and what `mutant
// lint` prints: renaming one is a change a test should notice.
func diagnosticsFrom(diagnostics []lsp.Diagnostic, sources ...string) []lsp.Diagnostic {
	out := make([]lsp.Diagnostic, 0, 1)
	for _, d := range diagnostics {
		if d.Source == nil {
			continue
		}
		for _, source := range sources {
			if *d.Source == source {
				out = append(out, d)
				break
			}
		}
	}
	return out
}

func hasErrorSeverity(diagnostics []lsp.Diagnostic) bool {
	for _, d := range diagnostics {
		if d.Severity != nil && *d.Severity == lsp.DiagnosticSeverityError {
			return true
		}
	}
	return false
}

// describeDiagnostics renders diagnostics for a failure message, one per line:
// severity, 1-based position, source and message.
func describeDiagnostics(diagnostics []lsp.Diagnostic) string {
	lines := make([]string, 0, len(diagnostics))
	for _, d := range diagnostics {
		severity := "no severity"
		if d.Severity != nil {
			switch *d.Severity {
			case lsp.DiagnosticSeverityError:
				severity = "error"
			case lsp.DiagnosticSeverityWarning:
				severity = "warning"
			case lsp.DiagnosticSeverityInformation:
				severity = "information"
			case lsp.DiagnosticSeverityHint:
				severity = "hint"
			}
		}
		source := ""
		if d.Source != nil {
			source = *d.Source
		}
		lines = append(lines, fmt.Sprintf("  %s at %d:%d %s %q", severity,
			d.Range.Start.Line+1, d.Range.Start.Character+1, source, d.Message))
	}
	if len(lines) == 0 {
		return "  (nothing)"
	}
	return strings.Join(lines, "\n")
}

// buildRefusal is what the build says about a one-file program: the first line
// of the parser's first error, or of the compiler's refusal, or "" when it
// compiles. parsed is false when the parser stopped it -- `mutant gen` never
// compiles a program that did not parse, so that refusal is the parser's.
func buildRefusal(src string) (refusal string, parsed bool) {
	p := parser.New(lexer.New(src))
	program := p.ParseProgram()
	if errs := p.TypedErrors(); len(errs) > 0 {
		return firstLine(errs[0].Msg), false
	}
	if err := compiler.New().Compile(program); err != nil {
		return firstLine(err.Error()), true
	}
	return "", true
}

// compilerRefusalCases holds a program for each refusal the compiler makes that
// a one-file program can reach, and beside them the shapes that must stay
// clean. A refusal only a file with imports can reach is in the test after this
// one. One only a program that does not parse could reach is the parser's to
// report, and the one such row kept below shows the editor does not compile a
// program the build would stop at the parser.
// refused is the build's verdict, decided by compiling and pinned here, so that
// a change to the compiler turns a row stale instead of quietly turning the
// table into a list of programs that compile.
var compilerRefusalCases = []struct {
	name    string
	src     string
	refused bool
}{
	// break and continue with no loop of their own to leave.
	{name: "a break in a closure inside a loop", refused: true,
		src: "for (let i = 0; i < 2; i++) {\n    let f = fn() { break; };\n    f();\n}\n"},
	{name: "a continue in a closure inside a for-in loop", refused: true,
		src: "for (x in [1, 2]) {\n    let g = fn() { continue; };\n    g();\n}\n"},
	{name: "a break at the top level", refused: true, src: "break;\n"},
	{name: "a continue at the top level", refused: true, src: "continue;\n"},
	{name: "a break in a function body", refused: true, src: "let f = fn() { break; };\nf();\n"},
	{name: "a continue in a for loop's post section", refused: true,
		src: "for (let i = 0; i < 3; if (true) { continue; }) {\n    putln(i);\n}\n"},

	// Assignments the compiler will not store.
	{name: "an assignment to a builtin", refused: true, src: "len = 5;\n"},
	{name: "a compound assignment to a builtin", refused: true, src: "len += 1;\n"},
	{name: "an assignment to the function being defined", refused: true,
		src: "let f = fn() { f = 1; };\nf();\n"},
	{name: "an assignment to the function being defined, two functions deep", refused: true,
		src: "let f = fn() {\n    let g = fn() { f = 1; };\n    g();\n};\nf();\n"},
	{name: "an assignment to an element of a literal", refused: true, src: "[1, 2][0] = 9;\n"},
	{name: "an assignment through an index that is a call", refused: true,
		src: "let a = [[1]];\nlet k = fn() { return 0; };\na[k()][0] = 2;\n"},
	// A bare assignment to a name nothing declares used to compile -- it made
	// the name an implicit let -- while the editor's undefined-name rule
	// reported it at Error. Only a let declares, so the build now refuses it
	// too (M26-LSP-035).
	{name: "an assignment to a name nothing declares", refused: true, src: "y = 1;\n"},
	{name: "a field write on a name nothing declares", refused: true, src: "nope.y = 1;\n"},
	{name: "an index write on a name nothing declares", refused: true, src: "nope[0] = 1;\n"},

	// for-in headers.
	{name: "a for-in whose iterable names its own binding", refused: true,
		src: "for (x in x) {\n    putln(x);\n}\n"},
	{name: "a for-in that binds one name twice", refused: true,
		src: "for (k, k in [1, 2]) {\n    putln(k);\n}\n"},

	// Names that are not there.
	{name: "a read of a name nothing declares", refused: true, src: "putln(nope);\n"},
	{name: "a read before the declaration", refused: true, src: "putln(later);\nlet later = 1;\n"},
	{name: "a block's local read after the block", refused: true,
		src: "if (true) {\n    let inner = 1;\n}\nputln(inner);\n"},
	{name: "a struct literal of a type nothing declares", refused: true, src: "let p = Nope{x: 1};\n"},

	// Macros. Neither program declares a macro at its top level, so neither is
	// expanded and the editor's compile is the build's.
	{name: "a quote call outside a macro", refused: true, src: "putln(quote(1 + 2));\n"},
	{name: "a macro literal inside a function", refused: true,
		src: "let f = fn() {\n    let m = macro(x) { quote(unquote(x)); };\n    return 1;\n};\nf();\n"},

	// Struct literals and declarations.
	{name: "a struct literal missing a field", refused: true,
		src: "struct P { x, y };\nlet p = P{x: 1};\nputln(p);\n"},
	{name: "a struct literal setting a field the struct does not declare", refused: true,
		src: "struct P { x };\nlet p = P{x: 1, z: 2};\nputln(p);\n"},
	{name: "a struct literal setting one field twice", refused: true,
		src: "struct P { x };\nlet p = P{x: 1, x: 2};\nputln(p);\n"},
	{name: "a struct declaring one field twice", refused: true, src: "struct P { x, x };\nputln(1);\n"},
	{name: "a read of a field the struct does not declare", refused: true,
		src: "struct P { x };\nlet p = P{x: 1};\nputln(p.y);\n"},
	{name: "a write to a field the struct does not declare", refused: true,
		src: "struct P { x };\nlet p = P{x: 1};\np.y = 2;\n"},

	// A let that declares nothing new.
	{name: "a let declared twice", refused: true, src: "let x = 1;\nlet x = 2;\nputln(x);\n"},
	{name: "a let redeclared from its own value", refused: true, src: "let x = 1;\nlet x = x + 1;\nputln(x);\n"},
	// The parser refuses this one first, so its Error is the build's refusal.
	{name: "a parameter list naming one parameter twice", refused: true,
		src: "let f = fn(a, a) { return a; };\nf(1, 2);\n"},
	{name: "a parameter redeclared by its body", refused: true,
		src: "let f = fn(a) { let a = 2; return a; };\nf(1);\n"},
	{name: "a local declared twice in one body", refused: true,
		src: "let f = fn() { let y = 1; let y = 2; return y; };\nf();\n"},
	{name: "a pair rebound with nothing new", refused: true,
		src: "let a, err = gets();\nlet a, err = gets();\nputln(a, err);\n"},
	{name: "a pair's first name rebound alone", refused: true,
		src: "let a, err = gets();\nlet a = 1;\nputln(a, err);\n"},
	{name: "a consumed half of a pair rebound alone", refused: true,
		src: "let abc, err = gets();\nabc;\nlet abc = 1;\nabc;\nerr;\n"},
	{name: "a pair's value half rebound after the error was checked", refused: true,
		src: "let abc, err = gets();\nif (err != \"\") { putln(\"bad\"); }\nabc;\nlet abc = 1;\nabc;\n"},
	{name: "a let declared twice in one block", refused: true,
		src: "if (true) {\n    let m = 1;\n    let m = 2;\n    putln(m);\n}\n"},
	{name: "a let declared twice in a for-in body", refused: true,
		src: "for (v in [1, 2]) {\n    let w = v;\n    let w = 2;\n    putln(w);\n}\n"},

	// Redeclarations every block's own scope makes legal.
	{name: "a for-in binding redeclared by its body",
		src: "for (v in [1, 2]) {\n    let v = 9;\n    putln(v);\n}\n"},
	{name: "a counted loop's variable redeclared by its body",
		src: "for (let i = 0; i < 2; i++) {\n    let i = 9;\n    putln(i);\n}\n"},
	{name: "an enclosing name redeclared in a block",
		src: "let x = 1;\nif (true) {\n    let x = 2;\n    putln(x);\n}\nputln(x);\n"},
	{name: "an enclosing name redeclared two blocks deep",
		src: "let x = 1;\nif (true) {\n    if (true) {\n        let x = 2;\n        putln(x);\n    }\n}\nputln(x);\n"},
	{name: "two counted loops over one name",
		src: "for (let i = 0; i < 2; i++) {\n    putln(i);\n}\nfor (let i = 0; i < 2; i++) {\n    putln(i);\n}\n"},
	{name: "two blocks declaring one name",
		src: "if (true) {\n    let m = 1;\n    putln(m);\n};\nif (true) {\n    let m = 2;\n    putln(m);\n};\n"},
	{name: "two for-in loops over one name",
		src: "for (item in [1, 2]) { putln(item); }\nfor (item in [3, 4]) { putln(item); }\n"},
	{name: "a program with nothing wrong", src: "let x = 1;\nputln(x);\n"},
}

// TestEveryRefusalTheCompilerMakesIsAnEditorError runs the table through the
// build and through the editor. A refused row must carry an Error, and the
// compiler's own copy, wherever the editor shows one, must read exactly what the
// compiler said. An accepted row must carry no refusal in the compiler's words:
// neither the compiler's copy nor a refused redeclaration.
func TestEveryRefusalTheCompilerMakesIsAnEditorError(t *testing.T) {
	for _, c := range compilerRefusalCases {
		t.Run(c.name, func(t *testing.T) {
			refusal, parsed := buildRefusal(c.src)
			if refused := refusal != ""; refused != c.refused {
				t.Fatalf("the build's verdict is refused=%v (%q) and the row says %v, "+
					"so the row is stale:\n%s", refused, refusal, c.refused, c.src)
			}
			diagnostics := Diagnostics(New().Analyze(c.src), DefaultLintConfig())
			fromCompiler := diagnosticsFrom(diagnostics, "mutant-compiler")

			if !c.refused {
				if claimed := diagnosticsFrom(diagnostics, "mutant-compiler", "mutant-declarations"); len(claimed) > 0 {
					t.Fatalf("the build accepts this, and the editor refuses it in the "+
						"compiler's words:\n%s\n%s", describeDiagnostics(claimed), c.src)
				}
				return
			}
			if !hasErrorSeverity(diagnostics) {
				t.Fatalf("the build refuses this with %q, and the editor reports no "+
					"Error:\n%s\n%s", refusal, describeDiagnostics(diagnostics), c.src)
			}
			if !parsed {
				// The editor compiles only what the build would compile.
				if len(fromCompiler) > 0 {
					t.Fatalf("the parser refuses this (%q), and the editor compiled it "+
						"anyway:\n%s\n%s", refusal, describeDiagnostics(fromCompiler), c.src)
				}
				return
			}
			for _, d := range fromCompiler {
				if d.Message != refusal {
					t.Errorf("the editor reports the compiler's refusal as %q; the "+
						"compiler says %q\n%s", d.Message, refusal, c.src)
				}
			}
		})
	}
}

// importLibrary is the module the importers below name: a public function, a
// private value and a struct. importMacros declares a macro, which the build
// expands in every file that calls it.
const (
	importLibrary = "let helper = fn() { return 1; };\nlet _secret = 2;\nstruct Point { x, y };\n"
	importMacros  = "let twice = macro(x) { quote(unquote(x) * 2); };\n"
)

// TestARefusalAcrossAnImportIsReportedInTheBuildsWords is the same property for
// a file with imports, where what the compiler refuses can depend on another
// module: a struct literal of an imported type, a write through a namespace, a
// struct name two modules declare. Those three, and a break in a file with
// imports, drew nothing in the editor at 2c5f24f.
//
// The editor sees the imports through a workspace filled the way the language
// server fills one, which never reads a file. The build is the real one --
// `mutant gen`'s module walk over the same files, written to disk for it. A
// refusal must reach the editor exactly once, as an Error, in the build's own
// sentence: paths included, because both were handed the same absolute paths.
// Two rows are reported by the module-member rule rather than by the compiler,
// and are here to show the compiler's copy is not shown beside that rule's.
func TestARefusalAcrossAnImportIsReportedInTheBuildsWords(t *testing.T) {
	for _, c := range []struct {
		name, src string
		refused   bool
	}{
		{name: "an imported struct literal missing a field", refused: true,
			src: "import \"lib.mut\";\nlet p = Point{x: 1};\nputln(p, lib.helper());\n"},
		{name: "an imported struct literal setting a field the struct does not declare", refused: true,
			src: "import \"lib.mut\";\nlet q = Point{x: 1, y: 2, z: 3};\nputln(q, lib.helper());\n"},
		{name: "a write through an import's namespace", refused: true,
			src: "import \"lib.mut\";\nlib.helper = fn() { return 5; };\nputln(lib.helper());\n"},
		{name: "a struct name an imported module declares too", refused: true,
			src: "import \"lib.mut\";\nstruct Point { z };\nputln(lib.helper());\n"},
		{name: "a break in a closure inside a loop, in a file with imports", refused: true,
			src: "import \"lib.mut\";\nfor (let i = 0; i < 2; i++) {\n    let f = fn() { break; };\n    f();\n}\nputln(lib.helper());\n"},
		{name: "a private name read through the namespace", refused: true,
			src: "import \"lib.mut\";\nputln(lib._secret);\n"},
		{name: "a name the imported module does not declare", refused: true,
			src: "import \"lib.mut\";\nputln(lib.nothere);\n"},
		{name: "a file with imports and nothing wrong",
			src: "import \"lib.mut\";\nlet p = Point{x: 1, y: 2};\nputln(p.x, lib.helper());\n"},
		// The build expands the call before it compiles. The editor does not
		// expand macros, so it must not compile this file and call twice an
		// undefined name.
		{name: "a call to a macro an imported module declares",
			src: "import \"macros.mut\";\nputln(twice(21));\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			root := t.TempDir()
			workspace := sema.NewWorkspace(nil)
			for name, src := range map[string]string{"lib.mut": importLibrary, "macros.mut": importMacros, "main.mut": c.src} {
				path := filepath.Join(root, name)
				if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
					t.Fatal(err)
				}
				workspace.PutFile("file:///"+filepath.ToSlash(path), path, parser.New(lexer.New(src)).ParseProgram())
			}
			entry := filepath.Join(root, "main.mut")

			refusal := ""
			if _, err, _, _ := generator.CompileForTest(entry, nil); err != nil {
				refusal = firstLine(err.Error())
			}
			if refused := refusal != ""; refused != c.refused {
				t.Fatalf("the build's verdict is refused=%v (%q) and the row says %v, "+
					"so the row is stale:\n%s", refused, refusal, c.refused, c.src)
			}
			diagnostics := Diagnostics(New().AnalyzeInWorkspace(entry, c.src, workspace), DefaultLintConfig())

			if !c.refused {
				if claimed := diagnosticsFrom(diagnostics, "mutant-compiler", "mutant-declarations"); len(claimed) > 0 {
					t.Fatalf("the build accepts this, and the editor refuses it in the "+
						"compiler's words:\n%s\n%s", describeDiagnostics(claimed), c.src)
				}
				return
			}
			saidIt := make([]lsp.Diagnostic, 0, 1)
			for _, d := range diagnostics {
				if d.Message == refusal {
					saidIt = append(saidIt, d)
				}
			}
			if len(saidIt) != 1 || !hasErrorSeverity(saidIt) {
				t.Fatalf("the build refuses this with %q; want that sentence once, as "+
					"an Error, and the editor reports:\n%s\n%s", refusal, describeDiagnostics(diagnostics), c.src)
			}
		})
	}
}

// buildNeedsNothingTheEditorDeclines reports whether a build of the module
// filed under key asks for nothing the editor declines to do: every module of
// its closure is indexed and clean, and none of them declares a macro. The
// editor's compile of such a file is the build's compile, so it has to run.
//
// It is worked out from the workspace's facts rather than asked of the
// analyzer, so that it can disagree with the analyzer's own gate.
func buildNeedsNothingTheEditorDeclines(workspace *sema.Workspace, key string) bool {
	closure, cycles := workspace.Closure(key)
	if len(cycles) > 0 {
		return false
	}
	for _, k := range closure {
		facts, known := workspace.Facts(k)
		if !known || len(workspace.Diagnostics(k)) > 0 || len(facts.Macros) > 0 {
			return false
		}
	}
	return true
}

// TestNoExampleDrawsACompilerRefusal runs the editor over every shipped
// example, in one workspace the way `mutant lint examples` builds it. Every
// example compiles -- the example sweep compiles each one, whatever its mode --
// so a refusal reported on one, in the compiler's name or as a refused
// redeclaration, is a false claim.
//
// That is worth something only if the examples are really being compiled, so
// each one the editor ought to compile is compiled a second time with a break
// appended at its top level, which the build refuses. If the editor declined
// the file, or a rule had already hidden a refusal in it, that break draws
// nothing and the test says which example it was.
func TestNoExampleDrawsACompilerRefusal(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", "..", "examples"))
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(path, ".mut") {
			paths = append(paths, path)
		}
		return nil
	})
	if walkErr != nil {
		t.Skipf("examples not readable here: %v", walkErr)
	}
	if len(paths) == 0 {
		t.Fatalf("no .mut file under %s", root)
	}
	sort.Strings(paths)

	workspace := sema.NewWorkspace(nil)
	sources := make(map[string]string, len(paths))
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		sources[path] = string(raw)
		workspace.PutFile("file:///"+filepath.ToSlash(path), path, parser.New(lexer.New(string(raw))).ParseProgram())
	}

	// What the build says about a break at the top level, decided by compiling
	// one rather than written down here.
	appended, _ := buildRefusal("break;\n")
	if appended == "" {
		t.Fatal("the compiler accepts a break at the top level, so it cannot stand in for a refusal here")
	}
	compiled := 0
	var silent []string
	for _, path := range paths {
		rel, _ := filepath.Rel(root, path)
		snapshot := New().AnalyzeInWorkspace(path, sources[path], workspace)
		diagnostics := Diagnostics(snapshot, DefaultLintConfig())
		if claimed := diagnosticsFrom(diagnostics, "mutant-compiler", "mutant-declarations"); len(claimed) > 0 {
			t.Errorf("%s compiles, and the editor refuses it in the compiler's words:\n%s",
				rel, describeDiagnostics(claimed))
		}

		if len(snapshot.ParseErrors) > 0 || !buildNeedsNothingTheEditorDeclines(workspace, sema.CanonicalKey(path)) {
			continue
		}
		compiled++
		broken := New().AnalyzeInWorkspace(path, sources[path]+"\nbreak;\n", workspace)
		found := false
		for _, d := range diagnosticsFrom(Diagnostics(broken, DefaultLintConfig()), "mutant-compiler") {
			if d.Message == appended && d.Severity != nil && *d.Severity == lsp.DiagnosticSeverityError {
				found = true
			}
		}
		if !found {
			silent = append(silent, rel)
		}
	}
	if compiled == 0 {
		t.Fatalf("none of %d examples was one the editor should compile", len(paths))
	}
	if len(silent) > 0 {
		t.Errorf("%d of the %d examples the editor should compile draw no %q from the "+
			"compiler with a break appended at their top level, so the editor is not "+
			"compiling them:\n  %s", len(silent), compiled, appended, strings.Join(silent, "\n  "))
	}
	t.Logf("%d of %d examples are ones the editor should compile", compiled, len(paths))
}
