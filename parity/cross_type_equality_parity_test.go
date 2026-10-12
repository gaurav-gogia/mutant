package parity

import "testing"

// `==` and `!=` never convert between types (M26-VM-010). Both engines used to
// end their comparison in a fallback that compared rendered text, and null
// renders as "", so null was equal to the empty string, "0" was equal to 0,
// "true" was equal to true and [1] was equal to "[1]". The two engines agreed
// on every one of those wrong answers, so -- as in truthy_parity_test.go -- a
// test that only asks whether they match could not have caught it. Each engine
// is asked separately for the right answer, and agreeing is not enough.
//
// The one exception to "different types are never equal" is the numeric pair:
// an integer and a float compare by value. TestIntFloatEqualityStillNumeric
// keeps that from being swept away by the type guard.

// crossTypeEqualityCase is one comparison. prelude holds any statements the
// operands need; want is what `left == right` must answer, and `left != right`
// must answer the opposite.
type crossTypeEqualityCase struct {
	label   string
	prelude string
	left    string
	right   string
	want    bool
}

// askEqualityInBothEngines runs `left == right` and `left != right` on each
// engine and fails on any answer other than want and !want.
func askEqualityInBothEngines(t *testing.T, tc crossTypeEqualityCase) {
	t.Helper()
	for _, op := range []struct {
		symbol string
		want   bool
	}{{"==", tc.want}, {"!=", !tc.want}} {
		input := tc.prelude + " " + tc.left + " " + op.symbol + " " + tc.right + ";"
		want := "BOOLEAN(false)"
		if op.want {
			want = "BOOLEAN(true)"
		}

		if got := normalize(evalViaEvaluator(input)); got != want {
			t.Errorf("%s: evaluator: %s = %s, want %s", tc.label, input, got, want)
		}
		viaVM, err := evalViaVM(t, input)
		if err != nil {
			t.Errorf("%s: VM declined %s: %v", tc.label, input, err)
			continue
		}
		if got := normalize(viaVM); got != want {
			t.Errorf("%s: VM: %s = %s, want %s", tc.label, input, got, want)
		}
	}
}

func TestCrossTypeEqualityIsFalse(t *testing.T) {
	const nullFn = `let nf = fn() { return; };`
	for _, tc := range []crossTypeEqualityCase{
		// null against the empty string, reached the ways a program reaches null.
		{"null from a bare return vs empty string", nullFn, "nf()", `""`, false},
		{"empty string vs null from a bare return", nullFn, `""`, "nf()", false},
		{"null from a hash miss vs empty string", `let hm = {"a": 1};`, `hm["b"]`, `""`, false},
		{"null from a valueless call vs empty string", `let vf = fn() { let x = 1; };`, "vf()", `""`, false},

		// Strings spelling another type's rendering.
		{`"0" vs 0`, "", `"0"`, "0", false},
		{`0 vs "0"`, "", "0", `"0"`, false},
		{`"true" vs true`, "", `"true"`, "true", false},
		{`true vs "true"`, "", "true", `"true"`, false},
		{`"false" vs false`, "", `"false"`, "false", false},
		{`[1] vs "[1]"`, "", "[1]", `"[1]"`, false},
		{`"[1]" vs [1]`, "", `"[1]"`, "[1]", false},
		{`[] vs "[]"`, "", "[]", `"[]"`, false},

		// Through variables, so the VM's operands come back from the encrypted
		// store rather than straight off the stack.
		{"string and integer held in variables", `let s = "0"; let i = 0;`, "s", "i", false},
		{"string and null held in variables", nullFn + ` let e = ""; let n = nf();`, "e", "n", false},
	} {
		askEqualityInBothEngines(t, tc)
	}
}

// Within one type nothing changed: the guard decides only between types.
func TestSameTypeEqualityUnchangedByTypeGuard(t *testing.T) {
	const nullFn = `let nf = fn() { return; };`
	for _, tc := range []crossTypeEqualityCase{
		{"equal strings", "", `"a"`, `"a"`, true},
		{"unequal strings", "", `"a"`, `"b"`, false},
		{"empty strings", "", `""`, `""`, true},
		{"equal booleans", "", "true", "true", true},
		{"unequal booleans", "", "true", "false", false},
		{"null and null", nullFn, "nf()", "nf()", true},
		{"equal arrays", "", "[1, 2]", "[1, 2]", true},
		{"unequal arrays", "", "[1, 2]", "[2, 1]", false},
		{"equal hashes", "", `{"a": 1}`, `{"a": 1}`, true},
	} {
		askEqualityInBothEngines(t, tc)
	}
}

func TestIntFloatEqualityStillNumeric(t *testing.T) {
	for _, tc := range []crossTypeEqualityCase{
		{"1 vs 1.0", "", "1", "1.0", true},
		{"1.0 vs 1", "", "1.0", "1", true},
		{"2 vs 2.5", "", "2", "2.5", false},
		{"integer and float held in variables", "let i = 3; let f = 3.0;", "i", "f", true},
	} {
		askEqualityInBothEngines(t, tc)
	}
}

// match tests its arms with `==`, so the guard reaches it too: an arm for the
// integer 0 does not catch the string "0", and the reverse.
func TestMatchArmsDoNotCrossTypes(t *testing.T) {
	for _, tc := range []struct {
		input string
		want  string
	}{
		{`let r = match ("0") { 0 => "integer arm", _ => "fallthrough" }; r;`, `STRING("fallthrough")`},
		{`let r = match (0) { "0" => "string arm", _ => "fallthrough" }; r;`, `STRING("fallthrough")`},
		{`let r = match ("true") { true => "boolean arm", _ => "fallthrough" }; r;`, `STRING("fallthrough")`},
		{`let r = match (1) { 1.0 => "float arm", _ => "fallthrough" }; r;`, `STRING("float arm")`},
	} {
		if got := normalize(evalViaEvaluator(tc.input)); got != tc.want {
			t.Errorf("evaluator: %s = %s, want %s", tc.input, got, tc.want)
		}
		viaVM, err := evalViaVM(t, tc.input)
		if err != nil {
			t.Errorf("VM declined %s: %v", tc.input, err)
			continue
		}
		if got := normalize(viaVM); got != tc.want {
			t.Errorf("VM: %s = %s, want %s", tc.input, got, tc.want)
		}
	}
}
