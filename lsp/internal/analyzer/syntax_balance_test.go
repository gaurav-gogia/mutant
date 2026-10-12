package analyzer

import (
	"strings"
	"testing"
)

// parserMessages returns the messages of every mutant-parser diagnostic in src.
//
// That source covers two producers: the parser's own errors, and the byte-level
// delimiter scan that puts a precise range on an unclosed bracket. Both are
// published at ERROR severity, which `mutant lint` exits 1 on, so a false one
// from either fails a pre-commit hook on a program that runs.
func parserMessages(t *testing.T, src string) []string {
	t.Helper()

	snapshot := New().Analyze(src)
	out := make([]string, 0, 2)
	for _, d := range Diagnostics(snapshot, DefaultLintConfig()) {
		if d.Source != nil && *d.Source == "mutant-parser" {
			out = append(out, d.Message)
		}
	}
	return out
}

// A raw string has no escapes -- that is its entire rule -- so the delimiter
// scan must not read the backslash before its closing quote as escaping it.
// Reading `r"C:\evidence\"` that way left the scan inside a string, which then
// paired the quotes of every later string the wrong way round and made the
// delimiters inside them real. `mutant lint` reported an unclosed `(` and
// exited 1 on a program `mutant run` runs correctly, and a Windows directory
// written as a raw string is ordinary in a DFIR script.
func TestARawStringEndingInABackslashBalances(t *testing.T) {
	sources := []string{
		`let dir = r"C:\evidence\";
putln("(" + dir);`,
		`let dir = r"C:\evidence\";
let note = "[" + dir + "]";
putln(note);`,
		// Raw triple-quoted, which is how a program gets a quote back.
		`let note = r"""he said "(" and left""";
putln(note);`,
		// Plain triple-quoted: a lone quote inside it is text, and only three
		// of them end it.
		`let note = """he said "(" and left""";
putln(note);`,
		// The empty string is two quotes and not an opener.
		`let blank = "";
putln("(" + blank + ")");`,
		// A name ending in `r` is not a raw-string opener. The scan reads bytes
		// rather than tokens, so it has to decide this for itself.
		`let counter = "(";
putln(counter + ")");`,
	}

	for _, src := range sources {
		if messages := parserMessages(t, src); len(messages) != 0 {
			t.Fatalf("a program that parses, want no parser diagnostics:\n%s\ngot %v", src, messages)
		}
	}
}

// A narrowing, not an amputation. The scan exists to name the delimiter that
// was left open, which "unexpected end of input" cannot, and it still does
// wherever the parser actually failed.
func TestAnUnclosedDelimiterIsStillNamed(t *testing.T) {
	cases := []struct {
		src  string
		want string
	}{
		{"let x = (1 + 2;", "unclosed delimiter `(`"},
		{"let xs = [1, 2;", "unclosed delimiter `[`"},
		{"let f = fn() { return 1;", "unclosed delimiter `{`"},
	}

	for _, each := range cases {
		messages := parserMessages(t, each.src)
		found := false
		for _, message := range messages {
			if strings.Contains(message, each.want) {
				found = true
			}
		}
		if !found {
			t.Fatalf("want %q for %q, got %v", each.want, each.src, messages)
		}
	}
}

// The scan is not consulted at all on a program the parser accepted, because a
// program that parses has balanced delimiters by construction: anything the
// scan would say there is the scan misreading the source. The raw-string case
// above is one such misreading, fixed; this is what keeps the next one from
// being published as an error.
func TestTheBalanceScanIsSilentOnAProgramThatParses(t *testing.T) {
	src := `let xs = [1, 2, 3];
let total = 0;
for (x in xs) {
  total = total + x;
}
putln("total: " + str(total));`

	snapshot := New().Analyze(src)
	if len(snapshot.ParseErrors) != 0 {
		t.Fatalf("fixture should parse, got %d parse error(s)", len(snapshot.ParseErrors))
	}
	if messages := parserMessages(t, src); len(messages) != 0 {
		t.Fatalf("want no parser diagnostics, got %v", messages)
	}
}

// identifierByteBefore is what tells the `r` of a raw string from the `r`
// ending a name, and it is the only part of the scan with no source-level
// fixture: there is no infix rule joining a name to a string, so `error"x"` is
// not a program this can be asked about.
func TestIdentifierByteBefore(t *testing.T) {
	cases := []struct {
		text string
		at   int
		want bool
	}{
		{`r"x"`, 0, false},
		{` r"x"`, 1, false},
		{`=r"x"`, 1, false},
		{`(r"x"`, 1, false},
		{`error"x"`, 4, true},
		{`r2r"x"`, 2, true},
		{`_r"x"`, 1, true},
		{`r"x"`, -1, false},
		{`r"x"`, 99, false},
	}

	for _, each := range cases {
		if got := identifierByteBefore(each.text, each.at); got != each.want {
			t.Fatalf("identifierByteBefore(%q, %d) = %v, want %v", each.text, each.at, got, each.want)
		}
	}
}
