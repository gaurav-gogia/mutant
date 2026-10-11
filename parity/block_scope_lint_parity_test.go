package parity

import (
	"strings"
	"testing"

	"mutant/lsp/api"
	"mutant/object"
)

// M26-SEM-002 said the editor's inference scoped if, while and for-in bodies
// while the VM did not, so the argument-type warning fired on programs that ran.
// Since cf79eb4 ("a block is a scope, and a let declares") the compiler scopes
// every block and every loop header, and inference already did: after each
// block below, x is the outer INTEGER again, and the VM's str_upper refuses it
// with the sentence the editor shows. The warning is a true positive now; these
// tests pin that the two keep saying the same thing, in both directions.
func TestTheArgTypeWarningAgreesWithTheVMAfterABlock(t *testing.T) {
	const want = "argument 1 to `str_upper` must be STRING, got INTEGER"
	for _, c := range []struct{ name, src string }{
		{"an if body", "let x = 5;\nif (true) { let x = \"hello\"; }\nstr_upper(x);\n"},
		{"a for-in header", "let x = 7;\nfor (x in [\"a\", \"b\"]) { }\nstr_upper(x);\n"},
		{"a while body", "let x = 1;\nlet n = 0;\nwhile (n < 1) { let x = \"inner\"; n = n + 1; }\nstr_upper(x);\n"},
		{"an if body in a function", "let f = fn() { let x = 3; if (true) { let x = \"s\"; } return str_upper(x); };\nf();\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			warned := false
			for _, d := range api.Lint(c.src) {
				if d.Source == "mutant-lint" && strings.Contains(d.Message, want) {
					warned = true
				}
			}
			result, err := evalViaVM(t, c.src)
			if err != nil {
				t.Fatalf("the VM stopped: %v", err)
			}
			refused := false
			if e, ok := result.(*object.Error); ok && strings.Contains(e.Message, want) {
				refused = true
			}
			if warned != refused {
				t.Fatalf("editor warns: %v; VM refuses: %v (result %s)\n\n%s",
					warned, refused, result.Inspect(), c.src)
			}
			if !refused {
				t.Fatalf("the VM no longer refuses, so the block scoping this pins has changed (result %s)",
					result.Inspect())
			}
		})
	}
}

// TestAnInnerShadowDoesNotLeakPastItsBlock is the mirror image, and the reason
// M26-SEM-002's own proposal -- no scope for a block, for-in names bound in the
// enclosing scope -- must not be applied: here the INNER binding is the integer
// and the outer one the string, and the program runs. Flat inference would
// carry the inner INTEGER past the block and draw exactly the false warning the
// row set out to remove.
func TestAnInnerShadowDoesNotLeakPastItsBlock(t *testing.T) {
	const warning = "must be STRING"
	for _, c := range []struct{ name, src string }{
		{"an if body", "let x = \"s\";\nif (true) { let x = 5; }\nstr_upper(x);\n"},
		{"a for-in header", "let x = \"s\";\nfor (x in [1, 2]) { }\nstr_upper(x);\n"},
		{"a while body", "let x = \"s\";\nlet n = 0;\nwhile (n < 1) { let x = 1; n = n + 1; }\nstr_upper(x);\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			for _, d := range api.Lint(c.src) {
				if d.Source == "mutant-lint" && strings.Contains(d.Message, warning) {
					t.Errorf("the editor warns %q on a program that runs\n\n%s", d.Message, c.src)
				}
			}
			result, err := evalViaVM(t, c.src)
			if err != nil {
				t.Fatalf("the VM stopped: %v", err)
			}
			if got := normalize(result); got != `STRING("S")` {
				t.Fatalf("the VM answered %s, want STRING(\"S\")", got)
			}
		})
	}
}
