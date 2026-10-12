package parity

import "testing"

// A builtin is a function value like any other: equal to itself and to nothing
// else, in `==`, `contains`, `index_of` and `unique`, alone or inside a
// composite -- in both engines.
//
// Every builtin renders as the same constant, "builtin function", so a
// comparison by render called any two builtins equal. Once a composite holding
// a closure was walked element by element (M26-EVL-005), the builtins beside the
// closure were still compared by that render, and on the VM
// `[f, len] == [f, putln]` was true where it had been false.
func TestBuiltinIdentityParityEVL005(t *testing.T) {
	const f = "let f = fn(x) { x }; "

	for _, tt := range []struct {
		input string
		want  string
	}{
		{f + "[f, len] == [f, putln];", "BOOLEAN(false)"},
		{f + "[f, len] != [f, putln];", "BOOLEAN(true)"},
		{f + "[f, len] == [f, len];", "BOOLEAN(true)"},
		{f + `{"a": f, "b": len} == {"a": f, "b": putln};`, "BOOLEAN(false)"},
		{"len == putln;", "BOOLEAN(false)"},
		{"len != putln;", "BOOLEAN(true)"},
		{"len == len;", "BOOLEAN(true)"},
		{"let g = len; g == len;", "BOOLEAN(true)"},
		{"[len] == [putln];", "BOOLEAN(false)"},
		{"[len] == [len];", "BOOLEAN(true)"},
		{"len(unique([len, putln, len]));", "INTEGER(2)"},
		{"contains([len], putln);", "BOOLEAN(false)"},
		{"contains([len], len);", "BOOLEAN(true)"},
		{"index_of([putln, len], len);", "INTEGER(1)"},
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

// An error whose related values hold a function is a duplicate in `unique` only
// of an error whose related values hold the same function. unique used to file
// such an error under its render, which spells a function by its signature, so
// two errors holding two distinct functions of one signature were one element
// on the VM (M26-EVL-002).
//
// Two errors holding the same closure are left out on purpose: the VM stores an
// error gob-encoded whole, gob drops the closure's identity, and on the VM such
// an error is not even equal to itself. That is its own defect, not this one.
func TestErrorHoldingFunctionUniqueParityEVL002(t *testing.T) {
	const mk = "let mk = fn() { fn(y) { y } }; "

	for _, tt := range []struct {
		input string
		want  string
	}{
		{mk + `let e = fn() { error("m", "", {"f": mk()}) }; len(unique([e(), e()]));`, "INTEGER(2)"},
		{mk + `let e = fn() { error("m", "", {"h": {"f": mk()}}) }; len(unique([e(), e()]));`, "INTEGER(2)"},
		{`let e = fn() { error("m", "", {"f": len}) }; len(unique([e(), e()]));`, "INTEGER(1)"},
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
