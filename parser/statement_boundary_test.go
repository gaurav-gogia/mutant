package parser

import (
	"strings"
	"testing"
)

// Mutant mandates a `;` and inserts none, so two statements written on one line
// with no `;` between them are two statements -- and nothing said so above
// format severity. `let y = x--1;` was read as `let y = x--;` and then `1;`, and
// it ran: it printed 4 and 4, having mutated x, on a line the author wrote as
// one thing. These tests fail at 4360eec (M26-LEX-007).
//
// An omission across a line break is the other case entirely: there the author
// wrote two statements and left out a `;`, which is what they meant and what the
// formatter repairs, so it stays recoverable. The tests below pin both sides of
// that line, and parser_semicolon_test.go's five recoverable cases are the rest
// of the control -- each of them is an omission at end of input or before a `}`,
// and each must still be recoverable.

func TestTwoStatementsOnOneLineWithNoSemicolonAreRefused(t *testing.T) {
	_, typed := faults("let x = 5;\nlet y = x--1;\n")

	if len(typed) != 1 {
		t.Fatalf("expected 1 hard error, got %d: %v", len(typed), typed)
	}
	fault := typed[0]

	for _, want := range []string{"missing ';'", "`--`", "`1`", "two statements and not one"} {
		if !strings.Contains(fault.Msg, want) {
			t.Errorf("the error does not mention %q: %q", want, fault.Msg)
		}
	}
	// Written with nothing between them is the signal the author meant one
	// thing, and saying so is the difference between this message and a bare
	// complaint about a semicolon.
	if !strings.Contains(fault.Msg, "nothing between them") {
		t.Errorf("the error does not name the adjacency: %q", fault.Msg)
	}
	if got := fault.Range.Start.Line; got != 2 {
		t.Errorf("the error is on line %d, want 2", got)
	}
}

// The refusal does not depend on adjacency -- a space does not make two
// statements on one line any likelier to be what was meant. Only the extra
// sentence about adjacency depends on it.
func TestASpacedOmissionOnOneLineIsStillRefusedWithoutTheAdjacencyNote(t *testing.T) {
	_, typed := faults("let a = 1 let b = 2;\n")

	if len(typed) == 0 {
		t.Fatal("two statements on one line were accepted")
	}
	fault := typed[0]

	if !strings.Contains(fault.Msg, "missing ';'") {
		t.Errorf("the first error is not about the missing ';': %q", fault.Msg)
	}
	if strings.Contains(fault.Msg, "nothing between them") {
		t.Errorf("the error claims adjacency for a spaced omission: %q", fault.Msg)
	}
}

func TestAnOmissionAcrossALineBreakStaysRecoverable(t *testing.T) {
	program, p := parseSource(t, "let a = 1\nlet b = 2;\n")
	requireNoHardErrors(t, p)

	rec := p.Recoverables()
	if len(rec) != 1 || rec[0].Kind != MissingSemicolon {
		t.Fatalf("expected one missing-semicolon recoverable, got %#v", rec)
	}
	if len(program.Statements) != 2 {
		t.Fatalf("expected 2 statements, got %d", len(program.Statements))
	}
}

// The subtle half of statementFollowsOnSameLine: `}` follows a statement on the
// same line without beginning one, so these stay recoverable and the formatter
// still repairs them.
func TestAnOmissionBeforeAClosingBraceStaysRecoverable(t *testing.T) {
	sources := []string{
		"fn() { return 5 };",
		"for (;;) { break }",
		"for (;;) { continue }",
	}

	for _, src := range sources {
		t.Run(src, func(t *testing.T) {
			_, p := parseSource(t, src)
			requireNoHardErrors(t, p)

			rec := p.Recoverables()
			if len(rec) != 1 || rec[0].Kind != MissingSemicolon {
				t.Fatalf("expected one missing-semicolon recoverable, got %#v", rec)
			}
		})
	}
}

// And at end of input, where there is no next statement to be on the same line
// as. This is the case the formatter repairs most often.
func TestAnOmissionAtEndOfInputStaysRecoverable(t *testing.T) {
	_, p := parseSource(t, "let x = 5")
	requireNoHardErrors(t, p)

	if rec := p.Recoverables(); len(rec) != 1 || rec[0].Kind != MissingSemicolon {
		t.Fatalf("expected one missing-semicolon recoverable, got %#v", rec)
	}
}

// A `for` header is the one place a statement's terminator is written by the
// grammar rather than by the author, and its post clause is followed by `)` on
// the same line. Nothing here may be refused.
func TestForLoopsAreUntouched(t *testing.T) {
	sources := []string{
		"for (let i = 0; i < 3; i++) { putln(i); }",
		"for (;;) { break; }",
		"let i = 0; for (i = 0; i < 3; i++) { putln(i); }",
	}

	for _, src := range sources {
		t.Run(src, func(t *testing.T) {
			_, p := parseSource(t, src)
			requireNoHardErrors(t, p)
		})
	}
}
