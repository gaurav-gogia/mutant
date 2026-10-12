package analyzer

import (
	"slices"
	"strings"
	"testing"

	lsp "github.com/tliron/glsp/protocol_3_16"
)

// A helper declared inside another function is described exactly like one
// declared at the top level. The lookup from a let's name to its function
// literal used to stop at the first function literal it met, so a helper inside
// main had no signature help, no function card and no parameter hints
// (M26-SEM-008).
func TestANestedHelperIsDescribedLikeATopLevelOne(t *testing.T) {
	top := "let pad = fn(text, width) {\n" +
		"  return text;\n" +
		"};\n" +
		"pad(\"a\", 2);\n"
	nested := "let main = fn() {\n" +
		"  let pad = fn(text, width) {\n" +
		"    return text;\n" +
		"  };\n" +
		"  pad(\"a\", 2);\n" +
		"};\n" +
		"main();\n"

	type answer struct {
		signature string
		hover     string
		hints     []string
	}
	ask := func(src string) answer {
		s := New().Analyze(src)
		call := shadowingPos(t, src, "pad(\"a\"", 0)
		label, _ := shadowingSignature(s, shadowingPos(t, src, "pad(\"a\"", len("pad(\"")))
		hover, _, _ := s.HoverText(call)
		return answer{signature: label, hover: hover, hints: parameterHintsOnLine(s, call.Line)}
	}

	want := ask(top)
	got := ask(nested)
	if !strings.HasPrefix(want.signature, "pad(text") {
		t.Fatalf("control: the top-level helper's signature is %q", want.signature)
	}
	if got.signature != want.signature {
		t.Errorf("nested helper signature = %q, want %q, the top-level answer", got.signature, want.signature)
	}
	if got.hover != want.hover {
		t.Errorf("nested helper hover = %q, want the function card %q", got.hover, want.hover)
	}
	if !slices.Equal(got.hints, []string{"text:", "width:"}) {
		t.Errorf("nested helper parameter hints = %v, want [text: width:]", got.hints)
	}
}

func TestAHelperIsFoundAtAnyDepth(t *testing.T) {
	for _, c := range []struct {
		name string
		src  string
	}{
		{
			name: "a function in an if in a function",
			src: "let main = fn(flag) {\n" +
				"  if (flag) {\n" +
				"    let pad = fn(text, width) { return text; };\n" +
				"    pad(\"a\", 2);\n" +
				"  }\n" +
				"};\n",
		},
		{
			name: "a function passed as an argument",
			src: "let run = fn(f) { return f(); };\n" +
				"run(fn() {\n" +
				"  let pad = fn(text, width) { return text; };\n" +
				"  return pad(\"a\", 2);\n" +
				"});\n",
		},
		{
			name: "a function returned from a function",
			src: "let make = fn() {\n" +
				"  return fn() {\n" +
				"    let pad = fn(text, width) { return text; };\n" +
				"    return pad(\"a\", 2);\n" +
				"  };\n" +
				"};\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			s := New().Analyze(c.src)
			call := shadowingPos(t, c.src, "pad(\"a\"", 0)
			label, _ := shadowingSignature(s, shadowingPos(t, c.src, "pad(\"a\"", len("pad(\"")))
			if !strings.HasPrefix(label, "pad(text") {
				t.Errorf("signature = %q, want pad(text, width)", label)
			}
			if hints := parameterHintsOnLine(s, call.Line); !slices.Equal(hints, []string{"text:", "width:"}) {
				t.Errorf("parameter hints = %v, want [text: width:]", hints)
			}
		})
	}
}

// TestANestedHelperReusingATopLevelNameIsTheOneInScope keeps the index honest
// about which let a call resolves to: it is keyed by the declaring identifier,
// so the nested pad answers inside main and the top-level one outside it.
func TestANestedHelperReusingATopLevelNameIsTheOneInScope(t *testing.T) {
	src := "let pad = fn(a) { return a; };\n" +
		"let main = fn() {\n" +
		"  let pad = fn(text, width) { return text; };\n" +
		"  return pad(\"a\", 2);\n" +
		"};\n" +
		"pad(\"b\");\n"
	s := New().Analyze(src)
	inside, _ := shadowingSignature(s, shadowingPos(t, src, "pad(\"a\"", len("pad(\"")))
	outside, _ := shadowingSignature(s, shadowingPos(t, src, "pad(\"b\"", len("pad(\"")))
	if !strings.HasPrefix(inside, "pad(text, width)") {
		t.Errorf("inside main, signature = %q, want the nested pad(text, width)", inside)
	}
	if !strings.HasPrefix(outside, "pad(a)") {
		t.Errorf("outside main, signature = %q, want the top-level pad(a)", outside)
	}
}

// signatureParameterSpans asks for signature help at pos and returns the label
// with each parameter's offset pair, failing if a parameter is addressed any
// other way.
func signatureParameterSpans(t *testing.T, s *Snapshot, pos lsp.Position) (string, [][2]lsp.UInteger) {
	t.Helper()
	help, ok := s.SignatureHelp(pos)
	if !ok || help == nil || len(help.Signatures) == 0 {
		t.Fatalf("no signature help at %+v", pos)
	}
	sig := help.Signatures[0]
	spans := make([][2]lsp.UInteger, 0, len(sig.Parameters))
	for i, param := range sig.Parameters {
		span, ok := param.Label.([2]lsp.UInteger)
		if !ok {
			t.Fatalf("parameter %d of %q is labelled %#v, want an offset pair", i, sig.Label, param.Label)
		}
		spans = append(spans, span)
	}
	return sig.Label, spans
}

// A parameter is addressed by its offsets in the signature label. A string
// label is found by the client as the first whole-word match of the name, so
// once M26-SEM-019 dropped the solver's kinds that had made each label unique,
// a parameter named like its function -- `let x = fn(x)`, signature x(x) --
// was highlighted on the function's own name.
func TestAParameterNamedLikeItsFunctionIsHighlightedOnItself(t *testing.T) {
	for _, c := range []struct {
		name, src, label string
		spans            [][2]lsp.UInteger
	}{
		{
			name:  "the only parameter",
			src:   "let x = fn(x) { return x; };\nx(1);\n",
			label: "x(x)",
			spans: [][2]lsp.UInteger{{2, 3}},
		},
		{
			name:  "the second parameter",
			src:   "let g = fn(a, g) { return a; };\ng(1, 2);\n",
			label: "g(a, g)",
			spans: [][2]lsp.UInteger{{2, 3}, {5, 6}},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			s := New().Analyze(c.src)
			label, spans := signatureParameterSpans(t, s, lsp.Position{Line: 1, Character: 2})
			if label != c.label || !slices.Equal(spans, c.spans) {
				t.Errorf("signature %q with parameters at %v, want %q with %v", label, spans, c.label, c.spans)
			}
		})
	}
}

// The offsets count UTF-16 code units, which is how LSP 3.16 measures a label,
// and the lexer admits any Unicode letter in a name. In the source below, e
// with an acute accent is two bytes and one unit, and U+1D465 (mathematical
// italic x) is four bytes, one rune and two units, so the pairs differ from a
// byte count ([6,8] [10,14] [16,17]) and from a rune count ([5,6] [8,9] [11,12]).
// Inlay hints read the same offsets from inside the package, where they are
// bytes, and must still name each parameter whole.
func TestSignatureParameterOffsetsAreUTF16Units(t *testing.T) {
	const (
		eAcute  = "\xc3\xa9"
		italicX = "\xf0\x9d\x91\xa5"
	)
	src := "let caf" + eAcute + " = fn(" + eAcute + ", " + italicX + ", b) { return b; };\n" +
		"caf" + eAcute + "(1, 2, 3);\n"
	s := New().Analyze(src)

	// Character 5 is the 1, counted in UTF-16 units as positions are.
	label, spans := signatureParameterSpans(t, s, lsp.Position{Line: 1, Character: 5})
	wantLabel := "caf" + eAcute + "(" + eAcute + ", " + italicX + ", b)"
	wantSpans := [][2]lsp.UInteger{{5, 6}, {8, 10}, {12, 13}}
	if label != wantLabel || !slices.Equal(spans, wantSpans) {
		t.Errorf("signature %q with parameters at %v, want %q with %v", label, spans, wantLabel, wantSpans)
	}

	if hints := parameterHintsOnLine(s, 1); !slices.Equal(hints, []string{eAcute + ":", italicX + ":", "b:"}) {
		t.Errorf("parameter hints = %q, want each parameter's whole name", hints)
	}
}
