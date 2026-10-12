package vm

import (
	"strings"
	"testing"
)

// The assertions compare two errors the way `==` does: by what went wrong, not
// by where. The VM stamps a position onto every error a builtin returns, so two
// identical errors built on different lines render differently, and comparing
// renders made assert_eq fail where `==` said the two were equal -- and
// assert_ne pass on that same pair, which `==` also called equal.
//
// Each program first asserts that the two errors really do carry different
// lines and really are `==`, so a pass here cannot come from a position that was
// never stamped.
func TestAssertEqComparesErrorsLikeEquals(t *testing.T) {
	const prelude = "test(\"t\", fn() {\n" +
		"    let a = error(\"boom\");\n" +
		"    let b = error(\"boom\");\n" +
		"    assert_ne(a.line, b.line);\n" +
		"    assert(a == b);\n"

	cases := []struct {
		name     string
		check    string
		want     string
		wantText string
	}{
		{"assert_eq/same failure on two lines", `assert_eq(a, b);`, "t:ok", ""},
		{"assert_ne/same failure on two lines", `assert_ne(a, b);`, "t:FAIL", "assert_ne: both are ERROR:boom"},
		{"assert_contains/same failure on two lines", `assert_contains([a], b);`, "t:ok", ""},
		{"assert_eq/different message", `assert_eq(a, error("bang"));`, "t:FAIL", "assert_eq: got ERROR:boom"},
		{"assert_ne/different message", `assert_ne(a, error("bang"));`, "t:ok", ""},
		{"assert_contains/different message", `assert_contains([a], error("bang"));`, "t:FAIL", "does not contain the element ERROR:bang"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			machine := runTestLedger(t, prelude+"    "+c.check+"\n});")
			if got := strings.Join(resultNames(machine), " "); got != c.want {
				t.Fatalf("ledger = %s, want %s; failures: %+v", got, c.want, machine.TestResults()[0].Failures)
			}
			if c.wantText == "" {
				return
			}
			failures := machine.TestResults()[0].Failures
			if len(failures) != 1 {
				t.Fatalf("expected 1 failure, got %d: %+v", len(failures), failures)
			}
			if !strings.Contains(failures[0].Message, c.wantText) {
				t.Errorf("failure message %q does not contain %q", failures[0].Message, c.wantText)
			}
		})
	}
}
