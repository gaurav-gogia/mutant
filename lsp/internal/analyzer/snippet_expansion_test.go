package analyzer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	mast "mutant/ast"
)

// absentValueLiterals are value spellings other languages have and Mutant does
// not. There is no literal for the null value in this language at all: it is
// not a keyword, and nothing binds it, so `let x = nil;` stops the compiler
// with "undefined variable: nil".
//
// A snippet whose default is one of these offers a line that does not build,
// and the reader has no way to tell a placeholder they are meant to rename from
// a keyword they are meant to keep -- which is why `return ${2:nil};` survived
// in the editor's own teaching feature.
var absentValueLiterals = []string{"nil", "null", "NULL", "None", "undefined", "void"}

var (
	// snippetPlaceholder matches ${1:default}, whose default is what lands in
	// the buffer when the reader accepts the snippet and types nothing.
	snippetPlaceholder = regexp.MustCompile(`\$\{\d+:([^}]*)\}`)
	// snippetTabStop matches ${1}, $1 and $0, which land as nothing.
	snippetTabStop = regexp.MustCompile(`\$\{?\d+\}?`)
)

// expandSnippetDefaults is the text the editor leaves behind when a reader
// accepts a snippet and presses nothing else.
func expandSnippetDefaults(insert string) string {
	return snippetTabStop.ReplaceAllString(
		snippetPlaceholder.ReplaceAllString(insert, "$1"), "")
}

// A snippet is the editor teaching the language, so its default expansion has
// to be a thing the language accepts. The guard-clause snippet returned `nil`,
// and a reader who tabbed through it replacing the condition and kept the
// return -- the obvious reading, because `nil` looks like a keyword -- got
// `undefined variable: nil` from the compiler and an error from `mutant lint`.
//
// Placeholder names a reader is expected to rename are not the subject here:
// `condition` and `items` are undefined too and that is what a placeholder is.
// What must not appear is a word the language has no spelling for at all.
func TestEverySnippetDefaultIsSomethingThisLanguageCanWrite(t *testing.T) {
	items := languageSnippetCompletionItems()
	if len(items) == 0 {
		t.Fatal("there are no language snippets to check")
	}

	for _, item := range items {
		if item.InsertText == nil {
			t.Errorf("snippet %q has no insert text", item.Label)
			continue
		}
		expanded := expandSnippetDefaults(*item.InsertText)

		if errs := New().Analyze(expanded).ParseErrors; len(errs) > 0 {
			t.Errorf("snippet %q does not parse with its defaults:\n%s\n%v",
				item.Label, expanded, errs)
			continue
		}

		undefined := undefinedNames(t, expanded)
		for _, literal := range absentValueLiterals {
			if undefined[literal] {
				t.Errorf("snippet %q offers `%s`, which this language has no "+
					"spelling for:\n%s", item.Label, literal, expanded)
			}
		}
	}
}

// The test above is worth something only while these words really are absent.
// If Mutant ever gains a null literal this fails and says to revisit the list,
// rather than passing quietly on a word that now compiles.
func TestTheAbsentValueLiteralsAreStillAbsent(t *testing.T) {
	for _, literal := range absentValueLiterals {
		src := "let guard = " + literal + ";\nputln(guard);\n"
		if !undefinedNames(t, src)[literal] {
			t.Errorf("`%s` is no longer an undefined identifier, so "+
				"absentValueLiterals needs revisiting: %q", literal, src)
		}
	}
}

// extensionSnippetBodies decodes a VS Code snippet file into the text each
// snippet inserts: its body lines joined with newlines, the way VS Code joins
// them. A body may be one string or an array of them.
func extensionSnippetBodies(t *testing.T, raw []byte) map[string]string {
	t.Helper()
	var file map[string]struct {
		Body json.RawMessage `json:"body"`
	}
	if err := json.Unmarshal(raw, &file); err != nil {
		t.Fatalf("the snippet file does not parse: %v", err)
	}
	bodies := make(map[string]string, len(file))
	for name, snippet := range file {
		var lines []string
		if err := json.Unmarshal(snippet.Body, &lines); err != nil {
			var line string
			if err := json.Unmarshal(snippet.Body, &line); err != nil {
				t.Fatalf("snippet %q: its body is neither a string nor an array of strings", name)
			}
			lines = []string{line}
		}
		bodies[name] = strings.Join(lines, "\n")
	}
	return bodies
}

// guardIfIn returns the first if statement in the body of the function the
// program's first let binds, or nil.
func guardIfIn(program *mast.Program) *mast.IfExpression {
	if program == nil || len(program.Statements) == 0 {
		return nil
	}
	let, ok := program.Statements[0].(*mast.LetStatement)
	if !ok {
		return nil
	}
	fn, ok := let.Value.(*mast.FunctionLiteral)
	if !ok || fn.Body == nil {
		return nil
	}
	for _, statement := range fn.Body.Statements {
		if expression, ok := statement.(*mast.ExpressionStatement); ok {
			if guard, ok := expression.Expression.(*mast.IfExpression); ok {
				return guard
			}
		}
	}
	return nil
}

// firstReturnedName is the name the first statement of block returns, or ""
// when that statement is not a return of a bare name.
func firstReturnedName(block *mast.BlockStatement) string {
	if block == nil || len(block.Statements) == 0 {
		return ""
	}
	ret, ok := block.Statements[0].(*mast.ReturnStatement)
	if !ok {
		return ""
	}
	value := ret.ReturnValue
	if value == nil && len(ret.ReturnValues) == 1 {
		value = ret.ReturnValues[0]
	}
	if name, ok := value.(*mast.Identifier); ok {
		return name.Value
	}
	return ""
}

// assertTheGuardTestsTheError expands the "guard return" snippet of the snippet
// file raw with every default accepted, inside a function that takes err, and
// asserts that what the reader is left with builds, tests the error itself and
// returns it. The failure quotes the expanded text rather than the tree,
// because the tree prints an empty string literal as nothing.
func assertTheGuardTestsTheError(t *testing.T, raw []byte) {
	t.Helper()
	body, ok := extensionSnippetBodies(t, raw)["guard return"]
	if !ok {
		t.Fatal(`the extension has no "guard return" snippet`)
	}
	expanded := expandSnippetDefaults(body)
	src := "let guarded = fn(err) {\n" + expanded + "\nreturn 0;\n};\nputln(guarded(0));\n"
	if refusal, _ := buildRefusal(src); refusal != "" {
		t.Fatalf("the guard snippet's default expansion does not build (%s):\n%s", refusal, expanded)
	}
	guard := guardIfIn(New().Analyze(src).Program)
	if guard == nil {
		t.Fatalf("the guard snippet's default expansion holds no if:\n%s", expanded)
	}
	condition, isName := guard.Condition.(*mast.Identifier)
	if !isName {
		t.Fatalf("the guard snippet expands to\n%s\nand its condition must be the error itself, "+
			"`if (err)`. A call that succeeds leaves err null, and null != \"\" is false today "+
			"only because == falls back to comparing rendered text, where null renders as \"\" "+
			"(M26-VM-010)", expanded)
	}
	if returned := firstReturnedName(guard.Consequence); returned != condition.Value {
		t.Errorf("the guard snippet tests %q but returns %q:\n%s", condition.Value, returned, expanded)
	}
}

// The extension's snippets teach the language too, and its guard clause taught
// a test that is right only by accident: `if (err != "")`. A call that succeeds
// leaves err null, and null != "" is false today only because == falls back to
// comparing rendered text, where null renders as "" (M26-VM-010) -- the day that
// fallback goes, the guard fires on every success. The language's test of an
// error is the error itself, `if (err)`: null is falsy and an error is truthy,
// whatever == does. That is what the unchecked-error lint and the language
// reference teach (M26-LSP-023).
//
// The snippet file is read from the extension rather than restated here, so the
// assertion is about what a reader is actually offered.
func TestTheExtensionGuardSnippetTestsTheErrorItself(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "mutant-vscode-extension", "snippets", "mutant.json"))
	if err != nil {
		t.Skipf("extension snippets not readable here: %v", err)
	}
	assertTheGuardTestsTheError(t, raw)
}
