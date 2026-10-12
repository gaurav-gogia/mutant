package analyzer

import (
	"slices"
	"testing"

	localprotocol "mutant/lsp/internal/protocol"

	lsp "github.com/tliron/glsp/protocol_3_16"
)

func wideRange() lsp.Range {
	return lsp.Range{Start: lsp.Position{Line: 0, Character: 0}, End: lsp.Position{Line: 1000, Character: 0}}
}

func TestInlayHintsUserFunctionParameters(t *testing.T) {
	src := "let add = fn(left, right) { return left + right; };\n" +
		"add(1, 2);\n"
	s := New().Analyze(src)
	hints := s.InlayHints(wideRange())

	labels := make([]string, 0, len(hints))
	for _, h := range hints {
		labels = append(labels, h.Label)
	}
	if !containsLabel(labels, "left:") || !containsLabel(labels, "right:") {
		t.Fatalf("expected left:/right: parameter hints, got %v", labels)
	}
}

func TestInlayHintsBuiltinParameters(t *testing.T) {
	// push(array, value) is a documented builtin with named params. This test
	// used to accept any hint with a Kind, and the `: []int` TYPE hint on xs
	// satisfied it while no builtin call had a parameter hint (M26-SEM-009).
	src := "let xs = [1, 2];\npush(xs, 3);\n"
	s := New().Analyze(src)
	if got := parameterHintsOnLine(s, 1); !slices.Equal(got, []string{"array:", "value:"}) {
		t.Fatalf("push(xs, 3) parameter hints = %v, want [array: value:]", got)
	}
}

// TestInlayHintsBuiltinParameterNames pins how a builtin parameter's name is
// read out of its typed label: the optional marker dropped, the variadic one
// kept, and the dotted spelling answered like the flat one.
func TestInlayHintsBuiltinParameterNames(t *testing.T) {
	for _, c := range []struct {
		name string
		src  string
		line lsp.UInteger
		want []string
	}{
		{"two plain parameters", "str_repeat(\"ab\", 3);\n", 0, []string{"s:", "n:"}},
		{"an optional parameter", "range(0, 10, 2);\n", 0, []string{"start:", "end:", "step:"}},
		{"a variadic tail", "max(1, 2, 3);\n", 0, []string{"value:", "...values:"}},
		{"the dotted spelling", "str.repeat(\"ab\", 3);\n", 0, []string{"s:", "n:"}},
		{"an argument already named like its parameter", "let n = 3;\nstr_repeat(\"ab\", n);\n", 1, []string{"s:"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			s := New().Analyze(c.src)
			if got := parameterHintsOnLine(s, c.line); !slices.Equal(got, c.want) {
				t.Errorf("parameter hints = %v, want %v\n%s", got, c.want, c.src)
			}
		})
	}
}

// TestInlayHintsMethodCallOnAParameterIsNotABuiltin is the false hint that
// reading builtin labels would otherwise have produced. InlayHints asks for each
// call's signature with the zero position, where a parameter named str is not
// yet bound, so `str.repeat(3)` inside the function used to read as str_repeat.
func TestInlayHintsMethodCallOnAParameterIsNotABuiltin(t *testing.T) {
	src := "let f = fn(str) {\n  return str.repeat(3);\n};\n"
	s := New().Analyze(src)
	if got := parameterHintsOnLine(s, 1); len(got) != 0 {
		t.Fatalf("parameter hints on a method-style call through a parameter = %v, want none", got)
	}
}

// parameterHintsOnLine is every PARAMETER-kind hint label on one line, in
// order. A type hint does not count, which is what the old builtin test missed.
func parameterHintsOnLine(s *Snapshot, line lsp.UInteger) []string {
	var labels []string
	for _, h := range s.InlayHints(wideRange()) {
		if h.Position.Line != line || h.Kind == nil || *h.Kind != localprotocol.InlayHintKindParameter {
			continue
		}
		labels = append(labels, h.Label)
	}
	return labels
}

func TestInlayHintsRangeFiltering(t *testing.T) {
	src := "let f = fn(a) { return a; };\n" + // line 0
		"f(1);\n" + // line 1
		"f(2);\n" // line 2
	s := New().Analyze(src)

	// Only line 1 requested: hints from the line-2 call must be excluded.
	only := s.InlayHints(lsp.Range{
		Start: lsp.Position{Line: 1, Character: 0},
		End:   lsp.Position{Line: 1, Character: 100},
	})
	for _, h := range only {
		if h.Position.Line != 1 {
			t.Fatalf("hint outside requested range: line %d", h.Position.Line)
		}
	}
	if len(only) == 0 {
		t.Fatal("expected at least one hint on line 1")
	}
}

func TestInlayHintsSuppressSameNameArgument(t *testing.T) {
	// The argument identifier equals the parameter name -> no hint (noise).
	src := "let g = fn(value) { return value; };\nlet value = 1;\ng(value);\n"
	s := New().Analyze(src)
	hints := s.InlayHints(wideRange())
	for _, h := range hints {
		if h.Label == "value:" {
			t.Fatalf("expected the same-name argument hint to be suppressed, got %+v", hints)
		}
	}
}

func containsLabel(labels []string, want string) bool {
	for _, l := range labels {
		if l == want {
			return true
		}
	}
	return false
}
