package parser

import (
	"strings"
	"testing"

	"mutant/lexer"
)

// A call or array list is closed by its own bracket and by nothing else. The
// list parser used to return as soon as the current token was the closing
// bracket, and after an element that token can be the element's own: in
// `f(1, g(2)` the `)` that ends g(2) is not f's. So an unterminated outer list
// borrowed the inner one's bracket and parsed with zero errors -- `putln(1,`
// followed on the next line by `putln(3);` compiled as putln(1, putln(3)) and
// printed 3 and then 1. These tests fail at 2c5f24f (M26-LEX-022).

func listParseErrors(src string) []string {
	p := New(lexer.New(src))
	p.ParseProgram()
	return p.Errors()
}

func TestAListIsNotClosedByItsLastElementsBracket(t *testing.T) {
	for _, tc := range []struct {
		src  string
		want string
	}{
		{"putln(1,\nputln(3);\n", "expected next token to be , or ), but got ; instead"},
		{"f(1, g(2);\n", "expected next token to be , or ), but got ; instead"},
		{"let a = [[1], [2];\n", "expected next token to be , or ], but got ; instead"},
		{"let x = f(1, (2 + 3);\n", "expected next token to be , or ), but got ; instead"},
	} {
		errs := listParseErrors(tc.src)
		if len(errs) != 1 {
			t.Errorf("%q: got %d errors %v, want exactly one", tc.src, len(errs), errs)
			continue
		}
		if !strings.Contains(errs[0], tc.want) {
			t.Errorf("%q: the error is %q, want it to say %q", tc.src, errs[0], tc.want)
		}
	}
}

// The other direction: a nested element ending in the list's own bracket is
// still an element, and a list whose bracket is written is still closed by it.
// An element that reported an error may legitimately have stopped on the
// list's bracket while recovering, which is the one case the early return is
// for, so a broken element still costs one error and not a second one about
// punctuation.
func TestAListClosedByItsOwnBracketStillParses(t *testing.T) {
	for _, src := range []string{
		"putln(1, putln(3));\n",
		"let a = [[1], [2]];\n",
		"f(g(h(1)));\n",
		"let x = f(1, (2 + 3));\n",
		"f();\n",
		"let a = [];\n",
		"f(1, 2);\n",
	} {
		if errs := listParseErrors(src); len(errs) != 0 {
			t.Errorf("%q: got errors %v, want none", src, errs)
		}
	}

	errs := listParseErrors("putln(0xFF + 1);\n")
	if len(errs) != 1 {
		t.Errorf("a refused element should cost one error, got %d: %v", len(errs), errs)
	}
}
