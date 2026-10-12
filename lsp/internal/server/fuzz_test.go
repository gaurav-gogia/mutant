package server

import (
	"strings"
	"testing"

	"mutant/ast"
	"mutant/lsp/internal/analyzer"
)

// FuzzFormattingPreservesTheTree is the target M26-LEX-011's proposal asked for
// and that had nothing to run on: it wanted ast.Equal used in the fuzzers, and
// `go test -list '^Fuzz' ./...` found no fuzz target anywhere in the module, so
// the release gate's fuzz step reported "SKIP: no fuzz targets".
//
// The property is the formatter's whole contract. Formatting rewrites tokens on
// purpose -- it prints `((a))` as `(a)` and drops the optional terminator after
// a struct or enum declaration -- so the thing that has to survive is the tree,
// not the text. TestFormatterPreservesProgramStructureAcrossExamples checks it
// over the 123 example programs, which are the shapes someone wrote down; this
// checks it over the shapes nobody did.
//
// Nothing is evaluated. The analyzer reaches mutant/builtin for the metadata
// the hover and completion need and does not reach mutant/evaluator or
// mutant/vm, and the formatter is a printer over the tree, so no macro is
// expanded and no fuzz input is ever executed.
func FuzzFormattingPreservesTheTree(f *testing.F) {
	for _, seed := range []string{
		"let x = 1;",
		"let s = \"a${ b + 1 }c\";",
		"if (c) { a; } else { b; }",
		"for (let i = 0; i < 3; i++) { putln(i); }",
		"for k, v in h { putln(k); }",
		"while (c) { break; }",
		"let f = fn(a, b) { return a + b; };",
		"let m = macro(a) { quote(unquote(a)); };",
		"let h = {\"k\": 1, \"j\": [1, 2]};",
		"struct P { x; y; }",
		"enum C { RED, GREEN }",
		"match (x) { 1 => { a; }, 2 => { b; } }",
		"let z = -a * (b + c)[0].d;",
		"let p = P{x: 1};",
		"import p \"lib/p.mut\";",
		"let x, err = f();",
		"// a comment\nlet x = 1;\n",
		"let x = ((1 + 2));",
		"",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, src string) {
		before := analyzer.New().Analyze(src)
		if before == nil || before.Program == nil || len(before.ParseErrors) > 0 {
			// Only a program the formatter is willing to format can be checked,
			// and it refuses a snapshot with parse errors by design: there is
			// no canonical form for text that is not a program.
			return
		}

		formatted, parseErrors, ok := formatSnapshotText(before)
		if !ok {
			t.Fatalf("the formatter refused a snapshot with no parse errors: %v", parseErrors)
		}

		after := analyzer.New().Analyze(formatted)
		if after == nil || after.Program == nil || len(after.ParseErrors) > 0 {
			t.Fatalf("formatted output no longer parses.\nsource:    %q\nformatted: %q\nerrors:    %v",
				src, formatted, after.ParseErrors)
		}

		if !ast.Equal(before.Program, after.Program) {
			t.Fatalf("formatting changed the program.\nsource:    %q\nformatted: %q",
				src, formatted)
		}
	})
}

// FuzzFormattingIsIdempotent is the companion property, and a cheaper one to
// violate: formatting output that is already formatted has to be a no-op. A
// formatter that is not idempotent makes every file it touches a diff, and the
// tree comparison above cannot see it, because both rounds have the same tree.
func FuzzFormattingIsIdempotent(f *testing.F) {
	for _, seed := range []string{
		"let x = 1;",
		"if (c) { a; } else { b; }",
		"let s = \"a${ b }c\";",
		"let h = {\"k\": 1};",
		"struct P { x; y; }",
		"match (x) { 1 => { a; } }",
		"// a comment\nlet x = 1;\n",
		"",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, src string) {
		once, ok := formatOnce(src)
		if !ok {
			return
		}
		twice, ok := formatOnce(once)
		if !ok {
			t.Fatalf("formatted output could not be formatted again: %q", once)
		}
		if once != twice {
			t.Fatalf("formatting is not idempotent.\nsource: %q\nonce:   %q\ntwice:  %q",
				src, once, twice)
		}
	})
}

// formatOnce formats src, or reports false when src is not a program the
// formatter will take.
func formatOnce(src string) (string, bool) {
	snapshot := analyzer.New().Analyze(src)
	if snapshot == nil || snapshot.Program == nil || len(snapshot.ParseErrors) > 0 {
		return "", false
	}
	formatted, _, ok := formatSnapshotText(snapshot)
	if !ok {
		return "", false
	}
	// A formatter is allowed to decide a program renders as nothing at all --
	// an empty source does -- and feeding that back in is not a second round.
	if strings.TrimSpace(formatted) == "" {
		return "", false
	}
	return formatted, true
}
