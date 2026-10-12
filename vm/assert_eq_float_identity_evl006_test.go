package vm

import "testing"

// M26-EVL-006. assert_eq compared arrays and hashes by their renders, and
// Float.Inspect keeps six decimals, so two different floats inside a container
// passed as equal.
func TestEVL006AssertEqTellsNestedFloatsApart(t *testing.T) {
	machine := runTestLedger(t, `test("array", fn() { assert_eq([7.9999991], [7.9999994]); });
test("hash", fn() { assert_eq({1: 7.9999991}, {1: 7.9999994}); });
test("same", fn() { assert_eq([7.9999991], [7.9999991]); });`)
	assertLedger(t, machine, "array:FAIL", "hash:FAIL", "same:ok")
}

// M26-EVL-006, the struct case. A bare struct fell through to the rendered-form
// comparison, so assert_eq passed on two structs whose float fields differed
// past the sixth decimal while == called them different, and assert_eq on the
// same structs wrapped in arrays already failed.
func TestEVL006AssertionsTellStructsWithNestedFloatsApart(t *testing.T) {
	machine := runTestLedger(t, `struct EVL006P { a }
let x = EVL006P{a: 7.9999991};
let y = EVL006P{a: 7.9999994};
test("eq", fn() { assert_eq(x, y); });
test("ne", fn() { assert_ne(x, y); });
test("contains", fn() { assert_contains([x], y); });
test("strings", fn() { assert_eq(EVL006P{a: ["1"]}, EVL006P{a: [1]}); });
test("same", fn() { assert_eq(x, EVL006P{a: 7.9999991}); });`)
	assertLedger(t, machine, "eq:FAIL", "ne:ok", "contains:FAIL", "strings:FAIL", "same:ok")
}
