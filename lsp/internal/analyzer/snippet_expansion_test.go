package analyzer

import (
	"regexp"
	"testing"
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
