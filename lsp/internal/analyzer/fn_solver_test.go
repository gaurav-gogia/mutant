package analyzer

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	mast "mutant/ast"
	localprotocol "mutant/lsp/internal/protocol"

	lsp "github.com/tliron/glsp/protocol_3_16"
)

// cardFor renders the hover card for the first function bound in src.
func cardFor(t *testing.T, src string) string {
	t.Helper()

	s := New().Analyze(src)
	if len(s.ParseErrors) > 0 {
		t.Fatalf("fixture did not parse: %v", s.ParseErrors)
	}
	bindings := functionBindings(s.Program)
	if len(bindings) == 0 {
		t.Fatalf("fixture bound no function:\n%s", src)
	}
	return userFunctionCard(s, bindings[0].name, bindings[0].literal, leadingCommentBlock(src)).render()
}

// leadingCommentBlock returns the source's opening // block, which is what the
// hover path hands the card.
func leadingCommentBlock(src string) string {
	var out []string
	for _, line := range strings.Split(src, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "//") {
			break
		}
		out = append(out, strings.TrimSpace(strings.TrimPrefix(trimmed, "//")))
	}
	return strings.Join(out, "\n")
}

// TestSolverNarrowsParametersFromBuiltinContracts is the solver's whole reason
// for existing. Mutant has no type annotations, so what a card can say about a
// parameter can only come from what the body does with it — and the richest
// source of that is the 607 verified parameter contracts in builtin/metadata.go.
// The card says it as a use, "used as STRING", and keeps it off the signature
// line, because it is not a type a caller is held to (M26-SEM-019).
func TestSolverNarrowsParametersFromBuiltinContracts(t *testing.T) {
	tests := []struct {
		name   string
		src    string
		want   []string
		absent []string
	}{
		{
			name: "a chain of string builtins",
			src:  "let norm = fn(host) { return str_lower(str_trim(host)); };",
			want: []string{"function `norm(host) -> STRING`", "- `host` _(used as STRING)_"},
		},
		{
			name: "a pair builtin destructured, then consumed",
			src:  `let lines = fn(path) { let data, err = fs_read(path); return text_split(data, "x"); };`,
			want: []string{"function `lines(path) -> ARRAY of STRING`", "- `path` _(used as STRING)_"},
		},
		{
			name:   "only the parameters the body actually uses",
			src:    "let digest = fn(payload, algo) { return hash_sha256(payload); };",
			want:   []string{"function `digest(payload, algo) -> STRING`", "- `payload` _(used as STRING|BYTES)_"},
			absent: []string{"`algo` _(used as"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			card := cardFor(t, test.src)
			for _, want := range test.want {
				if !strings.Contains(card, want) {
					t.Errorf("card does not contain %q:\n%s", want, card)
				}
			}
			for _, absent := range test.absent {
				if strings.Contains(card, absent) {
					t.Errorf("card contains %q:\n%s", absent, card)
				}
			}
		})
	}
}

// TestSolverUsesTheVMsOperatorDomains checks the operator constraints match what
// the VM actually accepts, rather than what an arithmetic operator "obviously"
// takes. execBinaryStringOperation rejects every opcode but OpAdd, so `+` admits
// strings and `*` does not — a distinction worth getting right, because claiming
// `a` in `a + b` is numeric would be a type the program does not have. The same
// holds for buffers: execBinaryBytesOperation concatenates two with OpAdd.
func TestSolverUsesTheVMsOperatorDomains(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want []string
	}{
		{
			name: "addition admits strings",
			src:  "let join2 = fn(a, b) { return a + b; };",
			want: []string{"- `a` _(used as STRING|BYTES|INTEGER|FLOAT)_", "- `b` _(used as STRING|BYTES|INTEGER|FLOAT)_"},
		},
		{
			name: "multiplication does not",
			src:  "let scale = fn(n, factor) { return n * factor; };",
			want: []string{"- `n` _(used as INTEGER|FLOAT)_", "- `factor` _(used as INTEGER|FLOAT)_"},
		},
		{
			name: "ordering comparison is numeric",
			src:  "let over = fn(n) { if (n > 10) { return true; }; return false; };",
			want: []string{"- `n` _(used as INTEGER|FLOAT)_"},
		},
		{
			name: "negation is numeric",
			src:  "let flip = fn(n) { return -n; };",
			want: []string{"- `n` _(used as INTEGER|FLOAT)_"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			card := cardFor(t, test.src)
			for _, want := range test.want {
				if !strings.Contains(card, want) {
					t.Errorf("card does not contain %q:\n%s", want, card)
				}
			}
		})
	}
}

// TestSolverPropagatesThroughUserFunctions covers the fixpoint. A parameter
// handed straight to another function has whatever type that function demands,
// which cannot be worked out in a single pass over one body.
func TestSolverPropagatesThroughUserFunctions(t *testing.T) {
	src := "let inner = fn(s) { return str_upper(s); };\n" +
		"let outer = fn(v) { return inner(v); };\n"

	s := New().Analyze(src)
	bindings := functionBindings(s.Program)
	if len(bindings) != 2 {
		t.Fatalf("expected two function bindings, got %d", len(bindings))
	}

	var outer *mast.FunctionLiteral
	for _, b := range bindings {
		if b.name == "outer" {
			outer = b.literal
		}
	}
	if outer == nil {
		t.Fatal("outer was not collected")
	}

	card := userFunctionCard(s, "outer", outer, "").render()
	if !strings.Contains(card, "- `v` _(used as STRING)_") {
		t.Errorf("the constraint did not propagate from inner to outer:\n%s", card)
	}
}

// TestSolverLeavesUnconstrainedParametersBare is the restraint half. A parameter
// the body never constrains has no type to show, and inventing one — from a call
// site, from a name that looks like a path — is the single way this feature could
// mislead rather than inform.
func TestSolverLeavesUnconstrainedParametersBare(t *testing.T) {
	card := cardFor(t, "let passthrough = fn(value) { return value; };\npassthrough(\"text\");\n")

	if !strings.Contains(card, "passthrough(value)") {
		t.Errorf("an unconstrained parameter reached the signature line:\n%s", card)
	}
	// The call-site evidence is still worth showing, but only as an observation
	// on the bullet, never as the parameter's type.
	if !strings.Contains(card, "- `value` · `STRING` _(observed at call sites)_") {
		t.Errorf("call-site evidence is missing from the bullet:\n%s", card)
	}
}

// TestSolverIgnoresNestedFunctionParameters guards a shadow the solver used to
// survive only by not looking inside nested functions, when it matched names.
// The inner `fn(host)` declares a host of its own, so the str_upper call inside
// it resolves to the inner parameter and says nothing about the outer one.
// fn_solver_scope_test.go holds the shadows that not looking did not cover.
func TestSolverIgnoresNestedFunctionParameters(t *testing.T) {
	src := "let outer = fn(host) { let inner = fn(host) { return str_upper(host); }; return inner(1); };"
	card := cardFor(t, src)

	if strings.Contains(card, "`host` _(used as") {
		t.Errorf("a shadowed parameter's constraint leaked to the outer function:\n%s", card)
	}
}

// TestSolverShowsNothingForContradictoryConstraints covers the empty set. A
// parameter both negated and passed to str_upper is used in two ways no single
// kind satisfies, so there is no kind to name, and the honest response is to say
// nothing — reporting the contradiction is the diagnostics' job, and the solver
// is a description that is not allowed to fail anything.
func TestSolverShowsNothingForContradictoryConstraints(t *testing.T) {
	card := cardFor(t, "let broken = fn(v) { let a = -v; return str_upper(v); };")

	if strings.Contains(card, "`v` _(used as") {
		t.Errorf("an impossible parameter was given a type:\n%s", card)
	}
}

// TestFunctionCardRendersDocTags checks the @param/@returns convention reaches
// the card, so a documented user function reads exactly like a builtin.
func TestFunctionCardRendersDocTags(t *testing.T) {
	src := "// Normalises a hostname for comparison.\n" +
		"// @param host — the raw hostname, with or without a scheme\n" +
		"// @returns the lower-cased, trimmed host\n" +
		"let norm = fn(host) { return str_lower(str_trim(host)); };\n"

	card := cardFor(t, src)
	for _, want := range []string{
		"Normalises a hostname for comparison.",
		"- `host` _(used as STRING)_ — the raw hostname, with or without a scheme",
		"- `STRING` _(inferred)_ — the lower-cased, trimmed host",
	} {
		if !strings.Contains(card, want) {
			t.Errorf("card is missing %q:\n%s", want, card)
		}
	}
	// A tag line must not also appear in the description.
	if strings.Contains(card, "@param") || strings.Contains(card, "@returns") {
		t.Errorf("tag lines leaked into the description:\n%s", card)
	}
}

// TestUntaggedDocCommentIsUnchanged is the compatibility check. Every doc comment
// in the corpus predates the tags, so adding them must leave those cards exactly
// as they were.
func TestUntaggedDocCommentIsUnchanged(t *testing.T) {
	card := cardFor(t, "// Adds two numbers together\nlet add = fn(a, b) { a + b; };\n")

	if !strings.Contains(card, "Adds two numbers together") {
		t.Errorf("an untagged doc comment was lost:\n%s", card)
	}
}

// TestFunctionCardMarksAnUninferredReturn keeps a user function's unknown apart
// from a builtin's verified ANY. Both render as ANY; only the note says whether
// that is a fact about the callable or a limit of the editor.
func TestFunctionCardMarksAnUninferredReturn(t *testing.T) {
	card := cardFor(t, "let opaque = fn(v) { return mystery_call(v); };")

	if !strings.Contains(card, "- `ANY` _(not inferred)_") {
		t.Errorf("an uninferred return did not say so:\n%s", card)
	}
}

// TestFunctionCardHasTheSameSectionsAsABuiltinCard is the requirement stated
// plainly: hovering a user function and hovering a builtin should teach a reader
// the same things in the same places.
func TestFunctionCardHasTheSameSectionsAsABuiltinCard(t *testing.T) {
	userCard := cardFor(t, "let norm = fn(host) { return str_lower(host); };")
	builtinCard, ok := builtinHoverText("str_lower")
	if !ok {
		t.Fatal("str_lower has no hover card")
	}

	for _, section := range []string{"**Parameters**", "**Returns**"} {
		if !strings.Contains(userCard, section) {
			t.Errorf("user function card is missing %q:\n%s", section, userCard)
		}
		if !strings.Contains(builtinCard, section) {
			t.Errorf("builtin card is missing %q:\n%s", section, builtinCard)
		}
	}

	// A zero-parameter function keeps the heading, exactly as a zero-parameter
	// builtin does.
	empty := cardFor(t, "let now = fn() { return 1; };")
	if !strings.Contains(empty, "**Parameters**\n- _none_") {
		t.Errorf("zero-parameter function dropped the section:\n%s", empty)
	}
}

// chainFixture is a chain of functions each passing its parameter to the next,
// ending at a builtin that demands a string. Each link is declared inside the
// one that calls it, and the solver visits them outer-first, which is source
// order, so constraints have to travel back out through every link, one round
// apiece.
//
// It used to be written caller-first at the top level, which is how a reader
// might sketch one and which the compiler refuses: f1 is not declared yet where
// f0 calls it, `undefined variable: f1`. The solver resolves names the way the
// compiler does now, so that shape carries no constraint at all, and a chain
// that compiles is the only kind worth pinning.
func chainFixture(links int) string {
	var src strings.Builder
	for i := 0; i < links-1; i++ {
		fmt.Fprintf(&src, "let f%d = fn(x) {\n", i)
	}
	fmt.Fprintf(&src, "let f%d = fn(x) { return str_upper(x); };\n", links-1)
	for i := links - 2; i >= 0; i-- {
		fmt.Fprintf(&src, "return f%d(x);\n};\n", i+1)
	}
	src.WriteString("f0(\"a\");\n")
	return src.String()
}

// The same document asked the same question has to give the same answer. The
// solver iterated the map it was filling, and Go randomises that order, so the
// direction a constraint travelled changed between runs: on a 14-function chain
// the answer came out `f0(x)` on 39 of 40 fresh analyses and `f0(x: STRING)` on
// one. Hover and signature help are read between keystrokes, and a card that
// changes when nothing in the file did reads as the editor guessing.
//
// Both halves are asserted, because either alone would pass for the wrong
// reason: forty runs agreeing on a stalled answer is still a stalled answer,
// and one run reaching STRING says nothing about the next.
func TestTheSolverGivesOneAnswerForOneDocument(t *testing.T) {
	fixture := chainFixture(14)

	seen := map[string]int{}
	for run := 0; run < 40; run++ {
		seen[cardFor(t, fixture)]++
	}

	if len(seen) != 1 {
		var answers []string
		for card, count := range seen {
			answers = append(answers, fmt.Sprintf("%dx %s", count, firstLine(card)))
		}
		sort.Strings(answers)
		t.Fatalf("40 analyses of one document gave %d different cards: %s",
			len(seen), strings.Join(answers, " | "))
	}

	for card := range seen {
		if !strings.Contains(card, "- `x` _(used as STRING)_") {
			t.Fatalf("the chain's constraint never reached f0; every run said:\n%s", card)
		}
	}
}

// A chain longer than any fixed round count, to pin that the bound is computed
// from the document and not written down. Thirty-two links is four times the
// cap this used to carry.
func TestALongChainStillReachesItsFixpoint(t *testing.T) {
	card := cardFor(t, chainFixture(32))
	if !strings.Contains(card, "- `x` _(used as STRING)_") {
		t.Fatalf("a 32-link chain did not reach f0:\n%s", card)
	}
}

func firstLine(s string) string {
	if at := strings.IndexByte(s, '\n'); at >= 0 {
		return s[:at]
	}
	return s
}

// TestASolvedKindIsNotAnInferredType pins the display half of M26-SEM-003: a
// usage deduction is not an inferred type, so it reaches no type hint and no
// hover. The program is one where the deduction is false of the call:
// low_byte("abc") returns "abc" through its guarded branch.
func TestASolvedKindIsNotAnInferredType(t *testing.T) {
	guarded := "    if (type_of(v) == \"STRING\") { return v; }\n"
	src := "let low_byte = fn(v) {\n" +
		guarded +
		"    return v & 255;\n" +
		"};\n" +
		"let s = low_byte(\"abc\");\n"
	s := New().Analyze(src)

	for _, h := range s.InlayHints(wideRange()) {
		if h.Position.Line == 4 && h.Kind != nil && *h.Kind == localprotocol.InlayHintKindType {
			t.Errorf("let s carries the type hint %q, deduced from how low_byte uses v", h.Label)
		}
	}
	// v in the guarded return on line 1, and s on line 4.
	returnedV := lsp.UInteger(strings.Index(guarded, "return v") + len("return "))
	for _, at := range []lsp.Position{
		{Line: 1, Character: returnedV},
		{Line: 4, Character: 4},
	} {
		text, _, _ := s.HoverText(at)
		if strings.Contains(text, ": int") {
			t.Errorf("hover at %d:%d gives a type the program does not have: %q", at.Line, at.Character, text)
		}
	}
}
