package parity

import "testing"

// M26-EVL-006. Float.Inspect keeps six decimals, and both engines compared two
// arrays or two hashes by their renders, so [7.9999991] == [7.9999994] was true
// in both. unique, contains and the order of float hash keys leaned on the same
// renders. Each case must give one answer, and the right one, in both engines.
func TestEVL006FloatsThatRenderAlikeStayDistinctInBothEngines(t *testing.T) {
	const ab = "let a = 7.9999991; let b = 7.9999994;\n"
	for _, c := range []struct{ name, input, want string }{
		{"the scalars differ", ab + "a == b", "BOOLEAN(false)"},
		{"so do arrays holding them", ab + "[a] == [b]", "BOOLEAN(false)"},
		{"and != agrees", ab + "[a] != [b]", "BOOLEAN(true)"},
		{"an array equals one with the same float", ab + "[a] == [7.9999991]", "BOOLEAN(true)"},
		{"hashes holding them differ", ab + "{1: a} == {1: b}", "BOOLEAN(false)"},
		{"float keys that render alike are two keys", "{0.0000001: 1} == {0.0000002: 1}", "BOOLEAN(false)"},
		{"hashes built in two orders are equal", `{"a": 1, "b": 2.5} == {"b": 2.5, "a": 1}`, "BOOLEAN(true)"},
		{"unique keeps both", ab + "len(unique([a, b]))", "INTEGER(2)"},
		{"unique keeps both arrays", ab + "len(unique([[a], [b]]))", "INTEGER(2)"},
		{"contains compares nested floats by value", ab + "contains([[a]], [b])", "BOOLEAN(false)"},
		{"a string element is not an integer element", `["1"] == [1]`, "BOOLEAN(false)"},
		{
			"float keys iterate in numeric order",
			`let h = {10.0: "ten", 9.0: "nine", 0.0000003: "c", 0.0000001: "a", 0.0000002: "b"};
let order = "";
for (k, v in h) { order = order + v; }
order`,
			`STRING("abcnineten")`,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			evalRes := normalize(evalViaEvaluator(c.input))
			vmObj, vmErr := evalViaVM(t, c.input)
			if vmErr != nil {
				t.Fatalf("the VM failed: %v", vmErr)
			}
			if vmRes := normalize(vmObj); vmRes != c.want {
				t.Errorf("the compiled engine answered %s, want %s", vmRes, c.want)
			}
			if evalRes != c.want {
				t.Errorf("the evaluator answered %s, want %s", evalRes, c.want)
			}
		})
	}
}
