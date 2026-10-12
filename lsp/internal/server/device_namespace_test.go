package server

// M26-LSP-031: a workspace opened through the device namespace, \\.\C:\..., is
// the drive path under another name, and VS Code sends it that way: the folder
// and every document as file://./c%3A/..., and rootPath as \\.\C:\.... uriToPath
// refused the authority ".", so the roots fell back to the raw rootPath, every
// scanned file was filed under a file://./C:/... URI that uriToPath refused as
// well, and the open document had no module key. Its go-to-definition landed
// on its own use site, it had no links, and its disk copy stayed in
// workspace/symbol beside the editor's, offering a name the editor had renamed.
//
// The fixtures live in a temporary directory reached through \\.\, so no share
// or special drive has to exist; off Windows there is no device namespace.

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/tliron/glsp"
	lsp "github.com/tliron/glsp/protocol_3_16"
)

const deviceMainSource = "import stats \"lib.mut\";\n" +
	"let report = fn() { return stats.mean([1]); };\n" +
	"stats.mean([2]);\n" +
	"let d = \"data.txt\";\n"

// deviceWorkspace writes lib.mut, main.mut and data.txt to a temporary
// directory and returns that directory under its device-namespace name, with
// the URI VS Code gives the name.
func deviceWorkspace(t *testing.T) (string, lsp.DocumentUri) {
	t.Helper()
	if runtime.GOOS != "windows" {
		t.Skip("the device namespace is a Windows spelling")
	}
	plain := t.TempDir()
	if len(filepath.VolumeName(plain)) != 2 {
		t.Skipf("the temporary directory %q is not on a drive", plain)
	}
	for name, text := range map[string]string{
		"lib.mut":  "let mean = fn(xs) { return 0; };\n",
		"main.mut": deviceMainSource,
		"data.txt": "x\n",
	} {
		if err := os.WriteFile(filepath.Join(plain, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	dir := filepath.FromSlash("//./") + plain
	if _, err := os.Stat(filepath.Join(dir, "lib.mut")); err != nil {
		t.Skipf("the device-namespace path is unreadable here: %v", err)
	}
	return dir, vsCodeFileURI(".", "/"+filepath.ToSlash(plain))
}

// vsCodeFileURI spells a file URI the way VS Code's URI.toString does: a
// lower-case drive letter, and every byte of the path percent-encoded except
// the unreserved characters and "/".
func vsCodeFileURI(authority, path string) lsp.DocumentUri {
	if len(path) >= 3 && path[0] == '/' && path[2] == ':' {
		path = "/" + strings.ToLower(path[1:2]) + path[2:]
	}
	var b strings.Builder
	for i := 0; i < len(path); i++ {
		c := path[i]
		if 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || '0' <= c && c <= '9' || strings.IndexByte("-._~/", c) >= 0 {
			b.WriteByte(c)
			continue
		}
		fmt.Fprintf(&b, "%%%02X", c)
	}
	return lsp.DocumentUri("file://" + authority + b.String())
}

func TestAWorkspaceOpenedThroughTheDeviceNamespaceKeepsItsCrossFileAnswers(t *testing.T) {
	// Both orders, because each takes a different path to the one answer: a
	// scan before the open is undone by the open's eviction, and a scan after
	// it has to recognise the open document and leave its file alone.
	for _, scanFirst := range []bool{true, false} {
		name := "open, then scan"
		if scanFirst {
			name = "scan, then open"
		}
		t.Run(name, func(t *testing.T) {
			dir, folder := deviceWorkspace(t)
			s := New(false)
			rootURI := folder
			rootPath := dir
			s.captureWorkspaceRoots(&lsp.InitializeParams{
				RootURI:          &rootURI,
				RootPath:         &rootPath,
				WorkspaceFolders: []lsp.WorkspaceFolder{{URI: string(folder), Name: "dev"}},
			})

			ctx := &glsp.Context{Notify: func(string, any) {}}
			mainURI := folder + "/main.mut"
			open := func() {
				if err := s.didOpen(ctx, &lsp.DidOpenTextDocumentParams{TextDocument: lsp.TextDocumentItem{
					URI: mainURI, LanguageID: "mutant", Version: 1, Text: deviceMainSource,
				}}); err != nil {
					t.Fatal(err)
				}
			}
			if scanFirst {
				s.scanWorkspace()
				open()
			} else {
				open()
				s.scanWorkspace()
			}

			if snapshot, ok := s.snapshot(mainURI); !ok || snapshot.ModuleKey == "" {
				t.Errorf("main.mut, opened as %s, has no module key", mainURI)
			}

			lines := strings.Split(deviceMainSource, "\n")
			at := lsp.Position{Line: 1, Character: lsp.UInteger(strings.Index(lines[1], "mean") + 1)}
			def, err := s.definition(nil, &lsp.DefinitionParams{TextDocumentPositionParams: lsp.TextDocumentPositionParams{
				TextDocument: lsp.TextDocumentIdentifier{URI: mainURI}, Position: at,
			}})
			var location lsp.Location
			switch found := def.(type) {
			case lsp.Location:
				location = found
			case *lsp.Location:
				if found != nil {
					location = *found
				}
			}
			lib := filepath.Join(dir, "lib.mut")
			if landed, ok := uriToPath(location.URI); err != nil || !ok || canonicalPath(landed) != canonicalPath(lib) {
				t.Errorf("go-to-definition on stats.mean = %+v (err %v), want a location in %s", def, err, lib)
			}

			links, err := s.documentLinks(ctx, &lsp.DocumentLinkParams{TextDocument: lsp.TextDocumentIdentifier{URI: mainURI}})
			if err != nil {
				t.Fatal(err)
			}
			targets := map[string]bool{}
			for _, link := range links {
				if link.Target == nil {
					t.Errorf("a link with no target: %+v", link)
					continue
				}
				target, ok := uriToPath(*link.Target)
				if !ok {
					t.Errorf("link target %s does not read back to a path", *link.Target)
					continue
				}
				targets[canonicalPath(target)] = true
			}
			for _, file := range []string{"lib.mut", "data.txt"} {
				if !targets[canonicalPath(filepath.Join(dir, file))] {
					t.Errorf("no link to %s among %d links", file, len(links))
				}
			}

			// The editor renames report without saving. The copy the scan read
			// from disk must not still be offering the old name.
			edited := strings.Replace(deviceMainSource, "let report", "let summary", 1)
			if err := s.didChange(ctx, &lsp.DidChangeTextDocumentParams{
				TextDocument:   lsp.VersionedTextDocumentIdentifier{TextDocumentIdentifier: lsp.TextDocumentIdentifier{URI: mainURI}, Version: 2},
				ContentChanges: []any{lsp.TextDocumentContentChangeEventWhole{Text: edited}},
			}); err != nil {
				t.Fatal(err)
			}
			if stale := s.symbols.WorkspaceSymbols("report", 100); len(stale) != 0 {
				t.Errorf("workspace/symbol still offers report after the rename: %+v", stale)
			}
			if renamed := s.symbols.WorkspaceSymbols("summary", 100); len(renamed) != 1 {
				t.Errorf("workspace/symbol offers summary %d times after the rename, want 1: %+v", len(renamed), renamed)
			}
		})
	}
}

// TestTheScanSkipsAFileWhoseURICannotReadBackToIt: VS Code sends a folder
// opened as \\?\C:\... as file://%3F/c%3A/..., an escape url.Parse rejects, so
// captureWorkspaceRoots falls back to rootPath and the scan walks the raw \\?\
// path. Each file was indexed under a URI that read back to no file at all,
// file://%3F/C:/..., and the open documents, refused just the same, could
// never evict those copies, so workspace/symbol kept serving the names they had
// on disk (M26-LSP-031). A file is indexed under a URI that reads back to the
// file itself, or not at all. setRoots stands in for the fallback here.
func TestTheScanSkipsAFileWhoseURICannotReadBackToIt(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("the long-path prefix is a Windows spelling")
	}
	plain := t.TempDir()
	if len(filepath.VolumeName(plain)) != 2 {
		t.Skipf("the temporary directory %q is not on a drive", plain)
	}
	if err := os.WriteFile(filepath.Join(plain, "lib.mut"), []byte("let long_path_only = 1;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	long := filepath.FromSlash("//?/") + plain
	if _, err := os.Stat(filepath.Join(long, "lib.mut")); err != nil {
		t.Skipf("the long-path spelling is unreadable here: %v", err)
	}

	s := New(false)
	s.setRoots([]string{long})
	s.scanWorkspace()

	if got := s.symbols.WorkspaceSymbols("long_path_only", 10); len(got) != 0 {
		t.Errorf("the scan of %s indexed lib.mut under a URI that does not read back to it: %+v", long, got)
	}
	s.mu.RLock()
	for key, uri := range s.scanned {
		t.Errorf("scanned[%q] = %s", key, uri)
	}
	s.mu.RUnlock()

	// The control: the same file under its plain name is still indexed.
	if !s.indexFileFromDisk(filepath.Join(plain, "lib.mut")) {
		t.Errorf("the scan skipped %s, whose URI reads back to it", filepath.Join(plain, "lib.mut"))
	}
}

// TestAFileOpenedUnderItsPlainNameInADeviceWorkspaceSaysNothingStale: \\.\c:\x
// and c:\x are one file to Windows, and one file reaches the editor under both
// spellings -- from the explorer of a folder opened through \\.\, and as c:\...
// from a terminal link or File > Open. Here lib.mut is opened under its plain
// name and its mean renamed to added without saving, while main.mut, opened
// through \\.\, uses both names. Keyed apart, the copy of lib.mut the scan read
// from disk stayed in workspace/symbol beside the editor's, still offering
// mean; main.mut's import went on resolving to that copy, so go-to-definition
// on stats.mean landed on the disk copy's mean, and stats.added was reported
// missing although the editor's lib.mut declares it (M26-LSP-031). What the
// server answers has to agree with the editor's copy, or be nothing. Resolving
// main.mut's import to the editor's copy needs the fold in sema.CanonicalKey,
// which the server cannot make for the module resolver.
func TestAFileOpenedUnderItsPlainNameInADeviceWorkspaceSaysNothingStale(t *testing.T) {
	const (
		libSource     = "let mean = fn(xs) { return 0; };\n"
		renamedSource = "let added = fn(xs) { return 0; };\n"
	)
	mainSource := deviceMainSource + "stats.added([3]);\n"
	lines := strings.Split(mainSource, "\n")
	meanAt := lsp.Position{Line: 2, Character: lsp.UInteger(strings.Index(lines[2], "mean") + 1)}
	addedAt := lsp.Position{Line: 4, Character: lsp.UInteger(strings.Index(lines[4], "added") + 1)}

	// Both orders: a scan before the opens is undone by the eviction of the
	// disk copy, and a scan after them has to see that lib.mut is open under
	// the other spelling and leave it alone.
	for _, scanFirst := range []bool{true, false} {
		name := "open, then scan"
		if scanFirst {
			name = "scan, then open"
		}
		t.Run(name, func(t *testing.T) {
			dir, folder := deviceWorkspace(t)
			plain := strings.TrimPrefix(dir, filepath.FromSlash("//./"))
			libURI := vsCodeFileURI("", "/"+filepath.ToSlash(filepath.Join(plain, "lib.mut")))
			mainURI := folder + "/main.mut"

			s := New(false)
			rootURI := folder
			rootPath := dir
			s.captureWorkspaceRoots(&lsp.InitializeParams{
				RootURI:          &rootURI,
				RootPath:         &rootPath,
				WorkspaceFolders: []lsp.WorkspaceFolder{{URI: string(folder), Name: "dev"}},
			})
			ctx := &glsp.Context{Notify: func(string, any) {}}
			if scanFirst {
				s.scanWorkspace()
			}
			for _, doc := range []lsp.TextDocumentItem{
				{URI: libURI, LanguageID: "mutant", Version: 1, Text: libSource},
				{URI: mainURI, LanguageID: "mutant", Version: 1, Text: mainSource},
			} {
				if err := s.didOpen(ctx, &lsp.DidOpenTextDocumentParams{TextDocument: doc}); err != nil {
					t.Fatal(err)
				}
			}
			if !scanFirst {
				s.scanWorkspace()
			}
			if err := s.didChange(ctx, &lsp.DidChangeTextDocumentParams{
				TextDocument:   lsp.VersionedTextDocumentIdentifier{TextDocumentIdentifier: lsp.TextDocumentIdentifier{URI: libURI}, Version: 2},
				ContentChanges: []any{lsp.TextDocumentContentChangeEventWhole{Text: renamedSource}},
			}); err != nil {
				t.Fatal(err)
			}

			if stale := s.symbols.WorkspaceSymbols("mean", 100); len(stale) != 0 {
				t.Errorf("workspace/symbol still offers mean after the editor renamed it: %+v", stale)
			}
			if renamed := s.symbols.WorkspaceSymbols("added", 100); len(renamed) != 1 {
				t.Errorf("workspace/symbol offers added %d times, want once: %+v", len(renamed), renamed)
			}

			snapshot, ok := s.snapshot(mainURI)
			if !ok {
				t.Fatal("main.mut has no snapshot")
			}
			for _, diagnostic := range s.diagnose(snapshot) {
				if strings.Contains(diagnostic.Message, "stats.added") {
					t.Errorf("main.mut is told %q, although the editor's lib.mut declares added", diagnostic.Message)
				}
			}

			// lib.mut under either spelling.
			lib := map[string]bool{
				canonicalPath(filepath.Join(dir, "lib.mut")):   true,
				canonicalPath(filepath.Join(plain, "lib.mut")): true,
			}
			if at, ok := deviceDefinition(t, s, mainURI, meanAt); ok {
				if path, read := uriToPath(at.URI); read && lib[canonicalPath(path)] {
					t.Errorf("go-to-definition on stats.mean = %+v, a declaration the editor's lib.mut no longer has", at)
				}
			}
			if at, ok := deviceDefinition(t, s, mainURI, addedAt); ok {
				editors := at.URI == libURI && at.Range.Start == lsp.Position{Line: 0, Character: 4}
				if path, read := uriToPath(at.URI); read && lib[canonicalPath(path)] && !editors {
					t.Errorf("go-to-definition on stats.added = %+v, want the editor's declaration in %s at 0:4, or nothing", at, libURI)
				}
			}
		})
	}
}

// deviceDefinition asks for go-to-definition at pos in uri and returns the one
// location it answers with. Any other shape of answer fails the test, so that
// an assertion about where it lands can never be skipped by a shape it did not
// expect.
func deviceDefinition(t *testing.T, s *Server, uri lsp.DocumentUri, pos lsp.Position) (lsp.Location, bool) {
	t.Helper()
	def, err := s.definition(nil, &lsp.DefinitionParams{TextDocumentPositionParams: lsp.TextDocumentPositionParams{
		TextDocument: lsp.TextDocumentIdentifier{URI: uri}, Position: pos,
	}})
	if err != nil {
		t.Fatalf("go-to-definition at %+v in %s: %v", pos, uri, err)
	}
	switch found := def.(type) {
	case nil:
		return lsp.Location{}, false
	case lsp.Location:
		return found, true
	case *lsp.Location:
		if found == nil {
			return lsp.Location{}, false
		}
		return *found, true
	}
	t.Fatalf("go-to-definition at %+v in %s answered %T %+v", pos, uri, def, def)
	return lsp.Location{}, false
}
