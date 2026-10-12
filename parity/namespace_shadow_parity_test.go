package parity

import (
	"path/filepath"
	"strings"
	"testing"

	"mutant/lexer"
	"mutant/parser"
	"mutant/sema"
)

// A value bound under an import alias's name does not hide the import.
//
// sema.Resolver asks whether the left of `stats.mean` names an import before it
// asks about any binding -- the import is a declaration in this very file --
// and the compiler binds a file's aliases for the whole file. So behind
// `let stats = {"mean": 7};` in a function body, `stats.mean` is the module's
// mean, and a mean the module does not declare is refused there as anywhere
// else.
//
// The editor reads it through the same resolver, with the scope the graph sees
// at the use, and has to come out the same. What it answers for the member of
// an import namespace -- the module's declaration, or nothing at all where
// that declines, as it does while the module is unread or the build refuses
// the reach (M26-LSP-048) -- rests on this reading. One that let the local win
// would answer for a field of the hash, which the build never reads.
func TestALocalNamedLikeAnImportAliasDoesNotHideIt(t *testing.T) {
	const main = "import stats \"lib/stats.mut\";\n" +
		"let f = fn() { let stats = {\"mean\": 7}; return stats.mean; };\n" +
		"f();\n"

	t.Run("a member the module declares", func(t *testing.T) {
		root := writeTree(t, map[string]string{"main.mut": main, "lib/stats.mut": "let mean = 42;\n"})
		value, err := runViaModules(t, root)
		if err != nil {
			t.Fatalf("the program did not run: %v", err)
		}
		if got := normalize(value); got != "INTEGER(42)" {
			t.Fatalf("f() = %s, want INTEGER(42), lib/stats.mut's mean; INTEGER(7) would be the local's field", got)
		}
		if resolved := editorReading(t, root, main); resolved.Kind != sema.FieldModuleMember || resolved.Confidence != sema.Certain {
			t.Fatalf("the editor reads stats.mean behind the local as %s (confidence %d), where the compiler reads lib/stats.mut's mean",
				resolved.Kind, resolved.Confidence)
		}
	})

	t.Run("a member the module does not declare", func(t *testing.T) {
		root := writeTree(t, map[string]string{"main.mut": main, "lib/stats.mut": "let median = 42;\n"})
		compiled, message := compilerAccepts(t, filepath.Join(root, "main.mut"))
		if compiled {
			t.Fatal("the compiler accepted stats.mean behind the local, which lib/stats.mut does not declare")
		}
		resolved := editorReading(t, root, main)
		if resolved.Refusal == nil || !strings.Contains(message, resolved.Refusal.Error()) {
			t.Fatalf("the engines disagree about stats.mean behind the local:\n compiler: %s\n editor:   %s, refusal %v",
				message, resolved.Kind, resolved.Refusal)
		}
	})
}

// editorReading asks the editor's resolver what `stats.mean` is at its use in
// the function body of main, main.mut's source, the way the language server
// asks: the workspace a scan builds, with the scope the file's graph has at
// that point.
func editorReading(t *testing.T, root, main string) sema.FieldResolution {
	t.Helper()
	w := workspaceOverTree(t, root)
	key := sema.CanonicalKey(filepath.Join(root, "main.mut"))

	p := parser.New(lexer.New(main))
	program := p.ParseProgram()
	if errs := p.Errors(); len(errs) > 0 {
		t.Fatalf("main.mut did not parse: %v", errs)
	}

	// The graph counts lines and byte columns from 1.
	line := strings.Split(main, "\n")[1]
	at := strings.Index(line, "return stats.mean")
	if at < 0 {
		t.Fatalf("main.mut's second line holds no `return stats.mean`: %q", line)
	}
	local := sema.BuildFile(key, program, w, nil).LocalScopeAt(2, at+len("return ")+1)
	return w.ResolveField(key, local, "stats", "mean")
}

// An enum named like an import alias hides the import only from the enum's
// declaration on.
//
// The compiler asks about an enum before an import, as sema.Resolver does --
// `Colour.Red` predates modules -- and the enums it knows are the ones whose
// statements it has compiled so far. So in these programs `stats.mean`
// written above `enum stats { mean };` is still the module's mean, and refused
// like any other reach when the module declares none, while below the enum it
// is the enum's variant. Compiled, not run: a function written above the enum
// reads the module even when it is called below it.
//
// Graph.EnumDeclared counts an enum for the whole file, and so does the
// LocalScope Graph.LocalScopeAt builds from it. Graph.EnumDeclaredBefore asks
// only as far as a position, and the language server's ModuleMemberNameAt
// asks it where the use begins: what that answers for the member of an import
// namespace (M26-LSP-048) rests on this reading.
func TestAnEnumDeclaredBelowTheUseDoesNotHideTheImport(t *testing.T) {
	const below = "import stats \"lib/stats.mut\";\n" +
		"let m = stats.mean;\n" +
		"enum stats { mean };\n" +
		"m;\n"

	t.Run("below the use, a member the module declares", func(t *testing.T) {
		root := writeTree(t, map[string]string{"main.mut": below, "lib/stats.mut": "let mean = 42;\n"})
		value, err := runViaModules(t, root)
		if err != nil {
			t.Fatalf("the program did not run: %v", err)
		}
		if got := normalize(value); got != "INTEGER(42)" {
			t.Fatalf("m = %s, want INTEGER(42), lib/stats.mut's mean; an ENUM_VALUE would be the enum's variant", got)
		}
		if resolved := readingInSourceOrder(t, root, below, 2); resolved.Kind != sema.FieldModuleMember || resolved.Confidence != sema.Certain {
			t.Fatalf("the editor reads stats.mean above the enum as %s (confidence %d), where the compiler reads lib/stats.mut's mean",
				resolved.Kind, resolved.Confidence)
		}
	})

	t.Run("below the use, a member the module does not declare", func(t *testing.T) {
		root := writeTree(t, map[string]string{"main.mut": below, "lib/stats.mut": "let median = 42;\n"})
		compiled, message := compilerAccepts(t, filepath.Join(root, "main.mut"))
		if compiled {
			t.Fatal("the compiler accepted stats.mean above the enum, which lib/stats.mut does not declare")
		}
		resolved := readingInSourceOrder(t, root, below, 2)
		if resolved.Refusal == nil || !strings.Contains(message, resolved.Refusal.Error()) {
			t.Fatalf("the engines disagree about stats.mean above the enum:\n compiler: %s\n editor:   %s, refusal %v",
				message, resolved.Kind, resolved.Refusal)
		}
	})

	t.Run("below the use, in a function called below the enum", func(t *testing.T) {
		const main = "import stats \"lib/stats.mut\";\n" +
			"let f = fn() { return stats.mean; };\n" +
			"enum stats { mean };\n" +
			"f();\n"
		root := writeTree(t, map[string]string{"main.mut": main, "lib/stats.mut": "let mean = 42;\n"})
		value, err := runViaModules(t, root)
		if err != nil {
			t.Fatalf("the program did not run: %v", err)
		}
		if got := normalize(value); got != "INTEGER(42)" {
			t.Fatalf("f() = %s, want INTEGER(42), lib/stats.mut's mean; an ENUM_VALUE would be the enum's variant", got)
		}
		if resolved := readingInSourceOrder(t, root, main, 2); resolved.Kind != sema.FieldModuleMember || resolved.Confidence != sema.Certain {
			t.Fatalf("the editor reads stats.mean in f as %s (confidence %d), where the compiler reads lib/stats.mut's mean",
				resolved.Kind, resolved.Confidence)
		}
	})

	t.Run("above the use", func(t *testing.T) {
		const above = "import stats \"lib/stats.mut\";\n" +
			"enum stats { mean };\n" +
			"let m = stats.mean;\n" +
			"m;\n"
		root := writeTree(t, map[string]string{"main.mut": above, "lib/stats.mut": "let mean = 42;\n"})
		value, err := runViaModules(t, root)
		if err != nil {
			t.Fatalf("the program did not run: %v", err)
		}
		if got := normalize(value); got != "ENUM_VALUE(stats.mean(0))" {
			t.Fatalf("m = %s, want ENUM_VALUE(stats.mean(0)), the enum's variant; INTEGER(42) would be lib/stats.mut's mean", got)
		}
		if resolved := readingInSourceOrder(t, root, above, 3); resolved.Kind != sema.FieldEnumValue {
			t.Fatalf("the editor reads stats.mean below the enum as %s, where the compiler reads the enum's variant", resolved.Kind)
		}
	})
}

// readingInSourceOrder asks the editor's resolver what the `stats.mean` on
// main's line is, main.mut's source, the way ModuleMemberNameAt asks: the
// workspace a scan of root builds, the scope the file's graph has where the
// use begins, and an enum counted only if it is declared at or before there.
// line counts from 1.
func readingInSourceOrder(t *testing.T, root, main string, line int) sema.FieldResolution {
	t.Helper()
	w := workspaceOverTree(t, root)
	key := sema.CanonicalKey(filepath.Join(root, "main.mut"))

	p := parser.New(lexer.New(main))
	program := p.ParseProgram()
	if errs := p.Errors(); len(errs) > 0 {
		t.Fatalf("main.mut did not parse: %v", errs)
	}

	text := strings.Split(main, "\n")[line-1]
	at := strings.Index(text, "stats.mean")
	if at < 0 {
		t.Fatalf("main.mut's line %d holds no `stats.mean`: %q", line, text)
	}
	// The graph counts byte columns from 1, as it counts lines.
	column := at + 1
	graph := sema.BuildFile(key, program, w, nil)
	local := graph.LocalScopeAt(line, column)
	local.Enums = func(name string) bool { return graph.EnumDeclaredBefore(name, line, column) }
	return w.ResolveField(key, local, "stats", "mean")
}
