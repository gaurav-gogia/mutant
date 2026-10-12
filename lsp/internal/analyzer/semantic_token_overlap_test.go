package analyzer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// LSP 3.16 lets a server send overlapping semantic tokens only to a client that
// advertised overlappingTokenSupport, and VS Code does not. A function literal,
// struct or enum statement written on one line got a token over the whole node,
// on top of every token inside it, and the declared name of a struct or enum
// was painted as a variable (M26-SEM-015).

func semanticTokenTypeName(id uint32) string {
	if int(id) < len(semanticTokenTypes) {
		return semanticTokenTypes[id]
	}
	return fmt.Sprintf("#%d", id)
}

// firstTokenOverlap reports the first pair of tokens on one line where the
// second starts before the first ends, and how many such pairs there are.
// semanticTokenList is sorted by line and start, so each token need only be
// checked against the next one on its line.
func firstTokenOverlap(tokens []semanticToken) (string, int) {
	first, count := "", 0
	for i := 0; i+1 < len(tokens); i++ {
		a, b := tokens[i], tokens[i+1]
		if a.line == b.line && b.start < a.start+a.length {
			if count == 0 {
				first = fmt.Sprintf("%d:%d+%d %s overlaps %d:%d+%d %s",
					a.line, a.start, a.length, semanticTokenTypeName(a.typeID),
					b.line, b.start, b.length, semanticTokenTypeName(b.typeID))
			}
			count++
		}
	}
	return first, count
}

func TestSemanticTokensNeverOverlapOnAOneLineDeclaration(t *testing.T) {
	for _, src := range []string{
		"struct Point { x; y; };\n",
		"enum Colour { Red, Green };\n",
		"let f = fn(x) { return x + 1; };\n",
		"let g = fn() { return fn(y) { return len(y); }; };\n",
	} {
		if first, n := firstTokenOverlap(New().Analyze(src).semanticTokenList()); n > 0 {
			t.Errorf("%q: %d overlapping pairs, first: %s", strings.TrimSpace(src), n, first)
		}
	}
}

// TestSemanticTokensNeverOverlapAcrossTheExamples holds the invariant over every
// real program in the tree, so a whole-node token added later for any other
// construct is caught too.
func TestSemanticTokensNeverOverlapAcrossTheExamples(t *testing.T) {
	root := filepath.Join("..", "..", "..", "examples")
	if _, err := os.Stat(root); err != nil {
		t.Skipf("examples directory not available: %v", err)
	}
	files, total := 0, 0
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".mut") {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files++
		if first, n := firstTokenOverlap(New().Analyze(string(data)).semanticTokenList()); n > 0 {
			if total == 0 {
				t.Logf("%s: %d overlapping pairs, first: %s", path, n, first)
			}
			total += n
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}
	if files == 0 {
		t.Skip("no example programs found")
	}
	if total > 0 {
		t.Errorf("%d overlapping token pairs across %d examples, want 0", total, files)
	}
}

// TestATypeDeclarationNameIsPaintedAsItsKind: `Point{...}` is painted type, so
// `struct Point` must be too, on one line or several.
func TestATypeDeclarationNameIsPaintedAsItsKind(t *testing.T) {
	for _, c := range []struct {
		src                 string
		line, start, length uint32
		want                string
	}{
		{"struct Point { x; y; };\n", 0, 7, 5, "type"},
		{"struct Point {\n  x; y;\n};\n", 0, 7, 5, "type"},
		{"enum Colour { Red, Green };\n", 0, 5, 6, "enum"},
		{"enum Colour {\n  Red, Green\n};\n", 0, 5, 6, "enum"},
	} {
		var got []string
		for _, tok := range New().Analyze(c.src).semanticTokenList() {
			if tok.line == c.line && tok.start == c.start && tok.length == c.length {
				got = append(got, semanticTokenTypeName(tok.typeID))
			}
		}
		if len(got) != 1 || got[0] != c.want {
			t.Errorf("%q: the declared name is painted %v, want [%s]",
				strings.SplitN(c.src, "\n", 2)[0], got, c.want)
		}
	}
}
