package server

import (
	"sort"
	"testing"

	"mutant/lsp/internal/analyzer"
	localprotocol "mutant/lsp/internal/protocol"

	"github.com/tliron/glsp"
	lsp "github.com/tliron/glsp/protocol_3_16"
)

// "Remove unused declaration" and "Remove duplicate top-level declaration"
// deleted the diagnostic's whole physical line, from column 0 to the start of
// the next one, and never looked at what the line held (M26-LSP-009). Both are
// marked preferred, which lets an editor apply them in one step with no menu,
// and the line held more than the declaration often enough to matter: a second
// statement written after it, the rest of a one-line block around it, the
// header of a counted loop, or only the first line of a declaration spread over
// several. Applied one at a time over the shipped examples, 151 of the 178
// fixes broke the file.
//
// The fixes now delete the declaring statement's own extent, widened to whole
// lines only when nothing else is written on them, and are offered only for a
// declaration that can go without taking anything with it: a let binding
// exactly one name to an initializer that runs nothing, or a struct or an enum.
// Everything else is declined. A refusal costs the author a keystroke; a wrong
// deletion costs working code.
//
// Edits are applied the way a client that honours the protocol applies them,
// with EditOffsets -- the store's own reading of a range -- rather than glsp's
// IndexesIn, which answers offset zero for a position past the end of the text.

// openAndReadDiagnostics opens text under uri and returns the diagnostics
// didOpen published for it.
func openAndReadDiagnostics(t *testing.T, s *Server, uri lsp.DocumentUri, text string) []lsp.Diagnostic {
	t.Helper()
	var published []lsp.Diagnostic
	_, _, _, err := s.handler.Handle(&glsp.Context{
		Method: string(lsp.MethodTextDocumentDidOpen),
		Params: mustJSON(t, lsp.DidOpenTextDocumentParams{
			TextDocument: lsp.TextDocumentItem{URI: uri, LanguageID: "mutant", Version: 1, Text: text},
		}),
		Notify: func(_ string, params any) {
			if p, ok := params.(lsp.PublishDiagnosticsParams); ok {
				published = p.Diagnostics
			}
		},
	})
	if err != nil {
		t.Fatalf("didOpen returned error: %v", err)
	}
	return published
}

// replaceWholeDocument sends text as the whole of uri's next version, and
// checks the server took it: a test of a stale diagnostic is only one if the
// text moved.
func replaceWholeDocument(t *testing.T, s *Server, uri lsp.DocumentUri, version lsp.Integer, text string) {
	t.Helper()
	_, _, _, err := s.handler.Handle(&glsp.Context{
		Method: string(lsp.MethodTextDocumentDidChange),
		Params: mustJSON(t, lsp.DidChangeTextDocumentParams{
			TextDocument: lsp.VersionedTextDocumentIdentifier{
				TextDocumentIdentifier: lsp.TextDocumentIdentifier{URI: uri},
				Version:                version,
			},
			ContentChanges: []any{lsp.TextDocumentContentChangeEventWhole{Text: text}},
		}),
		Notify: func(string, any) {},
	})
	if err != nil {
		t.Fatalf("didChange returned error: %v", err)
	}
	if doc, ok := s.documents.Snapshot(uri); !ok || doc == nil || doc.Text != text {
		t.Fatalf("the server did not take version %d of the document", version)
	}
}

// quickFixesOffered asks for the code actions of one diagnostic, as an editor
// does when the cursor is on its squiggle.
func quickFixesOffered(t *testing.T, s *Server, uri lsp.DocumentUri, diagnostic lsp.Diagnostic) []lsp.CodeAction {
	t.Helper()
	result, validMethod, validParams, err := s.handler.Handle(&glsp.Context{
		Method: string(lsp.MethodTextDocumentCodeAction),
		Params: mustJSON(t, lsp.CodeActionParams{
			TextDocument: lsp.TextDocumentIdentifier{URI: uri},
			Range:        diagnostic.Range,
			Context:      lsp.CodeActionContext{Diagnostics: []lsp.Diagnostic{diagnostic}},
		}),
	})
	if err != nil {
		t.Fatalf("codeAction returned error: %v", err)
	}
	if !validMethod || !validParams {
		t.Fatalf("codeAction validity flags = method:%t params:%t", validMethod, validParams)
	}
	if result == nil {
		return nil
	}
	actions, ok := result.([]lsp.CodeAction)
	if !ok {
		t.Fatalf("codeAction result type = %T, want []CodeAction", result)
	}
	return actions
}

// actionTitled returns the offered action with the given title, if there is one.
func actionTitled(actions []lsp.CodeAction, title string) (lsp.CodeAction, bool) {
	for _, action := range actions {
		if action.Title == title {
			return action, true
		}
	}
	return lsp.CodeAction{}, false
}

func actionTitles(actions []lsp.CodeAction) []string {
	titles := make([]string, 0, len(actions))
	for _, action := range actions {
		titles = append(titles, action.Title)
	}
	return titles
}

// diagnosticReading returns the published diagnostic whose message is exactly
// message. Its absence fails the test: a fix that was never asked for proves
// nothing by not being offered.
func diagnosticReading(t *testing.T, published []lsp.Diagnostic, message string) lsp.Diagnostic {
	t.Helper()
	for _, diagnostic := range published {
		if diagnostic.Message == message {
			return diagnostic
		}
	}
	t.Fatalf("no diagnostic reads %q; published %q", message, diagnosticMessages(published))
	return lsp.Diagnostic{}
}

// applyLikeAConformingClient applies edits to text as the protocol says a
// client does: each range read by EditOffsets, and the edits applied back to
// front so that the earlier offsets stay valid.
func applyLikeAConformingClient(t *testing.T, text string, edits []lsp.TextEdit) string {
	t.Helper()
	type span struct {
		start, end int
		newText    string
	}
	spans := make([]span, 0, len(edits))
	for _, edit := range edits {
		start, end, ok := localprotocol.EditOffsets(text, edit.Range)
		if !ok {
			t.Fatalf("edit range %+v names no part of the document", edit.Range)
		}
		spans = append(spans, span{start, end, edit.NewText})
	}
	sort.Slice(spans, func(i, j int) bool { return spans[i].start > spans[j].start })
	for _, sp := range spans {
		text = text[:sp.start] + sp.newText + text[sp.end:]
	}
	return text
}

func TestRemoveDeclarationQuickFixesDeleteOnlyTheDeclaration(t *testing.T) {
	const (
		removeUnused    = "Remove unused declaration"
		removeDuplicate = "Remove duplicate top-level declaration"
	)
	for _, tc := range []struct {
		name    string
		text    string
		message string // the diagnostic the fix is asked for
		title   string
		// want is the whole document once the fix is applied. It is empty for
		// a declaration the fix must decline, and declines says why.
		want     string
		declines string
	}{
		{
			name:    "unused, with a second statement after it on its line",
			text:    "let a = 1; putln(\"side effect\");\n",
			message: "unused declaration `a`",
			title:   removeUnused,
			want:    "putln(\"side effect\");\n",
		},
		{
			name:    "unused, after a statement on its line",
			text:    "putln(\"x\"); let a = 1;\nputln(\"y\");\n",
			message: "unused declaration `a`",
			title:   removeUnused,
			want:    "putln(\"x\");\nputln(\"y\");\n",
		},
		{
			name:    "unused, a function literal spread over three lines",
			text:    "let helper = fn(x) {\n    return x * 2;\n};\nputln(\"hi\");\n",
			message: "unused declaration `helper`",
			title:   removeUnused,
			want:    "putln(\"hi\");\n",
		},
		{
			name:    "unused, inside a one-line if",
			text:    "if (true) { let unused = 1; putln(\"kept\"); }\n",
			message: "unused declaration `unused`",
			title:   removeUnused,
			want:    "if (true) { putln(\"kept\"); }\n",
		},
		{
			name:    "unused, inside a one-line function body",
			text:    "let f = fn() { let unused = 1; return 2; };\nputln(f());\n",
			message: "unused declaration `unused`",
			title:   removeUnused,
			want:    "let f = fn() { return 2; };\nputln(f());\n",
		},
		{
			// The whole-line case, which was always right and must stay so.
			name:    "unused, on a line of its own in a block",
			text:    "if (true) {\n    let unused = 1;\n    putln(\"kept\");\n}\n",
			message: "unused declaration `unused`",
			title:   removeUnused,
			want:    "if (true) {\n    putln(\"kept\");\n}\n",
		},
		{
			// The start column is counted in UTF-16 units: one character
			// outside the basic multilingual plane is two of them and four
			// bytes.
			name:    "unused, after a character outside the basic multilingual plane on its line",
			text:    "putln(\"\xf0\x9f\x98\x80\"); let a = 1;\n",
			message: "unused declaration `a`",
			title:   removeUnused,
			want:    "putln(\"\xf0\x9f\x98\x80\");\n",
		},
		{
			// M26-LSP-006's last-line case, asked of the fix that replaced
			// lineDeleteRange (it was TestDeletingTheLastLineCoversAllOfIt).
			// The range ends at the end of an unterminated last line, counted
			// in UTF-16 units; counted in runes it ended one unit short and
			// left the final ';' behind.
			name:    "unused, on an unterminated last line holding a character outside the basic multilingual plane",
			text:    "putln(\"x\");\nlet a = \"\xf0\x9f\x98\x80\";",
			message: "unused declaration `a`",
			title:   removeUnused,
			want:    "putln(\"x\");\n",
		},
		{
			name:    "unused, with CRLF line endings",
			text:    "let a = 1;\r\nputln(\"x\");\r\n",
			message: "unused declaration `a`",
			title:   removeUnused,
			want:    "putln(\"x\");\r\n",
		},
		{
			// Two structs of one name compile, so this is a lint finding. The
			// two are identical, so removing either keeps what the program does.
			name:    "duplicate struct, with a second statement after it on its line",
			text:    "struct P { x };\nstruct P { x }; putln(\"kept\");\nputln(P{x: 1});\n",
			message: "duplicate top-level declaration `P`",
			title:   removeDuplicate,
			want:    "struct P { x };\nputln(\"kept\");\nputln(P{x: 1});\n",
		},
		{
			name:    "duplicate struct, spread over three lines",
			text:    "struct P { x };\nstruct P {\n    x\n};\nputln(P{x: 1});\n",
			message: "duplicate top-level declaration `P`",
			title:   removeDuplicate,
			want:    "struct P { x };\nputln(P{x: 1});\n",
		},
		{
			name:    "duplicate enum, with a second statement after it on its line",
			text:    "enum Color { Red };\nenum Color { Red }; putln(\"kept\");\nputln(Color.Red);\n",
			message: "duplicate top-level declaration `Color`",
			title:   removeDuplicate,
			want:    "enum Color { Red };\nputln(\"kept\");\nputln(Color.Red);\n",
		},
		{
			name:     "unused, the second name of a multiple binding",
			text:     "let pair = fn() { return 1, 2; };\nlet a, b = pair();\nputln(a);\n",
			message:  "unused declaration `b`",
			title:    removeUnused,
			declines: "deleting the let deletes a too, which putln reads, and the call that binds both",
		},
		{
			name:     "unused, the let of a counted loop's header",
			text:     "for (let j = 0; false; ) { putln(\"never\"); }\nputln(\"after\");\n",
			message:  "unused declaration `j`",
			title:    removeUnused,
			declines: "the let is part of the loop header, and deleting it leaves `for ( false; )`",
		},
		{
			name:     "unused, an initializer that calls something",
			text:     "let launch = fn() { putln(\"launched\"); return 1; };\nlet x = launch();\nputln(\"y\");\n",
			message:  "unused declaration `x`",
			title:    removeUnused,
			declines: "deleting the let deletes the call, and launch prints",
		},
		{
			name:     "unused, an initializer that can fail",
			text:     "let n = 0;\nlet q = 1 / n;\nputln(n);\n",
			message:  "unused declaration `q`",
			title:    removeUnused,
			declines: "deleting the let deletes a division by zero, which the program fails on",
		},
		{
			name:     "unused, in a file with a parse error elsewhere",
			text:     "let a = 1; putln(\"x\");\nlet b = ;\n",
			message:  "unused declaration `a`",
			title:    removeUnused,
			declines: "a range next to a parse error is where the parser stopped, not where the statement ends",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := New(false)
			initializeServer(t, s)
			uri := lsp.DocumentUri("file:///remove-declaration.mut")
			target := diagnosticReading(t, openAndReadDiagnostics(t, s, uri, tc.text), tc.message)
			action, offered := actionTitled(quickFixesOffered(t, s, uri, target), tc.title)

			if tc.declines != "" {
				if offered {
					t.Fatalf("%q was offered, and leaves %q; it must decline: %s",
						tc.title, applyLikeAConformingClient(t, tc.text, action.Edit.Changes[uri]), tc.declines)
				}
				return
			}

			if !offered {
				t.Fatalf("%q was not offered for %q", tc.title, tc.text)
			}
			if action.Edit == nil {
				t.Fatal("the fix carries no edit")
			}
			after := applyLikeAConformingClient(t, tc.text, action.Edit.Changes[uri])
			if after != tc.want {
				t.Fatalf("the fix left %q, want %q", after, tc.want)
			}

			// The compiler decides whether the result still builds, and it is
			// asked about the original too: a fixture the build already refuses
			// would make that question empty.
			if errs := analyzer.ParseDiagnostics(analyzer.New().Analyze(after)); len(errs) > 0 {
				t.Errorf("the result does not parse: %q", diagnosticMessages(errs))
			}
			if message, refused := compilerRefusalOf(tc.text); refused {
				t.Fatalf("the compiler refuses the fixture itself: %s", message)
			}
			if message, refused := compilerRefusalOf(after); refused {
				t.Errorf("the compiler refuses the result: %s", message)
			}
		})
	}
}

// TestARemovalFixDeclinesADiagnosticTheTextNoLongerBearsOut covers the request
// that arrives late. A code action request carries back the diagnostic the
// client was given, and the text may have moved on since it was published: the
// name may have gained a use, or no longer stand where the diagnostic says.
// The fix is measured against the text it will be applied to, and declines.
func TestARemovalFixDeclinesADiagnosticTheTextNoLongerBearsOut(t *testing.T) {
	const published = "let a = 1;\nputln(\"x\");\n"
	for _, tc := range []struct {
		name string
		now  string
	}{
		{name: "the name has gained a use", now: "let a = 1;\nputln(a);\n"},
		{name: "another name stands where it stood", now: "let b = 1;\nputln(\"x\");\n"},
		{name: "the name has moved off the position", now: "let  a = 1;\nputln(\"x\");\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := New(false)
			initializeServer(t, s)
			uri := lsp.DocumentUri("file:///remove-declaration-stale.mut")
			stale := diagnosticReading(t, openAndReadDiagnostics(t, s, uri, published), "unused declaration `a`")
			if _, offered := actionTitled(quickFixesOffered(t, s, uri, stale), "Remove unused declaration"); !offered {
				t.Fatal("the fix is not offered against the text it was published for, so declining below would prove nothing")
			}

			replaceWholeDocument(t, s, uri, 2, tc.now)
			if action, offered := actionTitled(quickFixesOffered(t, s, uri, stale), "Remove unused declaration"); offered {
				t.Fatalf("the fix was offered against %q and leaves %q",
					tc.now, applyLikeAConformingClient(t, tc.now, action.Edit.Changes[uri]))
			}
		})
	}
}
