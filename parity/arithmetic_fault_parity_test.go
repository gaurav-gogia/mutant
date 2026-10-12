package parity

import (
	"strings"
	"testing"

	"mutant/evaluator"
	"mutant/lexer"
	"mutant/object"
	"mutant/parser"
)

// An arithmetic or bitwise failure stops the program in both engines. The
// evaluator used to hold one as a value -- the operator code the engines share
// answers a failure with a plain error, which is a value to the tree-walker --
// so `let x = 1 / 0; 5` gave 5 there while the VM stopped the run, and a macro
// body expanded from a computation the same code written inline is refused for
// (M26-EVL-007).
//
// TestEvaluatorVMOperatorParity cannot see this: its normalize renders a
// failure and an error value alike. Binding the failure and going on to a
// value is what tells them apart, because only a stopped program never reaches
// the 5.

var arithmeticFailures = []struct{ expr, want string }{
	{"1 / 0", "integer division by zero"},
	{"1 % 0", "integer modulo by zero"},
	{"1 << -1", "negative shift count: -1"},
	{"1.5 & 1", "bitwise operator & requires INTEGER operands, got FLOAT and INTEGER"},
	{`"a" | 1`, "bitwise operator | requires INTEGER operands, got STRING and INTEGER"},
	{"~2.5", "bitwise complement requires an INTEGER, got FLOAT"},
	{`~"a"`, "bitwise complement requires an INTEGER, got STRING"},
}

func TestAnArithmeticFailureStopsBothEngines(t *testing.T) {
	for _, tc := range arithmeticFailures {
		src := "let x = " + tc.expr + ";\n5;\n"
		t.Run(tc.expr, func(t *testing.T) {
			vmFailure := runFails(t, src)
			if !strings.Contains(vmFailure, tc.want) {
				t.Fatalf("the VM gave %q, want it to stop with %q", vmFailure, tc.want)
			}

			p := parser.New(lexer.New(src))
			program := p.ParseProgram()
			if len(p.Errors()) > 0 {
				t.Fatalf("does not parse: %v", p.Errors())
			}
			result := evaluator.Eval(program, object.NewEnvironment())
			failure, stopped := result.(*object.Error)
			if !stopped {
				t.Fatalf("the evaluator ran on to %s; the VM stopped with %q", result.Inspect(), vmFailure)
			}
			if failure.Message != tc.want {
				t.Fatalf("the evaluator stopped with %q, want the VM's %q", failure.Message, tc.want)
			}
		})
	}
}

// TestAMacroBodyThatFailsDoesNotExpand is the reachable half: the evaluator's
// production role is macro expansion, so a body that divides by zero has to
// stop the expansion exactly as the same statement written inline stops the
// run.
func TestAMacroBodyThatFailsDoesNotExpand(t *testing.T) {
	const inline = "let q = 10 / 0;\n3;\n"
	if failure := runFails(t, inline); !strings.Contains(failure, "integer division by zero") {
		t.Fatalf("written inline the statement does not stop the run (%q), so the macro case proves nothing", failure)
	}

	const viaMacro = "let m = macro() { let q = 10 / 0; quote(unquote(3)); };\nm();\n"
	if message, expanded := expandOnly(viaMacro); expanded {
		t.Fatal("the macro expanded although its body divides by zero")
	} else if !strings.Contains(message, "integer division by zero") {
		t.Fatalf("the expansion failed with %q, want the division by zero", message)
	}
}
