package server

// M26-LSP-034: a document the store could not keep in step went on answering
// from the text it had stopped being.
//
// A change the store cannot apply leaves a document desynced: the text the
// server holds is known not to be the client's. M26-LSP-004 made the four
// requests that hand back an edit built from that text refuse, and left the
// rest answering from it. Hover described a value that was not there,
// definition jumped to a line that had moved, rename answered a WorkspaceEdit
// measured in the old text, member completion offered a TextEdit at a stale
// position, a configuration change republished diagnostics over code that was
// fine, and other files went on reading the desynced document's entry in the
// cross-file index -- a rename started in a file that was in step wrote an
// edit into one that was not.
//
// It is decided once now, at the refusal. Everything derived from the document
// is dropped, so every request answers for it as for a document nothing has
// analysed, nothing another file asks reaches into it, a rename that crosses
// files is refused while a document is out of step, republishing skips it, and
// the user is told once why its features have gone quiet. A whole-document
// change or a fresh didOpen puts everything back.

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"mutant/lsp/internal/analyzer"
	localprotocol "mutant/lsp/internal/protocol"

	"github.com/tliron/glsp"
	lsp "github.com/tliron/glsp/protocol_3_16"
)

// desyncFixture gives every request something to answer while it is in step.
// `unused  =  2` is spaced wrongly on purpose, so that the three formatting
// requests have an edit to offer, and "notes.txt" names a file beside it, so
// that documentLink has a link to offer.
const desyncFixture = "struct Point { x, y }\n" +
	"let p = Point{x: 1, y: 2};\n" +
	"let alpha = 1;\n" +
	"let unused  =  2;\n" +
	"let notes = \"notes.txt\";\n" +
	"let double = fn(n) { return n * 2; };\n" +
	"let add = fn(a, b) {\n" +
	"\treturn double(a) + b;\n" +
	"};\n" +
	"putln(add(alpha, 2));\n" +
	"putln(p.x);\n" +
	"putln(notes);\n"

// unusableChange is a change the store has to refuse: a range whose start is
// after its end. It is the one change a client can send over JSON that knocks a
// document out of step, because glsp decodes only the two change-event types
// and any other range is clamped into the document.
func unusableChange(uri lsp.DocumentUri, version int32) lsp.DidChangeTextDocumentParams {
	backwards := lsp.Range{Start: lsp.Position{Line: 0, Character: 8}, End: lsp.Position{Line: 0, Character: 2}}
	return lsp.DidChangeTextDocumentParams{
		TextDocument: lsp.VersionedTextDocumentIdentifier{
			TextDocumentIdentifier: lsp.TextDocumentIdentifier{URI: uri},
			Version:                version,
		},
		ContentChanges: []any{lsp.TextDocumentContentChangeEvent{Range: &backwards, Text: "X"}},
	}
}

// typingChange is an ordinary keystroke: one character inserted at the start.
func typingChange(uri lsp.DocumentUri, version int32) lsp.DidChangeTextDocumentParams {
	at := lsp.Range{Start: lsp.Position{Line: 0, Character: 0}, End: lsp.Position{Line: 0, Character: 0}}
	return lsp.DidChangeTextDocumentParams{
		TextDocument: lsp.VersionedTextDocumentIdentifier{
			TextDocumentIdentifier: lsp.TextDocumentIdentifier{URI: uri},
			Version:                version,
		},
		ContentChanges: []any{lsp.TextDocumentContentChangeEvent{Range: &at, Text: " "}},
	}
}

// wholeDocumentChange sends the document entire, the one change that puts a
// desynced document back in step.
func wholeDocumentChange(uri lsp.DocumentUri, version int32, text string) lsp.DidChangeTextDocumentParams {
	return lsp.DidChangeTextDocumentParams{
		TextDocument: lsp.VersionedTextDocumentIdentifier{
			TextDocumentIdentifier: lsp.TextDocumentIdentifier{URI: uri},
			Version:                version,
		},
		ContentChanges: []any{lsp.TextDocumentContentChangeEventWhole{Text: text}},
	}
}

// changeThroughTheHandler sends one didChange the way a client does and returns
// the notifications it caused. A refused change is still a notification, so
// its error reaches nobody, and none is checked here.
func changeThroughTheHandler(t *testing.T, s *Server, params lsp.DidChangeTextDocumentParams) []capturedNotification {
	t.Helper()
	var notes []capturedNotification
	_, _, _, _ = s.handler.Handle(&glsp.Context{
		Method: string(lsp.MethodTextDocumentDidChange),
		Params: mustJSON(t, params),
		Notify: func(method string, params any) {
			notes = append(notes, capturedNotification{method: method, params: params})
		},
	})
	return notes
}

// changeDirectly is changeThroughTheHandler for a server serverOver built,
// which is never initialized, so its handler refuses every message.
func changeDirectly(s *Server, params lsp.DidChangeTextDocumentParams) []capturedNotification {
	var notes []capturedNotification
	_ = s.didChange(&glsp.Context{Notify: func(method string, params any) {
		notes = append(notes, capturedNotification{method: method, params: params})
	}}, &params)
	return notes
}

// openDirectly opens a document on a server serverOver built.
func openDirectly(s *Server, uri lsp.DocumentUri, text string) {
	_ = s.didOpen(&glsp.Context{Notify: func(string, any) {}}, &lsp.DidOpenTextDocumentParams{
		TextDocument: lsp.TextDocumentItem{URI: uri, LanguageID: "mutant", Version: 1, Text: text},
	})
}

func requireDesynced(t *testing.T, s *Server, uri lsp.DocumentUri) {
	t.Helper()
	if doc, ok := s.documents.Snapshot(uri); !ok || doc == nil || !doc.Desynced {
		t.Fatalf("the store does not hold %s desynced (open=%t)", uri, ok)
	}
}

func requireInStep(t *testing.T, s *Server, uri lsp.DocumentUri) {
	t.Helper()
	if doc, ok := s.documents.Snapshot(uri); !ok || doc == nil || doc.Desynced {
		t.Fatalf("the store does not hold %s in step (open=%t)", uri, ok)
	}
}

// shownMessages are the window/showMessage notifications among notes.
func shownMessages(t *testing.T, notes []capturedNotification) []lsp.ShowMessageParams {
	t.Helper()
	var shown []lsp.ShowMessageParams
	for _, note := range notes {
		if note.method != string(lsp.ServerWindowShowMessage) {
			continue
		}
		params, ok := note.params.(lsp.ShowMessageParams)
		if !ok {
			t.Fatalf("showMessage params type = %T, want ShowMessageParams", note.params)
		}
		shown = append(shown, params)
	}
	return shown
}

// diagnosticsPublishedFor are the publishDiagnostics notifications among notes
// that are about uri.
func diagnosticsPublishedFor(notes []capturedNotification, uri lsp.DocumentUri) []lsp.PublishDiagnosticsParams {
	var published []lsp.PublishDiagnosticsParams
	for _, note := range notes {
		params, ok := note.params.(lsp.PublishDiagnosticsParams)
		if ok && note.method == string(lsp.ServerTextDocumentPublishDiagnostics) && params.URI == uri {
			published = append(published, params)
		}
	}
	return published
}

// featureAnswer is what one request answered for a document, described, or ""
// when it answered nothing.
type featureAnswer struct {
	feature string
	answer  string
	err     error
}

// describeAnswer is "" for a result that says nothing -- nil, a nil pointer, an
// empty list -- and a short rendering of anything else.
func describeAnswer(result any) string {
	v := reflect.ValueOf(result)
	if !v.IsValid() {
		return ""
	}
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface, reflect.Map, reflect.Slice:
		if v.IsNil() {
			return ""
		}
	}
	if (v.Kind() == reflect.Slice || v.Kind() == reflect.Map) && v.Len() == 0 {
		return ""
	}
	out := fmt.Sprintf("%+v", result)
	if len(out) > 160 {
		out = out[:160] + "..."
	}
	return out
}

// openDesyncFixture opens desyncFixture at a real path, so that its string
// literal can name a real file, and returns its URI with the two things a
// client holds on to between requests: the unused-declaration diagnostic it
// was sent, and a call hierarchy item for add.
func openDesyncFixture(t *testing.T, s *Server) (lsp.DocumentUri, lsp.Diagnostic, lsp.CallHierarchyItem) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("notes\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	uri := pathToURI(filepath.Join(dir, "out-of-step.mut"))
	openDocument(t, s, uri, desyncFixture)

	snapshot, ok := s.snapshot(uri)
	if !ok {
		t.Fatal("the fixture has no snapshot")
	}
	var unused lsp.Diagnostic
	found := false
	for _, diagnostic := range s.diagnose(snapshot) {
		if strings.HasPrefix(diagnostic.Message, "unused declaration `unused`") {
			unused, found = diagnostic, true
		}
	}
	if !found {
		t.Fatalf("the fixture draws no unused-declaration diagnostic: %q", diagnosticMessages(s.diagnose(snapshot)))
	}
	items, err := s.prepareCallHierarchy(nil, &lsp.CallHierarchyPrepareParams{
		TextDocumentPositionParams: lsp.TextDocumentPositionParams{
			TextDocument: lsp.TextDocumentIdentifier{URI: uri},
			Position:     lsp.Position{Line: 6, Character: 4},
		},
	})
	if err != nil || len(items) != 1 {
		t.Fatalf("prepareCallHierarchy on add = %+v, %v; want one item", items, err)
	}
	return uri, unused, items[0]
}

// askEveryFeature sends desyncFixture's document every request the server
// answers for a document, at a position where each has something to say, and
// describes what each answered.
func askEveryFeature(t *testing.T, s *Server, uri lsp.DocumentUri, unused lsp.Diagnostic, call lsp.CallHierarchyItem) []featureAnswer {
	t.Helper()
	at := func(line, character uint32) lsp.TextDocumentPositionParams {
		return lsp.TextDocumentPositionParams{
			TextDocument: lsp.TextDocumentIdentifier{URI: uri},
			Position:     lsp.Position{Line: line, Character: character},
		}
	}
	// alpha's use, inside add's argument list, and p in p.x.
	alphaUse, insideCall, receiver := at(9, 11), at(9, 10), at(10, 6)
	doc := lsp.TextDocumentIdentifier{URI: uri}
	whole := lsp.Range{Start: lsp.Position{Line: 0, Character: 0}, End: lsp.Position{Line: 12, Character: 0}}
	ctx := &glsp.Context{Notify: func(string, any) {}}

	var answers []featureAnswer
	record := func(feature string, result any, err error) {
		answers = append(answers, featureAnswer{feature: feature, answer: describeAnswer(result), err: err})
	}

	hover, err := s.hover(ctx, &lsp.HoverParams{TextDocumentPositionParams: alphaUse})
	record("hover", hover, err)

	// Completion always offers the language's own names, which need no
	// document. What a document adds is the part that has to go: names it
	// declares, and items carrying a TextEdit measured in its text.
	languageWide := map[string]bool{}
	for _, item := range analyzer.New().Analyze("").CompletionItems() {
		languageWide[item.Label] = true
	}
	completion, err := s.completion(ctx, &lsp.CompletionParams{TextDocumentPositionParams: at(10, 8)})
	var fromDocument []string
	if list, ok := completion.(*lsp.CompletionList); ok && list != nil {
		for _, item := range list.Items {
			if item.TextEdit != nil || !languageWide[item.Label] {
				fromDocument = append(fromDocument, item.Label)
			}
		}
	}
	record("completion", fromDocument, err)

	help, err := s.signatureHelp(ctx, &lsp.SignatureHelpParams{TextDocumentPositionParams: insideCall})
	record("signatureHelp", help, err)
	highlights, err := s.documentHighlights(ctx, &lsp.DocumentHighlightParams{TextDocumentPositionParams: alphaUse})
	record("documentHighlight", highlights, err)
	symbols, err := s.documentSymbols(ctx, &lsp.DocumentSymbolParams{TextDocument: doc})
	record("documentSymbol", symbols, err)
	folds, err := s.foldingRanges(ctx, &lsp.FoldingRangeParams{TextDocument: doc})
	record("foldingRange", folds, err)
	lenses, err := s.codeLens(ctx, &lsp.CodeLensParams{TextDocument: doc})
	record("codeLens", lenses, err)
	links, err := s.documentLinks(ctx, &lsp.DocumentLinkParams{TextDocument: doc})
	record("documentLink", links, err)
	definition, err := s.definition(ctx, &lsp.DefinitionParams{TextDocumentPositionParams: alphaUse})
	record("definition", definition, err)
	typeDefinition, err := s.typeDefinition(ctx, &lsp.TypeDefinitionParams{TextDocumentPositionParams: receiver})
	record("typeDefinition", typeDefinition, err)
	references, err := s.references(ctx, &lsp.ReferenceParams{TextDocumentPositionParams: alphaUse, Context: lsp.ReferenceContext{IncludeDeclaration: true}})
	record("references", references, err)
	prepared, err := s.prepareRename(ctx, &lsp.PrepareRenameParams{TextDocumentPositionParams: alphaUse})
	record("prepareRename", prepared, err)
	renamed, err := s.rename(ctx, &lsp.RenameParams{TextDocumentPositionParams: alphaUse, NewName: "beta"})
	record("rename", renamed, err)
	actions, err := s.codeActions(ctx, &lsp.CodeActionParams{
		TextDocument: doc,
		Range:        unused.Range,
		Context:      lsp.CodeActionContext{Diagnostics: []lsp.Diagnostic{unused}},
	})
	record("codeAction", actions, err)
	formatted, err := s.formatting(ctx, &lsp.DocumentFormattingParams{TextDocument: doc})
	record("formatting", formatted, err)
	rangeFormatted, err := s.rangeFormatting(ctx, &lsp.DocumentRangeFormattingParams{TextDocument: doc, Range: whole})
	record("rangeFormatting", rangeFormatted, err)
	typed, err := s.onTypeFormatting(ctx, &lsp.DocumentOnTypeFormattingParams{TextDocumentPositionParams: at(3, 17), Ch: ";"})
	record("onTypeFormatting", typed, err)

	full, err := s.semanticTokensFull(ctx, &lsp.SemanticTokensParams{TextDocument: doc})
	record("semanticTokens/full", full, err)
	previous := ""
	if full != nil && full.ResultID != nil {
		previous = *full.ResultID
	}
	ranged, err := s.semanticTokensRange(ctx, &lsp.SemanticTokensRangeParams{TextDocument: doc, Range: whole})
	record("semanticTokens/range", ranged, err)
	delta, err := s.semanticTokensFullDelta(ctx, &lsp.SemanticTokensDeltaParams{TextDocument: doc, PreviousResultID: previous})
	record("semanticTokens/full/delta", delta, err)
	record("inlayHint", s.inlayHints(&localprotocol.InlayHintParams{TextDocument: doc, Range: whole}), nil)

	roots, err := s.prepareCallHierarchy(ctx, &lsp.CallHierarchyPrepareParams{TextDocumentPositionParams: at(6, 4)})
	record("prepareCallHierarchy", roots, err)
	incoming, err := s.callHierarchyIncomingCalls(ctx, &lsp.CallHierarchyIncomingCallsParams{Item: call})
	record("callHierarchy/incomingCalls", incoming, err)
	outgoing, err := s.callHierarchyOutgoingCalls(ctx, &lsp.CallHierarchyOutgoingCallsParams{Item: call})
	record("callHierarchy/outgoingCalls", outgoing, err)

	// The cross-file index: what this document declares, as every other file
	// and the workspace symbol search see it.
	found, err := s.workspaceSymbols(ctx, &lsp.WorkspaceSymbolParams{Query: "double"})
	var declaredHere []string
	for _, symbol := range found {
		if symbol.Location.URI == uri {
			declaredHere = append(declaredHere, symbol.Name)
		}
	}
	record("workspace/symbol", declaredHere, err)

	return answers
}

// The row's regression test, widened from the three requests it names to every
// request the server answers for a document. Each is asked first while the
// document is in step, where it has to answer, so that its silence afterwards
// is the desync and not a fixture with nothing to say.
func TestADesyncedDocumentAnswersNothing(t *testing.T) {
	s := New(false)
	initializeServer(t, s)
	uri, unused, call := openDesyncFixture(t, s)

	for _, a := range askEveryFeature(t, s, uri, unused, call) {
		if a.answer == "" {
			t.Errorf("%s answered nothing while the document was in step (err %v), so its silence once desynced would prove nothing", a.feature, a.err)
		}
	}
	if t.Failed() {
		t.FailNow()
	}

	changeThroughTheHandler(t, s, unusableChange(uri, 2))
	requireDesynced(t, s, uri)

	for _, a := range askEveryFeature(t, s, uri, unused, call) {
		if a.answer != "" {
			t.Errorf("%s answered on a desynced document, from text the client does not have: %s", a.feature, a.answer)
		}
	}

	// A configuration change republishes every open document's diagnostics. The
	// desynced one must be skipped, or the stale set didChange took down comes
	// back over code that is fine; the document in step beside it shows that the
	// republish did run.
	const inStep = lsp.DocumentUri("file:///in-step.mut")
	openDocument(t, s, inStep, "let spare = 1;\nputln(0);\n")
	var notes []capturedNotification
	_, _, _, err := s.handler.Handle(&glsp.Context{
		Method: string(lsp.MethodWorkspaceDidChangeConfiguration),
		Params: mustJSON(t, lsp.DidChangeConfigurationParams{Settings: map[string]any{}}),
		Notify: func(method string, params any) {
			notes = append(notes, capturedNotification{method: method, params: params})
		},
	})
	if err != nil {
		t.Fatalf("didChangeConfiguration returned error: %v", err)
	}
	republished := diagnosticsPublishedFor(notes, inStep)
	if len(republished) == 0 || len(republished[len(republished)-1].Diagnostics) == 0 {
		t.Errorf("didChangeConfiguration republished nothing for the document in step, so the skip below proves nothing: %+v", republished)
	}
	for _, params := range diagnosticsPublishedFor(notes, uri) {
		if len(params.Diagnostics) > 0 {
			t.Errorf("didChangeConfiguration republished %d stale diagnostic(s) on the desynced document: %q",
				len(params.Diagnostics), diagnosticMessages(params.Diagnostics))
		}
	}
}

// Features that vanish without a word read as the editor breaking, so the user
// is told -- once, not on every keystroke the store goes on refusing, and again
// only when the document has been back in step and falls out a second time. The
// message names the file and says how to get its features back.
func TestADesyncedDocumentIsReportedOnce(t *testing.T) {
	s := New(false)
	initializeServer(t, s)
	uri := pathToURI(filepath.Join(t.TempDir(), "out-of-step.mut"))
	const text = "let alpha = 1;\nputln(alpha);\n"
	openDocument(t, s, uri, text)

	notes := changeThroughTheHandler(t, s, unusableChange(uri, 2))
	requireDesynced(t, s, uri)
	shown := shownMessages(t, notes)
	if len(shown) != 1 {
		t.Fatalf("the refused change showed %d message(s), want 1: %+v", len(shown), shown)
	}
	if shown[0].Type != lsp.MessageTypeWarning {
		t.Errorf("the notice is message type %v, want a warning (%v)", shown[0].Type, lsp.MessageTypeWarning)
	}
	if !strings.Contains(shown[0].Message, "out-of-step.mut") {
		t.Errorf("the notice does not name the file: %q", shown[0].Message)
	}
	if !strings.Contains(strings.ToLower(shown[0].Message), "close and reopen") {
		t.Errorf("the notice does not say to close and reopen the file: %q", shown[0].Message)
	}
	// The diagnostics already on screen describe text the client does not have,
	// so the refusal takes them down (M26-LSP-004).
	published := diagnosticsPublishedFor(notes, uri)
	if len(published) != 1 || len(published[0].Diagnostics) != 0 {
		t.Errorf("the refused change published %+v for the document, want one empty set", published)
	}

	// Every keystroke after it is refused as well, and none says so again.
	for version := int32(3); version <= 4; version++ {
		notes := changeThroughTheHandler(t, s, typingChange(uri, version))
		requireDesynced(t, s, uri)
		if again := shownMessages(t, notes); len(again) != 0 {
			t.Errorf("refused change v%d showed the notice again: %+v", version, again)
		}
	}

	// The whole document puts it back in step, which needs no message...
	notes = changeThroughTheHandler(t, s, wholeDocumentChange(uri, 5, text))
	requireInStep(t, s, uri)
	if unexpected := shownMessages(t, notes); len(unexpected) != 0 {
		t.Errorf("the whole-document change showed %+v", unexpected)
	}

	// ...and falling out of step after that is news again.
	notes = changeThroughTheHandler(t, s, unusableChange(uri, 6))
	requireDesynced(t, s, uri)
	if shown := shownMessages(t, notes); len(shown) != 1 {
		t.Errorf("falling out of step a second time showed %d message(s), want 1: %+v", len(shown), shown)
	}
}

// What the refusal takes away, the client sending the document whole gives
// back: every request answers again, and from the text that came back rather
// than the text that was lost.
func TestAWholeDocumentChangeBringsEveryAnswerBack(t *testing.T) {
	s := New(false)
	initializeServer(t, s)
	uri, unused, call := openDesyncFixture(t, s)
	changeThroughTheHandler(t, s, unusableChange(uri, 2))
	requireDesynced(t, s, uri)

	// gamma is as long as alpha, so every position askEveryFeature uses still
	// lands where it did.
	resynced := strings.ReplaceAll(desyncFixture, "alpha", "gamma")
	notes := changeThroughTheHandler(t, s, wholeDocumentChange(uri, 3, resynced))
	requireInStep(t, s, uri)
	if published := diagnosticsPublishedFor(notes, uri); len(published) != 1 || len(published[0].Diagnostics) == 0 {
		t.Errorf("the whole-document change published %+v, want the resynchronised text's diagnostics", published)
	}

	for _, a := range askEveryFeature(t, s, uri, unused, call) {
		if a.answer == "" {
			t.Errorf("%s still answers nothing after the client sent the document whole (err %v)", a.feature, a.err)
		}
	}

	hover, _ := s.hover(nil, &lsp.HoverParams{TextDocumentPositionParams: lsp.TextDocumentPositionParams{
		TextDocument: lsp.TextDocumentIdentifier{URI: uri},
		Position:     lsp.Position{Line: 9, Character: 11},
	}})
	if hover == nil || !strings.Contains(hover.Contents.(lsp.MarkupContent).Value, "gamma") {
		t.Errorf("hover after the resync = %+v, want a card for gamma, the name in the text the client sent", hover)
	}
	for name, want := range map[string]bool{"gamma": true, "alpha": false} {
		indexed := false
		for _, symbol := range s.symbols.WorkspaceSymbols(name, 100) {
			indexed = indexed || symbol.Location.URI == uri
		}
		if indexed != want {
			t.Errorf("the workspace index lists %s for the resynchronised document: %t, want %t", name, indexed, want)
		}
	}
}

// The notice tells the user to close and reopen the file, so that has to work:
// a fresh didOpen carries the whole text, and every request answers again.
func TestReopeningADesyncedDocumentBringsEveryAnswerBack(t *testing.T) {
	s := New(false)
	initializeServer(t, s)
	uri, unused, call := openDesyncFixture(t, s)
	changeThroughTheHandler(t, s, unusableChange(uri, 2))
	requireDesynced(t, s, uri)

	_, _, _, err := s.handler.Handle(&glsp.Context{
		Method: string(lsp.MethodTextDocumentDidClose),
		Params: mustJSON(t, lsp.DidCloseTextDocumentParams{TextDocument: lsp.TextDocumentIdentifier{URI: uri}}),
		Notify: func(string, any) {},
	})
	if err != nil {
		t.Fatalf("didClose returned error: %v", err)
	}
	openDocument(t, s, uri, desyncFixture)
	requireInStep(t, s, uri)

	for _, a := range askEveryFeature(t, s, uri, unused, call) {
		if a.answer == "" {
			t.Errorf("%s still answers nothing after the document was closed and reopened (err %v)", a.feature, a.err)
		}
	}

	// Reopened, it is a document that has not yet been out of step, so falling
	// out again is reported again.
	notes := changeThroughTheHandler(t, s, unusableChange(uri, 2))
	requireDesynced(t, s, uri)
	if shown := shownMessages(t, notes); len(shown) != 1 {
		t.Errorf("falling out of step after a reopen showed %d message(s), want 1: %+v", len(shown), shown)
	}
}

// An importer that falls out of step stops speaking for itself in other files'
// answers: its uses leave the references and the call hierarchy of what it
// imports, its declarations leave the workspace symbol search, and the
// reference count above the imported declaration drops to what is known.
func TestADesyncedImporterContributesNothingToOtherFiles(t *testing.T) {
	files := map[string]string{
		"lib.mut":  "let mean = fn(xs) { return 0; };\n",
		"main.mut": "import stats \"lib.mut\";\nlet total = stats.mean([1]);\nputln(total);\n",
	}
	root := writeModules(t, files)
	s, libURI := serverOver(t, root, "lib.mut")
	mainURI := pathToURI(filepath.Join(root, "main.mut"))
	openDirectly(s, mainURI, files["main.mut"])

	declaration := lsp.TextDocumentPositionParams{
		TextDocument: lsp.TextDocumentIdentifier{URI: libURI},
		Position:     lsp.Position{Line: 0, Character: 4},
	}
	items, err := s.prepareCallHierarchy(nil, &lsp.CallHierarchyPrepareParams{TextDocumentPositionParams: declaration})
	if err != nil || len(items) != 1 {
		t.Fatalf("prepareCallHierarchy on mean = %+v, %v; want one item", items, err)
	}

	// Each question is answered with what it says about main.mut, or "".
	aboutMain := func() map[string]string {
		said := map[string]string{}
		refs, _ := s.references(nil, &lsp.ReferenceParams{TextDocumentPositionParams: declaration, Context: lsp.ReferenceContext{IncludeDeclaration: true}})
		for _, ref := range refs {
			if ref.URI == mainURI {
				said["references to mean"] = fmt.Sprintf("%+v", ref.Range)
			}
		}
		calls, _ := s.callHierarchyIncomingCalls(nil, &lsp.CallHierarchyIncomingCallsParams{Item: items[0]})
		for _, call := range calls {
			if call.From.URI == mainURI {
				said["callers of mean"] = call.From.Name
			}
		}
		symbols, _ := s.workspaceSymbols(nil, &lsp.WorkspaceSymbolParams{Query: "total"})
		for _, symbol := range symbols {
			if symbol.Location.URI == mainURI {
				said["workspace symbol total"] = symbol.Name
			}
		}
		lenses, _ := s.codeLens(nil, &lsp.CodeLensParams{TextDocument: lsp.TextDocumentIdentifier{URI: libURI}})
		for _, lens := range lenses {
			if lens.Command != nil && lens.Command.Title != "0 references" {
				said["reference count above mean"] = lens.Command.Title
			}
		}
		return said
	}

	inStep := aboutMain()
	for _, question := range []string{"references to mean", "callers of mean", "workspace symbol total", "reference count above mean"} {
		if inStep[question] == "" {
			t.Errorf("%s says nothing about main.mut while it is in step, so its silence below would prove nothing", question)
		}
	}
	if t.Failed() {
		t.FailNow()
	}

	changeDirectly(s, unusableChange(mainURI, 2))
	requireDesynced(t, s, mainURI)
	for question, answer := range aboutMain() {
		t.Errorf("%s still answers from the desynced main.mut: %s", question, answer)
	}
}

// A module that falls out of step is, to the files that import it, a module
// nobody has read. Nothing they ask is answered from its text, and nothing in
// them is reported missing for it: sema answers an unread module
// provisionally, and a squiggle invented out of not knowing would be a false
// flag. Back in step, the answers return.
func TestADesyncedModuleAnswersNothingAndIsNotReportedMissing(t *testing.T) {
	files := map[string]string{
		"lib.mut":  "let mean = fn(xs) { return 0; };\n",
		"main.mut": "import stats \"lib.mut\";\nlet total = stats.mean([1]);\nputln(total);\n",
	}
	root := writeModules(t, files)
	s, mainURI := serverOver(t, root, "main.mut")
	libURI := pathToURI(filepath.Join(root, "lib.mut"))
	openDirectly(s, libURI, files["lib.mut"])

	// mean in `stats.mean`, and just after `stats.`.
	onMean := lsp.TextDocumentPositionParams{
		TextDocument: lsp.TextDocumentIdentifier{URI: mainURI},
		Position:     lsp.Position{Line: 1, Character: 19},
	}
	afterDot := lsp.TextDocumentPositionParams{
		TextDocument: lsp.TextDocumentIdentifier{URI: mainURI},
		Position:     lsp.Position{Line: 1, Character: 18},
	}
	// Each question is answered with what it says out of lib.mut, or "". What
	// main.mut says on its own is a separate question, asked below once
	// lib.mut is out of step.
	fromLib := func() map[string]string {
		said := map[string]string{}
		definition, _ := s.definition(nil, &lsp.DefinitionParams{TextDocumentPositionParams: onMean})
		switch location := definition.(type) {
		case lsp.Location:
			if location.URI == libURI {
				said["definition of stats.mean"] = fmt.Sprintf("%+v", location)
			}
		case *lsp.Location:
			if location != nil && location.URI == libURI {
				said["definition of stats.mean"] = fmt.Sprintf("%+v", *location)
			}
		}
		if hover, _ := s.hover(nil, &lsp.HoverParams{TextDocumentPositionParams: onMean}); hover != nil &&
			strings.Contains(hover.Contents.(lsp.MarkupContent).Value, "declared in") {
			said["hover on stats.mean"] = hover.Contents.(lsp.MarkupContent).Value
		}
		if result, _ := s.completion(nil, &lsp.CompletionParams{TextDocumentPositionParams: afterDot}); result != nil {
			if list, ok := result.(*lsp.CompletionList); ok && list != nil {
				for _, item := range list.Items {
					if item.Label == "mean" {
						said["completion after stats."] = item.Label
					}
				}
			}
		}
		return said
	}

	inStep := fromLib()
	for _, question := range []string{"definition of stats.mean", "hover on stats.mean", "completion after stats."} {
		if inStep[question] == "" {
			t.Errorf("%s says nothing from lib.mut while it is in step, so its silence below would prove nothing", question)
		}
	}
	snapshot, _ := s.snapshot(mainURI)
	before := diagnosticMessages(s.diagnose(snapshot))
	if t.Failed() {
		t.FailNow()
	}

	changeDirectly(s, unusableChange(libURI, 2))
	requireDesynced(t, s, libURI)
	for question, answer := range fromLib() {
		t.Errorf("%s still answers from the desynced lib.mut: %s", question, answer)
	}
	// Nor does main.mut answer in lib.mut's place. `mean` is declared in
	// lib.mut and nowhere in main.mut, and the importer's own walk used to
	// answer anyway: definition with the use's own range, hover with
	// "identifier `mean`" (M26-LSP-048).
	if definition, _ := s.definition(nil, &lsp.DefinitionParams{TextDocumentPositionParams: onMean}); describeAnswer(definition) != "" {
		t.Errorf("definition of stats.mean with lib.mut out of step answered %s, which main.mut cannot know", describeAnswer(definition))
	}
	if hover, _ := s.hover(nil, &lsp.HoverParams{TextDocumentPositionParams: onMean}); hover != nil {
		t.Errorf("hover on stats.mean with lib.mut out of step answered %+v, which main.mut cannot know", hover.Contents)
	}

	// main.mut analysed again, as the next keystroke in it would.
	notes := changeDirectly(s, wholeDocumentChange(mainURI, 2, files["main.mut"]))
	published := diagnosticsPublishedFor(notes, mainURI)
	if len(published) != 1 {
		t.Fatalf("main.mut's change published %d diagnostic set(s), want 1", len(published))
	}
	if after := diagnosticMessages(published[0].Diagnostics); !equalStrings(after, before) {
		t.Errorf("with lib.mut out of step, main.mut is reported for %q, where in step it was reported for %q", after, before)
	}

	changeDirectly(s, wholeDocumentChange(libURI, 3, files["lib.mut"]))
	requireInStep(t, s, libURI)
	if back := fromLib(); back["definition of stats.mean"] == "" {
		t.Errorf("with lib.mut back in step, definition of stats.mean still answers nothing: %+v", back)
	}
}

// A rename that reaches past its own file cannot know it has every use while an
// open document is out of step, because that document's uses are in text the
// server does not have. Started in a file that is in step, it used to write an
// edit into the desynced importer, measured in its stale index entry. It is
// refused now, and says how to recover; a rename that stays in its own file is
// still made, and so is the cross-file one once the importer is back in step.
func TestACrossFileRenameIsRefusedWhileAnImporterIsDesynced(t *testing.T) {
	files := map[string]string{
		"lib.mut":  "let mean = fn(xs) { return len(xs); };\n",
		"main.mut": "import stats \"lib.mut\";\nlet total = stats.mean([1]);\nputln(total);\n",
	}
	root := writeModules(t, files)
	s, libURI := serverOver(t, root, "lib.mut")
	mainURI := pathToURI(filepath.Join(root, "main.mut"))
	openDirectly(s, mainURI, files["main.mut"])

	changeDirectly(s, unusableChange(mainURI, 2))
	requireDesynced(t, s, mainURI)

	onMean := lsp.Position{Line: 0, Character: 4}
	edit, err := requestRename(s, libURI, onMean, "average")
	if err == nil {
		written := 0
		if edit != nil {
			written = len(edit.Changes[mainURI])
		}
		t.Fatalf("rename went through with %d edit(s) for the desynced main.mut, measured in text the client does not have", written)
	}
	if !strings.Contains(err.Error(), "main.mut") || !strings.Contains(err.Error(), "close and reopen") {
		t.Errorf("the refusal does not name the desynced file and how to recover: %v", err)
	}

	// xs is a parameter: nothing outside lib.mut can name it.
	edit, err = requestRename(s, libURI, lsp.Position{Line: 0, Character: 14}, "values")
	if err != nil {
		t.Fatalf("a rename confined to lib.mut was refused: %v", err)
	}
	if edit == nil || len(edit.Changes[libURI]) != 2 || len(edit.Changes) != 1 {
		t.Fatalf("renaming the parameter xs gave %+v, want 2 edits, all in lib.mut", edit)
	}

	changeDirectly(s, wholeDocumentChange(mainURI, 3, files["main.mut"]))
	requireInStep(t, s, mainURI)
	edit, err = requestRename(s, libURI, onMean, "average")
	if err != nil {
		t.Fatalf("with main.mut back in step, the rename is still refused: %v", err)
	}
	if edit == nil || len(edit.Changes[mainURI]) != 1 {
		t.Fatalf("with main.mut back in step, the rename gave %+v, want 1 edit in main.mut", edit)
	}
}
