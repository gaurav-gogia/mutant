package parity

import (
	"fmt"
	"testing"
)

// TestLoopBodyEndingInErrorValueRunsEveryIteration pins M26-EVL-008. A loop
// body whose last expression is an error VALUE -- error("e") on its own line,
// or an assignment that stores one, since an assignment yields what it stored
// -- is an ordinary iteration. The evaluator's three loops tested the result's
// type for ERROR_OBJ, which an error value shares with a fault, so each of them
// stopped after the first iteration where the VM ran all three. The evaluator
// is what runs a macro body, so a macro using such a loop spliced 1 where the
// same code written inline gives 3.
//
// Both engines are asserted against the count, not only against each other.
func TestLoopBodyEndingInErrorValueRunsEveryIteration(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"for-in, error() last", `let n = 0; for (x in [1, 2, 3]) { n = n + 1; error("e") }; n`},
		{"for-in, stored error last", `let n = 0; let errs = [0, 0, 0]; for (i, p in ["a", "b", "c"]) { n = n + 1; errs[i] = error(p); }; n`},
		{"while, error() last", `let n = 0; while (n < 3) { n = n + 1; error("e") }; n`},
		{"while, stored error last", `let n = 0; let errs = [0, 0, 0, 0]; while (n < 3) { n = n + 1; errs[n] = error("e"); }; n`},
		{"c-style for, error() last", `let n = 0; for (let i = 0; i < 3; i = i + 1) { n = n + 1; error("e") }; n`},
		{"c-style for, stored error last", `let n = 0; let errs = [0, 0, 0]; for (let i = 0; i < 3; i = i + 1) { n = n + 1; errs[i] = error("e"); }; n`},
		{"for-in in a function, error() last", `let f = fn() { let n = 0; for (x in [1, 2, 3]) { n = n + 1; error("e") }; return n; }; f()`},
	}

	const want = "INTEGER(3)"
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			evaluated := normalize(evalViaEvaluator(tc.input))

			vmObj, vmErr := evalViaVM(t, tc.input)
			if vmErr != nil {
				t.Fatalf("VM refused %q: %v (evaluator gave %s)", tc.input, vmErr, evaluated)
			}
			compiled := normalize(vmObj)

			if evaluated != want {
				t.Errorf("evaluator answered %s for %q, want %s (VM %s)", evaluated, tc.input, want, compiled)
			}
			if compiled != want {
				t.Errorf("VM answered %s for %q, want %s", compiled, tc.input, want)
			}
		})
	}
}

// TestLoopBodyEndingInErrorValueInsideMacro is the row's user-visible shape: a
// macro body is run by the evaluator, and what it computes is spliced into the
// program the VM then runs. The loop in the macro must count what the same loop
// counts inline.
func TestLoopBodyEndingInErrorValueInsideMacro(t *testing.T) {
	loops := []struct {
		name string
		loop string
	}{
		{"for-in", `for (x in [1, 2, 3]) { n = n + 1; error("e") }`},
		{"while", `while (n < 3) { n = n + 1; error("e") }`},
		{"c-style for", `for (let i = 0; i < 3; i = i + 1) { n = n + 1; error("e") }`},
	}

	const want = "INTEGER(3)"
	for _, lp := range loops {
		t.Run(lp.name, func(t *testing.T) {
			inline := fmt.Sprintf("let n = 0; %s; n", lp.loop)
			direct, directErr := evalViaVM(t, inline)
			if directErr != nil {
				t.Fatalf("inline %q failed: %v", inline, directErr)
			}
			if got := normalize(direct); got != want {
				t.Fatalf("inline %q answered %s, want %s", inline, got, want)
			}

			viaMacro := fmt.Sprintf("let m = macro() { let n = 0; %s; quote(unquote(n)); }; m();", lp.loop)
			spliced, macroErr := evalViaVMWithMacros(t, viaMacro)
			if macroErr != nil {
				t.Fatalf("macro %q failed: %v", viaMacro, macroErr)
			}
			if got := normalize(spliced); got != want {
				t.Errorf("macro %q spliced %s, inline gives %s", viaMacro, got, want)
			}
		})
	}
}
