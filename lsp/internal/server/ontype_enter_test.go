package server

import (
	"testing"

	"github.com/tliron/glsp"
	lsp "github.com/tliron/glsp/protocol_3_16"
)

// onTypeFormatted opens text in a fresh server, sends
// textDocument/onTypeFormatting for ch typed at pos, and returns the document
// as the client holds it once the answer is applied.
func onTypeFormatted(t *testing.T, name, text, ch string, pos lsp.Position) string {
	t.Helper()
	s := New(false)
	initializeServer(t, s)
	uri := lsp.DocumentUri("file:///" + name)
	openDocument(t, s, uri, text)

	result, validMethod, validParams, err := s.handler.Handle(&glsp.Context{
		Method: string(lsp.MethodTextDocumentOnTypeFormatting),
		Params: mustJSON(t, lsp.DocumentOnTypeFormattingParams{
			TextDocumentPositionParams: lsp.TextDocumentPositionParams{
				TextDocument: lsp.TextDocumentIdentifier{URI: uri},
				Position:     pos,
			},
			Ch:      ch,
			Options: lsp.FormattingOptions{"tabSize": 4, "insertSpaces": true},
		}),
	})
	if err != nil || !validMethod || !validParams {
		t.Fatalf("onTypeFormatting %q: err=%v method=%t params=%t", ch, err, validMethod, validParams)
	}
	var edits []lsp.TextEdit
	if result != nil {
		var ok bool
		if edits, ok = result.([]lsp.TextEdit); !ok {
			t.Fatalf("onTypeFormatting %q answered %T, want []lsp.TextEdit", ch, result)
		}
	}
	return applyTextEdits(t, text, edits)
}

// TestDocumentOnTypeFormattingLeavesEnterAlone is M26-LSP-019. The server
// registered "\n" as an on-type trigger and answered it by formatting the whole
// document, and the printer has no form for a line that holds only an indent:
// it prints an empty body as `{}`, puts no blank line before a closing brace
// and indents no blank line. So the edit Enter asked for removed the line Enter
// had just made, with the indent the editor had put on it, and a function body
// could not be typed. Each fixture is a document just after Enter, with the
// cursor at the end of the new line's indent, and that line has to survive as
// it is.
func TestDocumentOnTypeFormattingLeavesEnterAlone(t *testing.T) {
	for _, tc := range []struct {
		name string
		text string
		pos  lsp.Position
	}{
		// The row's fixture: Enter between the braces of an empty body.
		{"empty-body.mut", "let f = fn() {\n    \n};\nputln(f());\n", lsp.Position{Line: 1, Character: 4}},
		// Enter after the last statement of a body.
		{"after-last.mut", "let f = fn() {\n    let a = 1;\n    \n};\nputln(f());\n", lsp.Position{Line: 2, Character: 4}},
		// Enter after the last statement of an if block.
		{"if-body.mut", "let a = 1;\nif (a > 0) {\n    putln(a);\n    \n}\n", lsp.Position{Line: 3, Character: 4}},
		// Enter between two statements: the formatter keeps the blank line but
		// not its indent.
		{"between.mut", "let f = fn() {\n    let a = 1;\n    \n    let b = 2;\n};\n", lsp.Position{Line: 2, Character: 4}},
	} {
		line := splitLines(tc.text)[tc.pos.Line]
		kept := func(text string) bool {
			lines := splitLines(text)
			return int(tc.pos.Line) < len(lines) && lines[tc.pos.Line] == line
		}
		// Formatting the whole document has to lose the line, or the fixture
		// cannot show an answer to Enter losing it.
		if kept(format(t, tc.text)) {
			t.Fatalf("%s: formatting the whole document keeps line %d, so the fixture no longer shows the defect", tc.name, tc.pos.Line)
		}
		if got := onTypeFormatted(t, tc.name, tc.text, "\n", tc.pos); !kept(got) {
			t.Errorf("%s: Enter at %d:%d was undone: %q became %q", tc.name, tc.pos.Line, tc.pos.Character, tc.text, got)
		}
	}

	// A trigger the server still registers still formats, so the silence above
	// is the server declining Enter and not an on-type path that answers
	// nothing at all.
	if got, want := onTypeFormatted(t, "semicolon.mut", "let a=1;\n", ";", lsp.Position{Line: 0, Character: 8}), "let a = 1;\n"; got != want {
		t.Errorf("';' gave %q, want %q", got, want)
	}
}
