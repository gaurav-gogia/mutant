package parity

import "testing"

// M26-BLT-007, in both engines. keys, values and entries list a hash in the
// order a for-in loop walks it and printing shows it. They sorted by rendered
// key on their own, so integer keys came out as text ([10, 2]) and keys that
// render alike -- 1 and "1", true and "true" -- in whatever order the Go map
// was ranged in. The program spells each key and value with its type, because
// `==` on two arrays compares their printed forms and cannot tell 1 from "1".
func TestKeysValuesEntriesFollowTheForInOrderInBothEngines(t *testing.T) {
	const program = `
let h = {"1": "str", 1: "int", 10: "ten", 2: "two", true: "bool", "true": "s-true"};
let sig = fn(x) { return type_of(x) + ":" + to_string(x); };
let walked = "";
for (k, v in h) { walked = walked + sig(k) + "=" + sig(v) + ";"; }
let ks = keys(h);
let vs = values(h);
let es = entries(h);
let listed = "";
let paired = "";
let i = 0;
while (i < len(ks)) {
    listed = listed + sig(ks[i]) + "=" + sig(vs[i]) + ";";
    paired = paired + sig(es[i][0]) + "=" + sig(es[i][1]) + ";";
    i = i + 1;
}
[walked == listed, walked == paired, listed, to_string(h)]`

	const want = "ARRAY([true, true, " +
		"BOOLEAN:true=STRING:bool;INTEGER:1=STRING:int;INTEGER:2=STRING:two;INTEGER:10=STRING:ten;STRING:1=STRING:str;STRING:true=STRING:s-true;, " +
		"{true: bool, 1: int, 2: two, 10: ten, 1: str, true: s-true}])"

	evalRes := normalize(evalViaEvaluator(program))
	vmObj, vmErr := evalViaVM(t, program)
	if vmErr != nil {
		t.Fatalf("the VM failed: %v", vmErr)
	}
	vmRes := normalize(vmObj)
	if evalRes != want {
		t.Errorf("the evaluator answered\n %s\nwant\n %s", evalRes, want)
	}
	if vmRes != want {
		t.Errorf("the compiled engine answered\n %s\nwant\n %s", vmRes, want)
	}
}
