package parity

import "testing"

// A function value is equal to itself, to every name it is bound under and to
// every copy of it an engine makes, and to nothing else -- in both engines.
//
// The VM rebuilds a closure every time it stores or loads one, and `==` used to
// compare rendered forms, and a closure renders its own address: so on the VM a
// function was never equal to itself, `unique([f, f])` kept both and
// `contains([f], f)` was false. The evaluator rendered a function as its source
// text instead, so it answered the opposite and called two distinct functions
// with the same text equal (M26-EVL-005).
//
// Two literals are two function values, even when their text is the same, and
// each call to a function that returns a closure makes a new one: identity is
// where the value was made, never what it looks like.
func TestFunctionIdentityParityEVL005(t *testing.T) {
	const f = "let f = fn(x) { x }; "
	const mk = "let mk = fn() { fn(y) { y } }; "

	for _, tt := range []struct {
		input string
		want  string
	}{
		{f + "f == f;", "BOOLEAN(true)"},
		{f + "f != f;", "BOOLEAN(false)"},
		{f + "let g = f; g == f;", "BOOLEAN(true)"},
		{f + "let g = f; f != g;", "BOOLEAN(false)"},
		{f + "let id = fn(v) { v }; id(f) == f;", "BOOLEAN(true)"},
		{f + "let a = [f]; a[0] == f;", "BOOLEAN(true)"},
		{f + `let h = {"k": f}; h["k"] == f;`, "BOOLEAN(true)"},
		{"let n = 0; let inc = fn() { n = n + 1; n }; let h = inc; inc(); h == inc;", "BOOLEAN(true)"},
		{"let fact = fn(n) { if (n < 2) { 1 } else { n * fact(n - 1) } }; fact == fact;", "BOOLEAN(true)"},
		{"fn(x) { x } == fn(x) { x };", "BOOLEAN(false)"},
		{"fn(x) { x } != fn(x) { x };", "BOOLEAN(true)"},
		{mk + "mk() == mk();", "BOOLEAN(false)"},
		{mk + "let a = mk(); let b = mk(); a == b;", "BOOLEAN(false)"},
		{f + "f == 1;", "BOOLEAN(false)"},
		{f + `f == "fn(x) { x }";`, "BOOLEAN(false)"},
		{f + "len(unique([f, f, f]));", "INTEGER(1)"},
		{f + mk + "len(unique([f, mk(), f, mk()]));", "INTEGER(3)"},
		{f + "contains([f], f);", "BOOLEAN(true)"},
		{f + mk + "contains([mk()], mk());", "BOOLEAN(false)"},
		{f + "index_of([1, f], f);", "INTEGER(1)"},
	} {
		viaVM, err := evalViaVM(t, tt.input)
		if err != nil {
			t.Errorf("%s: the VM declined to answer: %s", tt.input, err)
			continue
		}
		if got := normalize(viaVM); got != tt.want {
			t.Errorf("VM: %s = %s, want %s", tt.input, got, tt.want)
		}
		if got := normalize(evalViaEvaluator(tt.input)); got != tt.want {
			t.Errorf("evaluator: %s = %s, want %s", tt.input, got, tt.want)
		}
	}
}

// A function inside an array, a hash or a struct inside one is compared by the
// same identity rule, in `==`, `contains`, `index_of` and `unique` alike. These
// used to compare the composite's rendered form, which carries the function's
// render with it: so on the VM `[f] == [f]` was false, and in the evaluator
// `[f] == [lit]` was true for two literals with one source text (M26-EVL-005).
func TestFunctionInsideCompositeParityEVL005(t *testing.T) {
	const f = "let f = fn(x) { x }; let lit = fn(x) { x }; "
	const p = "struct P { h; }; "

	for _, tt := range []struct {
		input string
		want  string
	}{
		{f + "[f] == [f];", "BOOLEAN(true)"},
		{f + "[f] != [f];", "BOOLEAN(false)"},
		{f + "[f] == [lit];", "BOOLEAN(false)"},
		{f + "[f] != [lit];", "BOOLEAN(true)"},
		{f + "let a = [f]; let b = a; a == b;", "BOOLEAN(true)"},
		{f + "[1, f] == [1, f];", "BOOLEAN(true)"},
		{f + "[1, f] == [2, f];", "BOOLEAN(false)"},
		{f + "[f, 1] == [f];", "BOOLEAN(false)"},
		{f + "[[f]] == [[f]];", "BOOLEAN(true)"},
		{f + "[[f]] == [[lit]];", "BOOLEAN(false)"},
		{f + `{"k": f} == {"k": f};`, "BOOLEAN(true)"},
		{f + `{"k": f} == {"k": lit};`, "BOOLEAN(false)"},
		{f + `{"k": f} == {"j": f};`, "BOOLEAN(false)"},
		{f + `{"a": 1, "k": f} == {"k": f, "a": 1};`, "BOOLEAN(true)"},
		{f + `{"a": [f]} == {"a": [f]};`, "BOOLEAN(true)"},
		{f + p + "[P { h: f }] == [P { h: f }];", "BOOLEAN(true)"},
		{f + p + "[P { h: f }] == [P { h: lit }];", "BOOLEAN(false)"},
		{f + p + "P { h: [f] } == P { h: [f] };", "BOOLEAN(true)"},
		{f + p + "P { h: [f] } == P { h: [lit] };", "BOOLEAN(false)"},
		{f + `[f] == "[f]";`, "BOOLEAN(false)"},
		{f + "len(unique([[f], [f], [lit]]));", "INTEGER(2)"},
		{f + `len(unique([{"k": f}, {"k": f}, {"k": lit}, [1], [1]]));`, "INTEGER(3)"},
		{f + "contains([[f]], [f]);", "BOOLEAN(true)"},
		{f + "contains([[lit]], [f]);", "BOOLEAN(false)"},
		{f + `index_of([{"k": f}], {"k": f});`, "INTEGER(0)"},
		{f + "index_of([[lit], [f]], [f]);", "INTEGER(1)"},
	} {
		viaVM, err := evalViaVM(t, tt.input)
		if err != nil {
			t.Errorf("%s: the VM declined to answer: %s", tt.input, err)
			continue
		}
		if got := normalize(viaVM); got != tt.want {
			t.Errorf("VM: %s = %s, want %s", tt.input, got, tt.want)
		}
		if got := normalize(evalViaEvaluator(tt.input)); got != tt.want {
			t.Errorf("evaluator: %s = %s, want %s", tt.input, got, tt.want)
		}
	}
}
