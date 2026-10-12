package vm

import "testing"

// M26-EVL-002. A function renders as its name and parameters, which two
// distinct functions share, and assert_eq compared everything that is not a
// scalar by its render: assert_eq passed for two different functions, and
// assert_ne failed for them, alone or inside a container.
func TestEVL002AssertEqTellsFunctionsApart(t *testing.T) {
	machine := runTestLedger(t, `let f = fn(x) { x };
let mk = fn() { fn(y) { y } };
struct P { g; };
test("distinct", fn() { assert_eq(fn(x) { x }, fn(x) { x * 2 }); });
test("made twice", fn() { assert_eq(mk(), mk()); });
test("ne distinct", fn() { assert_ne(fn(x) { x }, fn(x) { x * 2 }); });
test("in array", fn() { assert_eq([mk()], [fn(y) { 0 }]); });
test("in hash", fn() { assert_eq({"k": mk()}, {"k": mk()}); });
test("in struct", fn() { assert_eq(P { g: mk() }, P { g: mk() }); });
test("same", fn() { assert_eq(f, f); });
test("same in array", fn() { assert_eq([f], [f]); });
test("same in struct", fn() { assert_eq(P { g: f }, P { g: f }); });
test("contains", fn() { assert_contains([f], f); });
test("contains distinct", fn() { assert_contains([fn(x) { x }], fn(x) { x * 2 }); });`)
	assertLedger(t, machine,
		"distinct:FAIL", "made twice:FAIL", "ne distinct:ok",
		"in array:FAIL", "in hash:FAIL", "in struct:FAIL",
		"same:ok", "same in array:ok", "same in struct:ok", "contains:ok", "contains distinct:FAIL")
}

// assert_eq compared two builtins by their render, which is the same for
// every builtin, so assert_eq(len, putln) passed.
func TestEVL002AssertEqTellsBuiltinsApart(t *testing.T) {
	machine := runTestLedger(t, `test("distinct", fn() { assert_eq(len, putln); });
test("ne distinct", fn() { assert_ne(len, putln); });
test("same", fn() { assert_eq(len, len); });
test("in array", fn() { assert_eq([len], [putln]); });
test("contains distinct", fn() { assert_contains([len], putln); });`)
	assertLedger(t, machine, "distinct:FAIL", "ne distinct:ok", "same:ok", "in array:FAIL", "contains distinct:FAIL")
}
