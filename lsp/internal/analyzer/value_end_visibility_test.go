package analyzer

import (
	"strings"
	"testing"

	lsp "github.com/tliron/glsp/protocol_3_16"
)

// A let whose value is compiled before its names are declared (M26-CMP-001)
// is visible only strictly after the End of that value (M26-SEM-021), never
// at it. The End is exclusive, so it is the position right after the value's
// last character -- which is where the cursor sits while that character is
// being typed, and so where completion and hover are asked most. The
// compiler reads the name there as whatever it meant before the let: a
// builtin, when nothing else binds it.

// valueEndItems returns the completion items at pos labelled label.
func valueEndItems(s *Snapshot, pos lsp.Position, label string) []lsp.CompletionItem {
	var items []lsp.CompletionItem
	for _, item := range s.CompletionItemsAt(pos) {
		if item.Label == label {
			items = append(items, item)
		}
	}
	return items
}

// valueEndDescribe renders items as their kinds and details, for a message.
func valueEndDescribe(items []lsp.CompletionItem) string {
	parts := make([]string, 0, len(items))
	for _, item := range items {
		kind := "no kind"
		if item.Kind != nil {
			switch *item.Kind {
			case lsp.CompletionItemKindFunction:
				kind = "function"
			case lsp.CompletionItemKindVariable:
				kind = "variable"
			default:
				kind = "another kind"
			}
		}
		detail := ""
		if item.Detail != nil {
			detail = *item.Detail
		}
		parts = append(parts, kind+" "+detail)
	}
	return "[" + strings.Join(parts, "; ") + "]"
}

func valueEndIsTheBuiltin(items []lsp.CompletionItem) bool {
	return len(items) == 1 && items[0].Kind != nil && *items[0].Kind == lsp.CompletionItemKindFunction &&
		items[0].Detail != nil && strings.HasPrefix(*items[0].Detail, "builtin")
}

func TestCompletionAtTheEndOfALetsValueOffersTheBuiltinNotTheLet(t *testing.T) {
	// Line 1 is `let keys = ke`: `ke` is characters 11 and 12, so the cursor
	// right after it is at character 13, the value's End.
	src := "let h = {\"a\": 1};\nlet keys = ke"
	s := New().Analyze(src)
	for _, character := range []lsp.UInteger{12, 13} {
		items := valueEndItems(s, lsp.Position{Line: 1, Character: character}, "keys")
		if !valueEndIsTheBuiltin(items) {
			t.Errorf("completion at 1:%d offers %s for keys; want the builtin alone, because a "+
				"keys finished there is the builtin to the compiler: the let does not exist yet",
				character, valueEndDescribe(items))
		}
	}

	// Control: once the statement is behind the cursor, the let is what keys
	// means, and the builtin gives way to it.
	after := New().Analyze("let h = {\"a\": 1};\nlet keys = ke;\n")
	items := valueEndItems(after, lsp.Position{Line: 2, Character: 0}, "keys")
	if len(items) != 1 || items[0].Kind == nil || *items[0].Kind != lsp.CompletionItemKindVariable {
		t.Errorf("control: below the let, completion offers %s for keys; want the let alone",
			valueEndDescribe(items))
	}
}

// TestCompletionWhileALetsValueDoesNotParseOffersTheBuiltin: a value the
// parser drops -- an open parenthesis, a half-typed if, nothing yet -- left
// the let seen from its name on, so the builtin keys was hidden at the very
// cursor typing it. The statement's End stands in for the value's.
func TestCompletionWhileALetsValueDoesNotParseOffersTheBuiltin(t *testing.T) {
	for _, line := range []string{"let keys = (ke", "let keys = if (ke", "let keys = "} {
		s := New().Analyze("let h = {\"a\": 1};\n" + line)
		at := lsp.Position{Line: 1, Character: lsp.UInteger(len(line))}
		if items := valueEndItems(s, at, "keys"); !valueEndIsTheBuiltin(items) {
			t.Errorf("completion at the end of %q offers %s for keys; want the builtin alone: the "+
				"let being typed declares nothing yet", line, valueEndDescribe(items))
		}
	}
}

func TestCompletionAtTheEndOfALetsValueDoesNotOfferTheLet(t *testing.T) {
	// `cou` is characters 12 to 14, so the cursor after it is at 15: the
	// value's End. Nothing called count exists there yet.
	s := New().Analyze("let count = cou")
	if items := valueEndItems(s, lsp.Position{Line: 0, Character: 15}, "count"); len(items) != 0 {
		t.Errorf("completion at the End of `let count = cou` offers %s for count, the let being "+
			"written; it does not exist until its value has been compiled", valueEndDescribe(items))
	}
}

func TestHoverRightAfterALetsValueIsWhatTheValueReads(t *testing.T) {
	src := "let keys = keys;\nkeys;\n"
	s := New().Analyze(src)
	card, isBuiltin := builtinHoverText("keys")
	if !isBuiltin {
		t.Fatalf("keys has no builtin card; this test needs a builtin's name")
	}

	// The value's keys is characters 11 to 14 of line 0, so character 15 is
	// right after it: the value's End, which is still on that identifier.
	text, _, ok := s.HoverText(lsp.Position{Line: 0, Character: 15})
	if !ok || text != card {
		t.Errorf("hover right after the value of `let keys = keys;` = %q (ok=%t); want the builtin's "+
			"card, because the value reads the builtin keys", valueEndFirstLine(text), ok)
	}

	// Control: on the next line keys is the let.
	text, _, ok = s.HoverText(lsp.Position{Line: 1, Character: 0})
	if !ok || text == card || !strings.Contains(text, "identifier `keys`") {
		t.Errorf("control: hover on the use below the let = %q (ok=%t); want the identifier",
			valueEndFirstLine(text), ok)
	}
}

func TestTheSecondNameOfAMultiNameFunctionLetIsNotAFunction(t *testing.T) {
	// `let a, b = fn(x) {...};` destructures a closure: a holds the closure
	// and b holds null (vm destructureValues), so b has no function card to
	// show and is no function to complete.
	src := "let a, b = fn(x) { return x; };\nb;\n"
	s := New().Analyze(src)
	for _, pos := range []lsp.Position{{Line: 1, Character: 0}, {Line: 0, Character: 7}} {
		text, _, ok := s.HoverText(pos)
		if !ok {
			t.Errorf("no hover on b at %d:%d", pos.Line, pos.Character)
			continue
		}
		if strings.HasPrefix(text, "function ") {
			t.Errorf("hover on b at %d:%d is a function card %q; b holds null",
				pos.Line, pos.Character, valueEndFirstLine(text))
		}
	}
	items := valueEndItems(s, lsp.Position{Line: 2, Character: 0}, "b")
	if len(items) != 1 || items[0].Kind == nil || *items[0].Kind != lsp.CompletionItemKindVariable {
		t.Errorf("completion offers %s for b; want one variable", valueEndDescribe(items))
	}
}

func valueEndFirstLine(text string) string {
	if i := strings.IndexByte(text, '\n'); i >= 0 {
		return text[:i]
	}
	return text
}
