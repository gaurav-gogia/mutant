package server

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/tliron/glsp"
	lsp "github.com/tliron/glsp/protocol_3_16"
)

// TestALinkTargetAlwaysReadsBackToItsFile: a workspace root taken raw from
// rootPath can be a \\?\ path naming a segment Windows renames without that
// prefix -- here a directory whose name ends in a dot, which only the
// long-path prefix can create or reach. The link to a file under it was
// offered as file://%3F/..., which uriToPath refuses, and once pathToURI
// answered such a path with "", with an empty target (M26-LSP-031). A file
// whose URI cannot read back to it gets no link; one beside the document
// keeps its link.
func TestALinkTargetAlwaysReadsBackToItsFile(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("the long-path prefix is a Windows spelling")
	}
	plain := t.TempDir()
	if len(filepath.VolumeName(plain)) != 2 {
		t.Skipf("the temporary directory %q is not on a drive", plain)
	}
	odd := filepath.FromSlash("//?/") + filepath.Join(plain, "odd.")
	if err := os.Mkdir(odd, 0o755); err != nil {
		t.Skipf("cannot create a directory whose name ends in a dot: %v", err)
	}
	lib := filepath.Join(odd, "lib.mut")
	if err := os.WriteFile(lib, []byte("let x = 1;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	data := filepath.Join(plain, "data.txt")
	if err := os.WriteFile(data, []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	s := New(false)
	s.setRoots([]string{odd})
	ctx := &glsp.Context{Notify: func(string, any) {}}
	mainURI := pathToURI(filepath.Join(plain, "main.mut"))
	if mainURI == "" {
		t.Fatalf("no URI for %s", filepath.Join(plain, "main.mut"))
	}
	source := "let d = \"data.txt\";\nlet l = \"" + filepath.ToSlash(lib) + "\";\n"
	if err := s.didOpen(ctx, &lsp.DidOpenTextDocumentParams{TextDocument: lsp.TextDocumentItem{
		URI: mainURI, LanguageID: "mutant", Version: 1, Text: source,
	}}); err != nil {
		t.Fatal(err)
	}

	links, err := s.documentLinks(ctx, &lsp.DocumentLinkParams{TextDocument: lsp.TextDocumentIdentifier{URI: mainURI}})
	if err != nil {
		t.Fatal(err)
	}
	linked := false
	for _, link := range links {
		if link.Target == nil || *link.Target == "" {
			t.Errorf("a link at %+v with no target", link.Range)
			continue
		}
		target, ok := uriToPath(*link.Target)
		if !ok {
			t.Errorf("link target %s does not read back to a path", *link.Target)
			continue
		}
		if canonicalPath(target) != canonicalPath(data) {
			t.Errorf("a link to %s (%s); no URI reads back to %s, and data.txt is the only other literal", target, *link.Target, lib)
			continue
		}
		linked = true
	}
	if !linked {
		t.Errorf("no link to %s among %d links", data, len(links))
	}
}
