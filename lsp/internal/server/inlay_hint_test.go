package server

import (
	"encoding/json"
	"strings"
	"testing"

	localprotocol "mutant/lsp/internal/protocol"

	"github.com/tliron/glsp"
	lsp "github.com/tliron/glsp/protocol_3_16"
)

func TestMethodRouterDispatchesInlayHint(t *testing.T) {
	s := New(false)
	initializeServer(t, s)
	uri := "file:///inlay.mut"
	openMutantDoc(t, s, uri, "let add = fn(a, b) { return a + b; };\nadd(1, 2);\n")

	router := &methodRouter{base: s.handler, srv: s}
	result, validMethod, validParams, err := router.Handle(&glsp.Context{
		Method: localprotocol.MethodTextDocumentInlayHint,
		Params: mustJSON(t, localprotocol.InlayHintParams{
			TextDocument: lsp.TextDocumentIdentifier{URI: lsp.DocumentUri(uri)},
			Range:        lsp.Range{Start: lsp.Position{Line: 0, Character: 0}, End: lsp.Position{Line: 100, Character: 0}},
		}),
	})
	if err != nil {
		t.Fatalf("router inlayHint error: %v", err)
	}
	if !validMethod || !validParams {
		t.Fatalf("validity flags method:%t params:%t", validMethod, validParams)
	}
	hints, ok := result.([]localprotocol.InlayHint)
	if !ok {
		t.Fatalf("result type = %T, want []InlayHint", result)
	}
	if len(hints) == 0 {
		t.Fatal("expected inlay hints for add(1, 2)")
	}
}

func TestMethodRouterDelegatesUnknownMethods(t *testing.T) {
	s := New(false)
	router := &methodRouter{base: s.handler, srv: s}
	// A known glsp method must still be dispatched through the base handler.
	_, validMethod, _, err := router.Handle(&glsp.Context{
		Method: string(lsp.MethodInitialize),
		Params: mustJSON(t, lsp.InitializeParams{}),
	})
	if err != nil {
		t.Fatalf("delegated initialize error: %v", err)
	}
	if !validMethod {
		t.Fatal("initialize should be a valid delegated method")
	}
}

func TestInitializeResultSerializesInlayHintProvider(t *testing.T) {
	s := New(false)
	resultAny, _, _, err := s.handler.Handle(&glsp.Context{
		Method: string(lsp.MethodInitialize),
		Params: mustJSON(t, lsp.InitializeParams{}),
	})
	if err != nil {
		t.Fatalf("initialize error: %v", err)
	}
	raw, err := json.Marshal(resultAny)
	if err != nil {
		t.Fatalf("marshal initialize result: %v", err)
	}
	if !strings.Contains(string(raw), "\"inlayHintProvider\":true") {
		t.Fatalf("serialized capabilities missing inlayHintProvider: %s", raw)
	}
	// Sanity: an existing capability must still serialize alongside it.
	if !strings.Contains(string(raw), "\"foldingRangeProvider\":true") {
		t.Fatalf("serialized capabilities missing foldingRangeProvider: %s", raw)
	}
}

// TestInlayHintIsRefusedOutsideTheSession: inlayHint is the one request the
// router answers itself rather than hand to glsp's handler, and glsp's handler
// is where the session gate is. So inlayHint was served before initialize and
// after shutdown, where glsp refuses every request it routes, hover among them
// (M26-LSP-024). It gets hover's reply now, refusal for refusal, and is still
// served inside the session.
func TestInlayHintIsRefusedOutsideTheSession(t *testing.T) {
	const uri = "file:///inlay-session.mut"
	s := New(false)
	router := &methodRouter{base: s.handler, srv: s}

	type reply struct {
		result                   any
		validMethod, validParams bool
		err                      error
	}
	send := func(method string, params any) reply {
		t.Helper()
		result, validMethod, validParams, err := router.Handle(&glsp.Context{Method: method, Params: mustJSON(t, params)})
		return reply{result, validMethod, validParams, err}
	}
	hover := func() reply {
		t.Helper()
		return send(string(lsp.MethodTextDocumentHover), lsp.HoverParams{
			TextDocumentPositionParams: lsp.TextDocumentPositionParams{
				TextDocument: lsp.TextDocumentIdentifier{URI: lsp.DocumentUri(uri)},
				Position:     lsp.Position{Line: 1, Character: 0},
			},
		})
	}
	inlayHint := func() reply {
		t.Helper()
		return send(localprotocol.MethodTextDocumentInlayHint, localprotocol.InlayHintParams{
			TextDocument: lsp.TextDocumentIdentifier{URI: lsp.DocumentUri(uri)},
			Range:        lsp.Range{Start: lsp.Position{Line: 0, Character: 0}, End: lsp.Position{Line: 100, Character: 0}},
		})
	}
	// refusedLikeHover checks that, outside the session, inlayHint is refused
	// with the reply hover gets there, so the client sees one answer for both.
	refusedLikeHover := func(when string) {
		t.Helper()
		baseline := hover()
		if baseline.err == nil {
			t.Fatalf("hover was served %s, so it is no baseline", when)
		}
		got := inlayHint()
		if got.err == nil {
			hints, _ := got.result.([]localprotocol.InlayHint)
			t.Errorf("inlayHint was served %s with %d hint(s), where hover is refused: %v", when, len(hints), baseline.err)
			return
		}
		if got.result != nil || got.validMethod != baseline.validMethod || got.validParams != baseline.validParams || got.err.Error() != baseline.err.Error() {
			t.Errorf("inlayHint %s = %+v, want hover's reply %+v", when, got, baseline)
		}
	}

	refusedLikeHover("before initialize")

	initializeServer(t, s)
	openMutantDoc(t, s, uri, "let add = fn(a, b) { return a + b; };\nadd(1, 2);\n")
	served := inlayHint()
	if served.err != nil {
		t.Fatalf("inlayHint was refused inside the session: %v", served.err)
	}
	if hints, _ := served.result.([]localprotocol.InlayHint); len(hints) == 0 {
		t.Fatalf("inlayHint inside the session = %#v, want hints for add(1, 2)", served.result)
	}

	if _, _, _, err := router.Handle(&glsp.Context{Method: string(lsp.MethodShutdown)}); err != nil {
		t.Fatalf("shutdown returned error: %v", err)
	}
	refusedLikeHover("after shutdown")
}
