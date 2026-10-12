package parity

import (
	"fmt"
	"testing"
)

// A container is a value, not a shared object: binding it to a second name,
// passing it to a function, storing it into another container or iterating over
// it hands over a copy, and a write through one name is never seen through
// another. The VM has always behaved this way -- every variable store seals the
// value and every load opens a fresh copy of it (mutil.DecryptObject) -- and
// that is the runtime every program runs on.
//
// The evaluator shared the container instead, so `let b = a; b[0] = 9;` changed
// `a` there and left it alone on the VM (M26-VM-019). The evaluator is what
// runs a macro body, so a macro that mutated through an alias spliced a
// different value into the program than the same lines give when they run.
//
// Every row names the answer the VM gives, so a regression in either engine
// fails here, not only a divergence between them.
func TestAliasedContainerMutationParity(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		// The two rows the finding measured.
		{"let alias", "let a = [1, 2]; let b = a; b[0] = 9; a[0]", "INTEGER(1)"},
		{"element read out of a hash", `let h = {"k": [1]}; let x = h["k"]; x[0] = 5; h["k"][0]`, "INTEGER(1)"},
		// The same rule at function scope, where the VM keeps locals in frame
		// slots rather than globals.
		{"let alias in a function", "let probe = fn() { let a = [1, 2]; let b = a; b[0] = 9; return a[0]; }; probe()", "INTEGER(1)"},
		{"plain reassignment", "let a = [1]; let b = [0]; b = a; b[0] = 9; a[0]", "INTEGER(1)"},
		{"argument", "let f = fn(x) { x[0] = 9; return 0; }; let a = [1]; f(a); a[0]", "INTEGER(1)"},
		{"callback element", "let a = [[1]]; each(a, fn(x) { x[0] = 9; }); a[0][0]", "INTEGER(1)"},
		{"returned capture", "let a = [1]; let get = fn() { return a; }; let b = get(); b[0] = 9; a[0]", "INTEGER(1)"},
		{"two return values", "let a = [1]; let pair = fn() { return a, a; }; let x, y = pair(); x[0] = 9; y[0]", "INTEGER(1)"},
		{"stored by index", "let a = [1]; let g = [0]; g[0] = a; a[0] = 9; g[0][0]", "INTEGER(1)"},
		{"array literal element", "let a = [1]; let g = [a]; a[0] = 9; g[0][0]", "INTEGER(1)"},
		{"hash literal value", `let a = [1]; let h = {"k": a}; h["k"][0] = 9; a[0]`, "INTEGER(1)"},
		{"hash alias", `let h = {"k": 1}; let g = h; g["k"] = 2; h["k"]`, "INTEGER(1)"},
		{"struct alias", "struct P { x }; let p = P{x: 1}; let q = p; q.x = 5; p.x", "INTEGER(1)"},
		{"struct field written through", "struct P { xs }; let p = P{xs: [1]}; let q = p; q.xs[0] = 9; p.xs[0]", "INTEGER(1)"},
		{"stored by field", "struct P { xs }; let a = [1]; let p = P{xs: [0]}; p.xs = a; a[0] = 9; p.xs[0]", "INTEGER(1)"},
		{"buffer alias", `let b, e = string_to_bytes("abc", "raw"); let c = b; c[0] = 122; b[0]`, "INTEGER(97)"},
		// A loop walks the collection as it was when the loop began, and its
		// binding is a copy of the element.
		{"written while iterated", "let a = [1, 2, 3]; let s = 0; for (x in a) { a[2] = 100; s = s + x; } s", "INTEGER(6)"},
		{"loop binding", "let a = [[1]]; for (x in a) { x[0] = 9; } a[0][0]", "INTEGER(1)"},
		// A container stored into itself stores what it held, not a cycle.
		// It is asked through type_of rather than ==, because on the pre-fix
		// evaluator the cycle sends any rendering of it into unbounded
		// recursion.
		{"stored into itself", "let a = [1, 2]; a[0] = a; type_of(a[0][0])", `STRING("INTEGER")`},
		// Writing through a name, at any depth, still lands: the copy is made
		// where a value is read, never where a target is written.
		{"write through a name", "let a = [1, 2]; a[0] = 9; a[0]", "INTEGER(9)"},
		{"nested write through a name", "let grid = [[1, 2], [3, 4]]; grid[0][1] = 9; grid[0][1]", "INTEGER(9)"},
		{"closure writes its capture", "let a = [1]; let f = fn() { a[0] = 9; return 0; }; f(); a[0]", "INTEGER(9)"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			evalRes := normalize(evalViaEvaluator(c.input))
			vmObj, vmErr := evalViaVM(t, c.input)
			vmRes := normalize(vmObj)
			if vmErr != nil {
				vmRes = "ERROR"
			}
			if evalRes != c.want || vmRes != c.want {
				t.Errorf("%q: want %s, evaluator=%s vm=%s", c.input, c.want, evalRes, vmRes)
			}
		})
	}
}

// TestAliasedContainerMutationInMacroBody is where the divergence reached a
// program: the macro body runs on the evaluator at expansion time, and the
// value it splices must be the one the same lines give when the VM runs them.
func TestAliasedContainerMutationInMacroBody(t *testing.T) {
	bodies := []string{
		"let a = [1, 2]; let b = a; b[0] = 9; a[0]",
		`let h = {"k": [1]}; let x = h["k"]; x[0] = 5; h["k"][0]`,
	}
	for _, body := range bodies {
		inline, inlineErr := evalViaVM(t, body)
		if inlineErr != nil {
			t.Fatalf("running %q inline failed: %s", body, inlineErr)
		}
		// The macro body ends in the expression whose value is spliced.
		macro := fmt.Sprintf("let m = macro() { %s; quote(unquote(%s)); }; m();",
			body[:lastSemicolon(body)], body[lastSemicolon(body)+2:])
		viaMacro, macroErr := evalViaVMWithMacros(t, macro)
		if macroErr != nil {
			t.Fatalf("running %q through a macro failed: %s", macro, macroErr)
		}
		if normalize(inline) != normalize(viaMacro) {
			t.Errorf("macro body splices %s where the same lines run inline give %s: %s",
				normalize(viaMacro), normalize(inline), body)
		}
	}
}

func lastSemicolon(s string) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == ';' {
			return i
		}
	}
	return -1
}
