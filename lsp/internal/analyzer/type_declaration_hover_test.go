package analyzer

import (
	"strings"
	"testing"

	lsp "github.com/tliron/glsp/protocol_3_16"
)

// The name a struct or enum declaration writes hovers as that declaration. The
// type cards find a type by its name alone -- the first top-level struct or enum
// of that name, every literal of that name, the last type declared under it --
// so for a declaration that shares its name with another of its kind, or that
// sits inside a function where only top-level statements are looked at, a card
// asked for by name describes another declaration or nothing. The hover then
// fell through to whatever else had the spelling: a builtin, the quote and
// unquote macro special forms, or the struct of the same name.

// typeDeclarationHover is the hover text one character into needle's first
// occurrence, in an ASCII source that parses.
func typeDeclarationHover(t *testing.T, src, needle string) string {
	t.Helper()
	s := New().Analyze(src)
	if len(s.ParseErrors) != 0 {
		t.Fatalf("the fixture does not parse: %v", s.ParseErrors[0])
	}
	offset := strings.Index(src, needle)
	if offset < 0 {
		t.Fatalf("%q does not occur in the source", needle)
	}
	line := strings.Count(src[:offset], "\n")
	column := offset - (strings.LastIndex(src[:offset], "\n") + 1) + 1
	text, _, ok := s.HoverText(lsp.Position{Line: lsp.UInteger(line), Character: lsp.UInteger(column)})
	if !ok {
		t.Fatalf("no hover at %q", needle)
	}
	return text
}

// TestHoverOnATypeDeclarationIsThatDeclarationWhereItsNameIsShared pins the
// declarations whose own card the name lookup cannot find. Each is described
// by its kind and its name, and by nothing that belongs to something else of
// that name: not a builtin's or a macro special form's card, not the fields of
// another struct, and not a count of the literals that build another struct.
func TestHoverOnATypeDeclarationIsThatDeclarationWhereItsNameIsShared(t *testing.T) {
	for _, c := range []struct {
		name    string
		src     string
		needle  string   // its first occurrence starts the declaring name
		want    string   // the hover's first line
		with    []string // what this declaration's own card says
		without []string // what a card for something else of the name says
	}{
		{
			name:    "a struct named quote inside a function",
			src:     "let f = fn() { struct quote { a } return 1; };\nputln(f());\n",
			needle:  "quote",
			want:    "struct `quote`",
			without: []string{"macro special form"},
		},
		{
			name:    "an enum named unquote inside a function",
			src:     "let f = fn() { enum unquote { a } return 1; };\nputln(f());\n",
			needle:  "unquote",
			want:    "enum `unquote`",
			without: []string{"macro special form"},
		},
		{
			name:    "a struct named like a builtin inside a function",
			src:     "let f = fn() { struct max { a } return max{a: 1}; };\nputln(f());\n",
			needle:  "max",
			want:    "struct `max`",
			without: []string{"builtin"},
		},
		{
			name:    "an enum named like a builtin inside a function",
			src:     "let f = fn() { enum max { a } return max.a; };\nputln(f());\n",
			needle:  "max",
			want:    "enum `max`",
			without: []string{"builtin"},
		},
		{
			// Structs are looked up first, so the enum's name found the struct.
			name:    "an enum that shares a struct's name",
			src:     "struct Color { a }\nenum Color { b }\nputln(Color.b);\n",
			needle:  "Color { b",
			want:    "enum `Color`",
			with:    []string{"- `b` \xc2\xb7 `0`"},
			without: []string{"struct", "- `a`"},
		},
		{
			name:    "an enum that shares a builtin-named struct's name",
			src:     "struct max { a }\nenum max { b }\nputln(max.b);\n",
			needle:  "max { b",
			want:    "enum `max`",
			with:    []string{"- `b` \xc2\xb7 `0`"},
			without: []string{"builtin", "struct", "- `a`"},
		},
		{
			// The fields came from the first struct P, whatever P was hovered.
			name:    "the second of two structs of one name",
			src:     "struct P { a }\nstruct P { b }\nlet p = P{b: 1};\n",
			needle:  "P { b",
			want:    "struct `P`",
			without: []string{"- `a`"},
		},
		{
			// The count came from the last struct P, whatever P was hovered:
			// the one literal builds the second, and nothing builds this one.
			name:    "the first of two structs of one name",
			src:     "struct P { a }\nstruct P { b }\nlet p = P{b: 1};\n",
			needle:  "P { a",
			want:    "struct `P`",
			without: []string{"Built once"},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			text := typeDeclarationHover(t, c.src, c.needle)
			name := strings.Fields(c.needle)[0]
			if first := strings.SplitN(text, "\n", 2)[0]; first != c.want {
				t.Errorf("hover on the declaring %s begins %q, want %q", name, first, c.want)
			}
			for _, want := range c.with {
				if !strings.Contains(text, want) {
					t.Errorf("hover on the declaring %s does not say %q:\n%s", name, want, text)
				}
			}
			for _, unwanted := range c.without {
				if strings.Contains(text, unwanted) {
					t.Errorf("hover on the declaring %s says %q, which belongs to something else of its name:\n%s", name, unwanted, text)
				}
			}
		})
	}
}

// TestHoverOnATypeDeclaredOnceAtTheTopLevelKeepsItsCard is the other side of
// the rule above, and covers nearly every type a program declares: where the
// name is the declaration's own, the declaration keeps its full card, which is
// the card a use of the name reaches too. Only a declaration the lookup could
// mistake for another is reduced to its kind and name.
func TestHoverOnATypeDeclaredOnceAtTheTopLevelKeepsItsCard(t *testing.T) {
	for _, c := range []struct {
		name string
		src  string
		decl string // its first occurrence starts the declaring name
		use  string // its first occurrence starts a use of the same type
		with []string
	}{
		{
			name: "a struct",
			src:  "struct Point { x; y; }\nlet p = Point{x: 1, y: \"a\"};\n",
			decl: "Point {",
			use:  "Point{",
			with: []string{
				"struct `Point`",
				"- `x` \xc2\xb7 `INTEGER` _(inferred)_",
				"- `y` \xc2\xb7 `STRING` _(inferred)_",
				"_Built once in this file._",
			},
		},
		{
			name: "an enum",
			src:  "enum Severity { Low, High }\nlet level = Severity.High;\n",
			decl: "Severity {",
			use:  "Severity.High",
			with: []string{
				"enum `Severity`",
				"- `Low` \xc2\xb7 `0`",
				"- `High` \xc2\xb7 `1`",
			},
		},
		{
			// One struct and one enum of a name: neither is a second of its
			// kind, so the struct's card is still its own.
			name: "a struct that shares its name with an enum",
			src:  "struct Color { a }\nenum Color { b }\nlet c = Color{a: 1};\n",
			decl: "Color { a",
			use:  "Color{",
			with: []string{
				"struct `Color`",
				"- `a` \xc2\xb7 `INTEGER` _(inferred)_",
			},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			declared := typeDeclarationHover(t, c.src, c.decl)
			for _, want := range c.with {
				if !strings.Contains(declared, want) {
					t.Errorf("hover on the declaration does not say %q:\n%s", want, declared)
				}
			}
			if used := typeDeclarationHover(t, c.src, c.use); used != declared {
				t.Errorf("hover on the declaration and on a use differ:\n%s\n---\n%s", declared, used)
			}
		})
	}
}
