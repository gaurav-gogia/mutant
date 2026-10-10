package parser

import (
	"strings"
	"testing"

	"mutant/sema"
)

// A parameter list naming one parameter twice was refused by the compiler and by
// nothing else, and a macro never reaches the compiler's copy: a macro literal
// is expanded by the evaluator, which has no such check at all. So `macro(a, a)`
// compiled and ran with the second parameter winning, while `fn(a, a)` was
// refused -- the two engines disagreeing about the same list. The parser is the
// one place both go through. These tests fail at 4360eec (M26-LEX-013).
//
// It also moves the refusal from the lint warning it was in the editor to an
// error, which is what a list that can never run should show.

func TestADuplicateFunctionParameterIsRefusedWhereItIsWritten(t *testing.T) {
	fault := oneFault(t, "let g = fn(a, a) { return a; };")

	if !strings.Contains(fault.Msg, "declared twice") {
		t.Fatalf("the error does not say the parameter is declared twice: %q", fault.Msg)
	}
	// Column 15 is the second `a`, which is the one that cannot be read.
	if got := fault.Range.Start.Column; got != 15 {
		t.Errorf("the error is at column %d, want 15 (the second `a`)", got)
	}
}

// The row's own case, and the half nothing refused: this compiled at exit 0.
func TestADuplicateMacroParameterIsRefusedToo(t *testing.T) {
	fault := oneFault(t, "let m = macro(a, a) { quote(a); };")

	if !strings.Contains(fault.Msg, "declared twice") {
		t.Fatalf("the error does not say the parameter is declared twice: %q", fault.Msg)
	}
}

// One sentence, in one place. The parser cannot call sema -- it depends on
// token, ast and lexer and nothing else, which is what lets the formatter and
// the language server parse a file without the object model and the builtin
// table coming with it -- so the sentence is written out, and held here.
func TestTheDuplicateParameterSentenceIsTheOneSemaWrites(t *testing.T) {
	fault := oneFault(t, "let g = fn(a, a) { return a; };")
	want := sema.DuplicateParameterRefusal("a").Message

	if fault.Msg != want {
		t.Fatalf("the parser and sema phrase one refusal two ways.\n"+
			"parser: %q\nsema:   %q\n"+
			"If sema's wording changed, change the parser's with it.", fault.Msg, want)
	}
}

// The discard slot is exempt, for the reason compiler.blankName exists: a
// parameter list is positional, so all three slots have to stay or c would read
// the wrong argument. Two discards are also not two readings of one name,
// because neither can be read.
func TestTwoDiscardParametersAreNotADuplicate(t *testing.T) {
	plain, _ := faults("let f = fn(_, _, c) { return c; };")

	if len(plain) != 0 {
		t.Fatalf("a list with two discards was refused: %v", plain)
	}
}

func TestWellFormedParameterListsAreUntouched(t *testing.T) {
	sources := []string{
		"let f = fn(a, b, c) { return a; };",
		"let f = fn() { return 5; };",
		"let f = fn(a) { return a; };",
		"let m = macro(a, b) { quote(a); };",
		// One name per list, twice over: two lists each naming `a` are not a
		// duplicate, because a parameter list is its own scope.
		"let f = fn(a) { return a; }; let g = fn(a) { return a; };",
		// And a nested list naming the outer one's parameter is shadowing,
		// which this language allows and which is not this rule.
		"let f = fn(a) { let g = fn(a) { return a; }; return g(a); };",
	}

	for _, src := range sources {
		t.Run(src, func(t *testing.T) {
			plain, _ := faults(src)
			if len(plain) != 0 {
				t.Fatalf("%s produced %v", src, plain)
			}
		})
	}
}

// Three of one name is still one refusal per repeat, and each names its own
// position, so a reader fixing them sees every one rather than the first.
func TestEveryRepeatOfAParameterNameIsReported(t *testing.T) {
	plain, typed := faults("let f = fn(a, a, a) { return a; };")

	if len(typed) != 2 {
		t.Fatalf("three `a` parameters produced %d errors, want 2: %v", len(typed), plain)
	}
	if typed[0].Range.Start.Column == typed[1].Range.Start.Column {
		t.Errorf("both errors point at column %d", typed[0].Range.Start.Column)
	}
}
