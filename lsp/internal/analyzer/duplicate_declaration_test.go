package analyzer

// The editor's redeclaration refusal, held against the compiler over programs
// nobody wrote by hand.
//
// The rule this replaced judged a let name by name, and a table of hand-written
// rows had kept it looking right while it was wrong in both directions. At
// 2c5f24f, over a seeded run of 3000 programs like the ones below, it warned
// about 361 of the 645 programs the compiler accepts, and it answered every
// refused one in its own words, as a Warning (M26-LSP-029). The refusal now
// follows the compiler's rule -- a let is refused when nothing on its left is
// new to its scope, a blank counting as new -- and this generator is what keeps
// the two rules one rule: it writes every kind of scope the compiler opens
// (blocks, counted and for-in loops, while, match arms, functions with their
// parameters), and its names collide often.
//
// The seed is fixed, so every run writes the same programs and a failure names
// one that can be pasted into an editor.

import (
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"testing"

	lsp "github.com/tliron/glsp/protocol_3_16"
)

// declarationProgram writes a random program made of declarations in nested
// scopes. The names are few on purpose: four names over a handful of scopes
// redeclare something in most programs, and the blank is among them because it
// is the case a name-by-name rule gets wrong.
type declarationProgram struct {
	r     *rand.Rand
	sb    strings.Builder
	depth int
}

var declarationNames = []string{"a", "b", "err", "_"}

func (g *declarationProgram) name(allowBlank bool) string {
	for {
		n := declarationNames[g.r.Intn(len(declarationNames))]
		if n != "_" || allowBlank {
			return n
		}
	}
}

func (g *declarationProgram) indent() string { return strings.Repeat("    ", g.depth) }

func (g *declarationProgram) block(n int) {
	g.depth++
	for i := 0; i < n; i++ {
		g.statement()
	}
	g.depth--
}

func (g *declarationProgram) statement() {
	choice := g.r.Intn(10)
	if g.depth >= 3 && choice >= 4 {
		choice = g.r.Intn(4)
	}
	in := g.indent()
	switch choice {
	case 0, 1:
		fmt.Fprintf(&g.sb, "%slet %s = 1;\n", in, g.name(false))
	case 2, 3:
		x, y := g.name(true), g.name(true)
		if x == y && x != "_" {
			y = "_"
		}
		fmt.Fprintf(&g.sb, "%slet %s, %s = [1, 2];\n", in, x, y)
	case 4:
		fmt.Fprintf(&g.sb, "%sif (true) {\n", in)
		g.block(1 + g.r.Intn(3))
		fmt.Fprintf(&g.sb, "%s}\n", in)
	case 5:
		fmt.Fprintf(&g.sb, "%sfor (let i = 0; i < 1; i++) {\n", in)
		g.block(1 + g.r.Intn(3))
		fmt.Fprintf(&g.sb, "%s}\n", in)
	case 6:
		fmt.Fprintf(&g.sb, "%sfor (%s in [1]) {\n", in, g.name(false))
		g.block(1 + g.r.Intn(3))
		fmt.Fprintf(&g.sb, "%s}\n", in)
	case 7:
		p, q := g.name(false), g.name(false)
		params := p
		if p != q {
			params = p + ", " + q
		}
		fmt.Fprintf(&g.sb, "%slet %s = fn(%s) {\n", in, g.name(false), params)
		g.block(1 + g.r.Intn(3))
		fmt.Fprintf(&g.sb, "%s};\n", in)
	case 8:
		fmt.Fprintf(&g.sb, "%swhile (false) {\n", in)
		g.block(1 + g.r.Intn(3))
		fmt.Fprintf(&g.sb, "%s}\n", in)
	case 9:
		fmt.Fprintf(&g.sb, "%smatch (1) {\n%s    1 => {\n", in, in)
		g.depth++
		g.block(1 + g.r.Intn(2))
		g.depth--
		fmt.Fprintf(&g.sb, "%s    },\n%s    _ => {\n", in, in)
		g.depth++
		g.block(1 + g.r.Intn(2))
		g.depth--
		fmt.Fprintf(&g.sb, "%s    }\n%s};\n", in, in)
	}
}

// TestRedeclarationParityAgainstTheCompiler compiles every generated program
// and holds the editor to the verdict.
//
// An accepted program must draw no refusal and no duplicate-declaration
// warning. A program refused as a redeclaration must draw the compiler's own
// sentence as the editor's first refusal, at Error, with no lint warning beside
// it -- and with no copy of the refusal in the compiler's name either. The
// compile pass shows that copy only when no rule has reported the refusal
// inside the range the compiler raised it at, so its absence is what says the
// editor's refusal is on the let the compiler refused.
func TestRedeclarationParityAgainstTheCompiler(t *testing.T) {
	const programs = 1000
	r := rand.New(rand.NewSource(20261011))

	accepted, refused, otherwise := 0, 0, 0
	var disagreements []string
	disagree := func(src, format string, args ...any) {
		disagreements = append(disagreements, fmt.Sprintf(format, args...)+"\n"+src)
	}
	for i := 0; i < programs; i++ {
		g := &declarationProgram{r: r}
		for n := 2 + r.Intn(5); n > 0; n-- {
			g.statement()
		}
		src := g.sb.String()

		refusal, parsed := buildRefusal(src)
		if !parsed {
			t.Fatalf("the generator wrote a program that does not parse (%s):\n%s", refusal, src)
		}
		diagnostics := Diagnostics(New().Analyze(src), DefaultLintConfig())
		refusals := diagnosticsFrom(diagnostics, "mutant-declarations")
		var duplicates []lsp.Diagnostic
		for _, d := range diagnosticsFrom(diagnostics, "mutant-lint") {
			if strings.Contains(d.Message, "duplicate") {
				duplicates = append(duplicates, d)
			}
		}
		sort.SliceStable(refusals, func(a, b int) bool {
			x, y := refusals[a].Range.Start, refusals[b].Range.Start
			return x.Line < y.Line || (x.Line == y.Line && x.Character < y.Character)
		})

		switch {
		case refusal == "":
			accepted++
			if len(refusals) > 0 || len(duplicates) > 0 {
				disagree(src, "the compiler accepts this, and the editor says:\n%s",
					describeDiagnostics(append(refusals, duplicates...)))
			}
		case strings.Contains(refusal, "already declared in this scope") ||
			strings.Contains(refusal, "declares nothing new"):
			refused++
			switch {
			case len(refusals) == 0:
				disagree(src, "the compiler refuses this with %q, and the editor refuses nothing:\n%s",
					refusal, describeDiagnostics(diagnostics))
			case refusals[0].Message != refusal || !hasErrorSeverity(refusals[:1]):
				disagree(src, "the compiler refuses this with %q, and the editor's first refusal is:\n%s",
					refusal, describeDiagnostics(refusals[:1]))
			case len(duplicates) > 0:
				disagree(src, "the editor reports the refusal and a lint warning in its own words as well:\n%s",
					describeDiagnostics(duplicates))
			case len(diagnosticsFrom(diagnostics, "mutant-compiler")) > 0:
				disagree(src, "the editor's refusal is not where the compiler raised %q, so the compiler's copy is shown too:\n%s",
					refusal, describeDiagnostics(diagnostics))
			}
		default:
			otherwise++
		}
	}

	t.Logf("%d programs: %d accepted, %d refused as a redeclaration, %d refused otherwise",
		programs, accepted, refused, otherwise)
	if accepted == 0 || refused == 0 {
		t.Fatalf("the generator no longer writes both accepted and refused programs, so this test checks one direction only")
	}
	for i, d := range disagreements {
		if i == 5 {
			t.Errorf("... and %d more", len(disagreements)-i)
			break
		}
		t.Errorf("disagreement %d of %d: %s", i+1, len(disagreements), d)
	}
}
