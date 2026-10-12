package parity

import (
	"testing"

	"mutant/compiler"
	"mutant/lexer"
	"mutant/parser"
)

// M26-LSP-035, in both engines. An assignment to a name nothing declares used
// to be an implicit let in each of them -- the compiler defined the name, the
// evaluator Set it -- so `y = 1` ran while the editor called y undefined. Both
// now refuse it, and both still store into a name that is declared.
func TestUndeclaredAssignmentRefusedInBothEnginesLSP035(t *testing.T) {
	refused := []string{
		"y = 1;\ny",
		"let f = fn() { y = 5; return y; };\nf()",
		"if (true) { y = 1; }\n0",
	}
	for _, src := range refused {
		if err := compiler.New().Compile(parser.New(lexer.New(src)).ParseProgram()); err == nil {
			t.Errorf("the compiler accepted an assignment to a name nothing declares:\n%s", src)
		}
		if got := normalize(evalViaEvaluator(src)); got != "ERROR" {
			t.Errorf("the evaluator answered %s for an assignment to a name nothing declares, want ERROR:\n%s", got, src)
		}
	}

	accepted := []struct {
		src  string
		want string
	}{
		{"let y = 1;\ny = 2;\ny", "INTEGER(2)"},
		{"let y = 1;\nlet f = fn() { y = 7; return 0; };\nf();\ny", "INTEGER(7)"},
		{"let f = fn() { let y = 1; y = 3; return y; };\nf()", "INTEGER(3)"},
		{"let y = 1;\nif (true) { y = 4; }\ny", "INTEGER(4)"},
	}
	for _, tc := range accepted {
		vmResult, err := evalViaVM(t, tc.src)
		if err != nil {
			t.Errorf("the VM failed on an assignment to a declared name: %v\n%s", err, tc.src)
			continue
		}
		if got := normalize(vmResult); got != tc.want {
			t.Errorf("the VM answered %s, want %s:\n%s", got, tc.want, tc.src)
		}
		if got := normalize(evalViaEvaluator(tc.src)); got != tc.want {
			t.Errorf("the evaluator answered %s, want %s:\n%s", got, tc.want, tc.src)
		}
	}
}
