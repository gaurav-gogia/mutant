package parity

import (
	"testing"

	"mutant/object"
)

// M26-BLT-017. err.context names the builtin that raised the error, and both
// engines call builtins the same way -- from outside the builtin package --
// so both must name the builtin the program called. Each of these named
// another: db_query said builtin.db_query_nodes and net_syn_scan said
// builtin.net_connect_scan, the builtins they reuse, while bin_is_go said
// builtin.is_go and email_urls said builtin.email_ur_ls, which name nothing a
// program can call. The arguments come back from a function so that nothing
// refuses them before the builtin runs.
func TestErrorContextNamesTheCalledBuiltinInBothEngines(t *testing.T) {
	const helpers = `let text = fn() { return "x"; }; let one = fn() { return 1; }; `
	cases := []struct {
		call string
		want string
	}{
		{`db_query(text())`, "builtin.db_query"},
		{`net_syn_scan(one(), 1, 2, 3)`, "builtin.net_syn_scan"},
		{`net_connect_scan(one(), 1, 2, 3)`, "builtin.net_connect_scan"},
		{`bin_is_go(one())`, "builtin.bin_is_go"},
		{`email_urls(one())`, "builtin.email_urls"},
	}

	for _, tc := range cases {
		input := helpers + "let value, err = " + tc.call + "; err.context"
		want := normalize(&object.String{Value: tc.want})

		if got := normalize(evalViaEvaluator(input)); got != want {
			t.Errorf("evaluator: %s gave err.context %s, want %s", tc.call, got, want)
		}
		vmObj, vmErr := evalViaVM(t, input)
		if vmErr != nil {
			t.Errorf("vm: %s failed to run: %v", tc.call, vmErr)
			continue
		}
		if got := normalize(vmObj); got != want {
			t.Errorf("vm: %s gave err.context %s, want %s", tc.call, got, want)
		}
	}
}
