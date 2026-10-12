package server

import (
	"strings"
	"testing"

	"mutant/ast"
	"mutant/lsp/internal/analyzer"
)

// An if or match statement is written without a `;` in canonical form, and
// Mutant has no automatic semicolon insertion: when the next statement begins
// with a token that can also continue an expression, dropping the `;` makes the
// parser read the two statements as one. `if (a) { x; };` followed by `(b)();`
// became a call on the if's value, `[1, 2];` an index on it and `-1;` a
// subtraction from it -- a different program, written with no diagnostic
// (M26-LSP-049).
//
// The leads cover a statement beginning with every token the parser has a
// prefix parse function for, so a token that gains an infix meaning later is
// caught here rather than in someone's reformatted file.
var ifMatchContinuationLeads = []struct {
	name string
	stmt string
}{
	{"identifier", "b;"},
	{"integer", "1;"},
	{"float", "1.5;"},
	{"bang", "!a;"},
	{"minus", "-1;"},
	{"tilde", "~1;"},
	{"true", "true;"},
	{"false", "false;"},
	{"paren call", "(b)();"},
	{"paren group", "(1 + 2);"},
	{"if", "if (a) { putln(2); }"},
	{"match", "match (a) { _ => 2 }"},
	{"function", "fn() { return 1; };"},
	{"string", `"s";`},
	{"template", `"t ${a}";`},
	{"array", "[1, 2];"},
	{"hash", `{"k": 1};`},
	{"macro", "macro(x) { x; };"},
}

var ifMatchContinuationOpeners = []struct {
	name string
	stmt string
}{
	{"if", "if (a) { putln(1); };"},
	{"if else", "if (a) { putln(1); } else { putln(0); };"},
	{"match", "match (a) { true => 1, _ => 0 };"},
}

// assertFormatKeepsIfMatchProgram formats src and requires the output to parse
// cleanly to the same tree as src, and to be a fixed point of the formatter.
func assertFormatKeepsIfMatchProgram(t *testing.T, src string) string {
	t.Helper()
	before := analyzer.New().Analyze(src)
	if len(before.ParseErrors) > 0 || before.Program == nil {
		t.Fatalf("test input does not parse: %v\n%s", before.ParseErrors, src)
	}

	got, diags, ok := FormatSource(src)
	if !ok || len(diags) > 0 {
		t.Fatalf("FormatSource refused a valid program: ok=%v diags=%v\n%s", ok, diags, src)
	}

	after := analyzer.New().Analyze(got)
	if len(after.ParseErrors) > 0 || after.Program == nil {
		t.Fatalf("formatted output no longer parses: %v\ninput:\n%s\noutput:\n%s", after.ParseErrors, src, got)
	}
	if !ast.Equal(before.Program, after.Program) {
		t.Fatalf("formatting changed the program\ninput:\n%s\noutput:\n%s\ninput tree:  %s\noutput tree: %s",
			src, got, before.Program.String(), after.Program.String())
	}

	again, _, ok := FormatSource(got)
	if !ok || again != got {
		t.Fatalf("formatting is not idempotent\nfirst:\n%s\nsecond:\n%s", got, again)
	}
	return got
}

func TestFormatIfMatchStatementKeepsProgramBeforeContinuingLead(t *testing.T) {
	const decls = "let a = true;\nlet b = fn() { return 1; };\n"
	for _, opener := range ifMatchContinuationOpeners {
		for _, lead := range ifMatchContinuationLeads {
			t.Run(opener.name+"/"+lead.name+"/top level", func(t *testing.T) {
				assertFormatKeepsIfMatchProgram(t, decls+opener.stmt+"\n"+lead.stmt+"\n")
			})
			t.Run(opener.name+"/"+lead.name+"/in a function body", func(t *testing.T) {
				assertFormatKeepsIfMatchProgram(t, decls+"let f = fn() {\n"+opener.stmt+"\n"+lead.stmt+"\n};\n")
			})
		}
	}
}

// TestFormatIfMatchStatementKeepsSemicolonAcrossComments puts a comment, and a
// blank line, between the if and the statement that would continue it: the
// `;` belongs to the if, so it stays on the if's closing line and before any
// comment trailing it.
func TestFormatIfMatchStatementKeepsSemicolonAcrossComments(t *testing.T) {
	src := "let a = true;\nlet b = fn() { return 1; };\n" +
		"if (a) {\n    putln(1);\n}; // trailing\n\n// leading\n(b)();\n"
	got := assertFormatKeepsIfMatchProgram(t, src)
	if !strings.Contains(got, "}; // trailing\n") {
		t.Errorf("want the `;` straight after the if's closing brace, before its trailing comment; got\n%s", got)
	}
}

// TestFormatIfMatchStatementStaysBareBeforeSafeLead pins the other half: the
// canonical form of an if or match statement is still unterminated wherever
// the next statement cannot continue it, and at the end of a block.
func TestFormatIfMatchStatementStaysBareBeforeSafeLead(t *testing.T) {
	tests := []struct {
		src  string
		want string
	}{
		{"if (a) { putln(1); };\nputln(2);\n", "if (a) {\n    putln(1);\n}\nputln(2);\n"},
		{"match (a) { _ => 1 };\nlet c = 2;\n", "match (a) {\n    _ => 1,\n}\nlet c = 2;\n"},
		{"let f = fn() { if (a) { putln(1); }; };\n", "let f = fn() {\n    if (a) {\n        putln(1);\n    }\n};\n"},
		{"if (a) { putln(1); };\n", "if (a) {\n    putln(1);\n}\n"},
	}
	for _, tt := range tests {
		got := assertFormatKeepsIfMatchProgram(t, "let a = true;\n"+tt.src)
		if want := "let a = true;\n" + tt.want; got != want {
			t.Errorf("formatted %q\n got %q\nwant %q", tt.src, got, want)
		}
	}
}
