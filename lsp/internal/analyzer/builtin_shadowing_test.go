package analyzer

import (
	"slices"
	"strings"
	"testing"

	lsp "github.com/tliron/glsp/protocol_3_16"
)

// A binding of a builtin's name is what the VM calls, from the point it is
// bound: `let max = fn(first, second, third) {...}; max(1, 2, 3);` runs the
// author's max and prints 1. Signature help, completion, inlay hints and hover
// all described the builtin instead, because each asked the builtin table
// before it asked the scope (M26-SEM-011). The scope question is the compiler's
// own, isBoundAt, so a struct or enum of the same name and a let further down
// leave the builtin in place, exactly as they do in the build.

const shadowingMaxSource = "let max = fn(first, second, third) {\n" +
	"  return first;\n" +
	"};\n" +
	"max(1, 2, 3);\n"

// shadowingPos is the position of needle's first occurrence in an ASCII
// source, moved right by shift characters.
func shadowingPos(t *testing.T, src, needle string, shift int) lsp.Position {
	t.Helper()
	offset := strings.Index(src, needle)
	if offset < 0 {
		t.Fatalf("%q does not occur in the source", needle)
	}
	line := strings.Count(src[:offset], "\n")
	lineStart := strings.LastIndex(src[:offset], "\n") + 1
	return lsp.Position{Line: lsp.UInteger(line), Character: lsp.UInteger(offset - lineStart + shift)}
}

func shadowingSignature(s *Snapshot, pos lsp.Position) (string, bool) {
	help, ok := s.SignatureHelp(pos)
	if !ok || help == nil || len(help.Signatures) == 0 {
		return "", false
	}
	return help.Signatures[0].Label, true
}

func TestAUserFunctionShadowingABuiltinIsTheOneDescribed(t *testing.T) {
	s := New().Analyze(shadowingMaxSource)
	label, ok := shadowingSignature(s, shadowingPos(t, shadowingMaxSource, "max(1", len("max(")))
	if !ok || !strings.HasPrefix(label, "max(first, second, third)") {
		t.Errorf("signature help on the author's max = %q (ok=%t), want max(first, second, third)", label, ok)
	}
	if got := parameterHintsOnLine(s, 3); !slices.Equal(got, []string{"first:", "second:", "third:"}) {
		t.Errorf("parameter hints on the author's max = %v, want [first: second: third:]", got)
	}
}

func TestCompletionOffersTheBindingThatShadowsABuiltin(t *testing.T) {
	s := New().Analyze(shadowingMaxSource)
	details := func(pos lsp.Position) []string {
		var got []string
		for _, item := range s.CompletionItemsAt(pos) {
			if item.Label != "max" {
				continue
			}
			detail := ""
			if item.Detail != nil {
				detail = *item.Detail
			}
			got = append(got, detail)
		}
		return got
	}

	below := details(lsp.Position{Line: 3, Character: 0})
	if len(below) != 1 {
		t.Fatalf("below the let, completion offers %d items labelled max %v, want exactly 1", len(below), below)
	}
	if strings.HasPrefix(below[0], "builtin") {
		t.Errorf("below the let, the one max offered is the builtin (%q); the VM calls the author's", below[0])
	}

	above := details(lsp.Position{Line: 0, Character: 0})
	if len(above) != 1 || !strings.HasPrefix(above[0], "builtin") {
		t.Errorf("above the let nothing shadows max yet, so the builtin is offered; got %v", above)
	}
}

// TestBuiltinShadowingFollowsTheCompiler pins the cases where the build calls
// the builtin although the file mentions the name, and the one where a
// parameter shadows it. Each was decided by running the compiled program.
func TestBuiltinShadowingFollowsTheCompiler(t *testing.T) {
	for _, c := range []struct {
		name        string
		src         string
		wantBuiltin bool // true: the builtin's signature; false: no signature at all
	}{
		{
			name:        "a use above a later let",
			src:         "let main = fn() { return max(1, 2, 3); };\nlet max = fn(a, b, c) { return a; };\n",
			wantBuiltin: true,
		},
		{
			name:        "a struct of the same name",
			src:         "struct max { a }\nmax(1, 2, 3);\n",
			wantBuiltin: true,
		},
		{
			name:        "a parameter of the same name",
			src:         "let apply = fn(max) { return max(1, 2, 3); };\n",
			wantBuiltin: false,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			s := New().Analyze(c.src)
			label, ok := shadowingSignature(s, shadowingPos(t, c.src, "max(1", len("max(")))
			isBuiltin := ok && strings.HasPrefix(label, "max(value")
			if c.wantBuiltin && !isBuiltin {
				t.Errorf("signature = %q (ok=%t), want the builtin's: the compiler calls the builtin here", label, ok)
			}
			if !c.wantBuiltin && ok {
				t.Errorf("signature = %q, want none: the parameter is what is called, and its signature is unknown", label)
			}
		})
	}
}

func TestHoverOnAValueNamedLikeABuiltinIsNotTheBuiltin(t *testing.T) {
	src := "let first = 1;\nfirst;\n"
	s := New().Analyze(src)
	text, _, ok := s.HoverText(lsp.Position{Line: 1, Character: 1})
	if !ok {
		t.Fatalf("no hover on a bound name")
	}
	if builtinText, isBuiltin := builtinHoverText("first"); isBuiltin && text == builtinText {
		t.Fatalf("hover on the author's first is the builtin's card:\n%s", text)
	}
	if !strings.Contains(text, "identifier `first`") {
		t.Errorf("hover on the author's first = %q, want it described as the identifier it is", text)
	}
}

// shadowingHover is the hover text at needle's first occurrence, one character
// into it: inside the identifier, clear of the boundary at either end.
func shadowingHover(t *testing.T, src, needle string) string {
	t.Helper()
	s := New().Analyze(src)
	if len(s.ParseErrors) != 0 {
		t.Fatalf("the fixture does not parse: %v", s.ParseErrors[0])
	}
	text, _, ok := s.HoverText(shadowingPos(t, src, needle, 1))
	if !ok {
		t.Fatalf("no hover at %q", needle)
	}
	return text
}

// TestHoverOnTheNameADeclarationWritesIsThatDeclaration pins hover on the
// identifier a declaration writes. A value let, a let of several names and a
// for-in binding are not visible at their own name, because the compiler
// defines them only after compiling the value or the iterable (M26-SEM-021,
// M26-SEM-007). A hover that asked the scope alone therefore described the
// builtin sum or keys at the very name being declared, while go-to-definition
// from the same position landed on the declaration (M26-SEM-011). A struct or
// enum named like a builtin is a declaration too and hovers as its own card,
// and a value is described as the value it is even when a struct, or a macro
// special form, shares its spelling.
func TestHoverOnTheNameADeclarationWritesIsThatDeclaration(t *testing.T) {
	for _, c := range []struct {
		name   string
		src    string
		needle string // its first occurrence is the declaring identifier
		want   string // what the hover starts with
	}{
		{"a value let", "let sum = 0;\nsum = sum + 1;\n", "sum", "identifier `sum`"},
		{"a let of several names", "let keys, err = [1, 2];\nputln(keys, err);\n", "keys", "identifier `keys`"},
		{"a for-in binding", "for (keys in [1, 2]) { putln(keys); }\n", "keys", "identifier `keys`"},
		{"a let whose value calls the builtin it shadows", "let h = {\"a\": 1};\nlet keys = keys(h);\nputln(keys);\n", "keys", "identifier `keys`"},
		{"a struct named like a builtin", "struct max { a }\n", "max", "struct `max`\n\n**Fields**\n- `a`"},
		{"an enum named like a builtin", "enum max { a }\n", "max", "enum `max`\n\n**Variants**\n- `a`"},
		{"a value named like a builtin and a struct", "struct sum { a }\nlet sum = 0;\nputln(sum);\n", "sum = 0", "identifier `sum`"},
		{"a value named like a macro special form", "let quote = 1;\nputln(quote);\n", "quote", "identifier `quote`"},
	} {
		t.Run(c.name, func(t *testing.T) {
			text := shadowingHover(t, c.src, c.needle)
			name := strings.Fields(c.needle)[0]
			if card, isBuiltin := builtinHoverText(name); isBuiltin && text == card {
				t.Fatalf("hover on the declaring %s is the builtin's card: %q", name, strings.SplitN(text, "\n", 2)[0])
			}
			if !strings.HasPrefix(text, c.want) {
				t.Errorf("hover on the declaring %s = %q, want it to start with %q", name, strings.SplitN(text, "\n", 2)[0], c.want)
			}
		})
	}
}

// TestHoverInsideALetsOwnValueIsStillTheBuiltin is the other side of the rule
// above. The value of a let is compiled before the let's names are defined
// (M26-SEM-021), and a for-in's iterable before its bindings (M26-SEM-007), so
// a builtin's name used there is the builtin, and its card is the true answer.
// Neither use declares anything, so the declaration rule must not reach it.
func TestHoverInsideALetsOwnValueIsStillTheBuiltin(t *testing.T) {
	for _, c := range []struct {
		name    string
		src     string
		needle  string // its first occurrence is the use inside the value
		builtin string
	}{
		{"a let's own value", "let sum = sum([1]);\nputln(sum);\n", "sum([1", "sum"},
		{"a for-in's iterable", "for (keys in keys({\"a\": 1})) { putln(keys); }\n", "keys({", "keys"},
	} {
		t.Run(c.name, func(t *testing.T) {
			card, ok := builtinHoverText(c.builtin)
			if !ok {
				t.Fatalf("%s is not a builtin, so this case tests nothing", c.builtin)
			}
			if text := shadowingHover(t, c.src, c.needle); text != card {
				t.Errorf("hover on the %s the compiler reads as the builtin = %q, want the builtin's card", c.builtin, strings.SplitN(text, "\n", 2)[0])
			}
		})
	}
}

// TestHoverOnDeclarationsThatHadTheirOwnCardKeepsThem pins the declarations
// named like a builtin whose hover was already right before the rule above: a
// parameter is visible from its own name, and a function literal's let, a
// struct field and an enum variant each have a card of their own that is asked
// for first.
func TestHoverOnDeclarationsThatHadTheirOwnCardKeepsThem(t *testing.T) {
	for _, c := range []struct {
		name   string
		src    string
		needle string
		want   string
	}{
		{"a parameter", "let f = fn(sum) { return sum; };\n", "sum", "identifier `sum`"},
		{"a parameter's use", "let f = fn(sum) { return sum; };\n", "sum; }", "identifier `sum`"},
		{"a function literal's let", "let sum = fn(a) { return a; };\nsum(1);\n", "sum", "function `sum(a)`"},
		{"a struct field", "struct P { sum }\nlet p = P{sum: 1};\n", "sum", "field `sum`"},
		{"an enum variant", "enum E { max }\n", "max", "enum member `max`"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if text := shadowingHover(t, c.src, c.needle); !strings.HasPrefix(text, c.want) {
				t.Errorf("hover at %q = %q, want it to start with %q", c.needle, strings.SplitN(text, "\n", 2)[0], c.want)
			}
		})
	}
}
