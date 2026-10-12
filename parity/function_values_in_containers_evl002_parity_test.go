package parity

import "testing"

// A function inside an array, a hash or a struct is compared as a function is
// anywhere else -- by identity -- in both engines, and a value holding one
// function, an error included, is never equal to a value holding another.
//
// A VM function used to render its heap address, and arrays, hashes and the
// unique builtin compared values by their renders, so a container holding a
// function was never equal to anything, not even to itself. Once a function
// renders as its name and parameters (M26-EVL-002), those renders are shared by
// distinct functions, and a string can spell one. Comparing by render would
// then call [fn(x) { x }] equal to [fn(x) { x * 2 }], and ["fn(y)"] equal to an
// array holding the function fn(y). The evaluator has always rendered a
// function as its source text, so there [mk()] was equal to [mk()].
//
// No case asks whether two errors holding the same function are equal: the VM
// gob-encodes an error when it stores one, and a closure in its related values
// comes back without its identity. That is a separate defect.
func TestFunctionValuesInContainersParityEVL002(t *testing.T) {
	const f = "let f = fn(x) { x }; "
	const mk = "let mk = fn() { fn(y) { y } }; "
	const p = "struct P { g; }; "

	for _, tt := range []struct {
		input string
		want  string
	}{
		{f + "[f] == [f];", "BOOLEAN(true)"},
		{f + "[f] != [f];", "BOOLEAN(false)"},
		{"[fn(x) { x }] == [fn(x) { x * 2 }];", "BOOLEAN(false)"},
		{"[fn(x) { x }] != [fn(x) { x * 2 }];", "BOOLEAN(true)"},
		{mk + "[mk()] == [mk()];", "BOOLEAN(false)"},
		{mk + `["fn(y)"] == [mk()];`, "BOOLEAN(false)"},
		{mk + `["fn(y) { y }"] == [mk()];`, "BOOLEAN(false)"},
		{f + `{"k": f} == {"k": f};`, "BOOLEAN(true)"},
		{mk + `{"k": mk()} == {"k": fn(y) { y * 3 }};`, "BOOLEAN(false)"},
		{f + p + "P { g: f } == P { g: f };", "BOOLEAN(true)"},
		{mk + p + "P { g: mk() } == P { g: mk() };", "BOOLEAN(false)"},
		{f + "len(unique([[f], [f]]));", "INTEGER(1)"},
		{mk + "len(unique([[mk()], [mk()]]));", "INTEGER(2)"},
		{mk + `len(unique([{"k": mk()}, {"k": mk()}]));`, "INTEGER(2)"},
		{mk + p + "len(unique([P { g: mk() }, P { g: mk() }]));", "INTEGER(2)"},
		{f + p + "len(unique([P { g: f }, P { g: f }]));", "INTEGER(1)"},
		{mk + `len(unique([["fn(y)"], [mk()]]));`, "INTEGER(2)"},
		{f + "contains([[f]], [f]);", "BOOLEAN(true)"},
		{mk + "contains([[mk()]], [mk()]);", "BOOLEAN(false)"},
		{mk + `index_of([["fn(y)"]], [mk()]);`, "INTEGER(-1)"},
		{mk + `error("m", "", {"f": mk()}) == error("m", "", {"f": mk()});`, "BOOLEAN(false)"},
		{mk + `let e = fn() { error("m", "", {"f": mk()}) }; len(unique([e(), e()]));`, "INTEGER(2)"},
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
