package server

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tliron/glsp"
	lsp "github.com/tliron/glsp/protocol_3_16"

	"mutant/sema"
)

// M26-SEM-005: a module that stopped existing went on resolving for the rest of
// the session.
//
// sema.Workspace.RemoveFile had no caller. A Deleted event from the file
// watcher dropped the file's symbol-index entry and left its declarations in
// the shared workspace, and closing a buffer that had never been saved left
// behind everything it had filed. An import resolves to any file the workspace
// holds facts for, so an import of the deleted file went on resolving:
// go-to-definition jumped into a file that was gone and `stats.` offered its
// members, while the build said `cannot find module "lib/stats.mut"`.
//
// On Windows the symbol index missed too. The scan files a document under
// file:///C:/..., VS Code reports the delete as file:///c%3A/..., and both
// indexes are keyed by the string, so forgetting the URI the client sent
// forgot nothing at all.
//
// main.mut imports lib/stats.mut and calls stats.mean. Each test takes
// stats.mut away by one route and then asks what the editor still says about
// stats, re-analysing main.mut the way a keystroke does.

const (
	mainImportingStats = "import stats \"lib/stats.mut\";\nlet m = stats.mean([1]);\n"
	statsSource        = "let mean = fn(xs) { return 1; };\n"
)

var (
	// Inside `mean`, and just after `stats.`, on mainImportingStats's line 1.
	insideStatsMean     = lsp.Position{Line: 1, Character: 15}
	afterStatsDotInMain = lsp.Position{Line: 1, Character: 14}

	quietContext = &glsp.Context{Notify: func(string, any) {}}
)

// statsAsSeen is what the editor says about stats from inside main.mut.
type statsAsSeen struct {
	facts       bool     // the shared workspace holds stats.mut's declarations
	definition  bool     // go-to-definition on stats.mean jumps somewhere
	completions []string // what `stats.` offers
	symbols     int      // workspace symbols named mean
}

// resolves reports whether stats.mut is known everywhere the editor looks.
func (seen statsAsSeen) resolves() bool {
	return seen.facts && seen.definition &&
		len(seen.completions) == 1 && seen.completions[0] == "mean" &&
		seen.symbols == 1
}

// forgotten reports whether stats.mut is known nowhere.
func (seen statsAsSeen) forgotten() bool {
	return !seen.facts && !seen.definition && len(seen.completions) == 0 && seen.symbols == 0
}

// statsSeenFromMain re-analyses main.mut the way a keystroke does and reports
// what the editor now says about stats.
func statsSeenFromMain(t *testing.T, s *Server, mainURI lsp.DocumentUri, statsPath string) statsAsSeen {
	t.Helper()
	snapshot := s.analyzeDoc(mainURI, mainImportingStats)
	s.setSnapshot(mainURI, snapshot)

	var seen statsAsSeen
	_, seen.facts = s.sema.Facts(sema.CanonicalKey(statsPath))
	_, seen.definition = snapshot.ModuleMemberDefinition(insideStatsMean)
	items, _ := snapshot.MemberCompletionsAt(afterStatsDotInMain)
	for _, item := range items {
		seen.completions = append(seen.completions, item.Label)
	}
	seen.symbols = len(s.symbols.WorkspaceSymbols("mean", 100))
	return seen
}

// scannedStats scans a tree holding main.mut and lib/stats.mut, opens main.mut,
// and checks that stats resolves before anything is taken away.
func scannedStats(t *testing.T) (*Server, lsp.DocumentUri, string) {
	t.Helper()
	root := writeModules(t, map[string]string{
		"main.mut":      mainImportingStats,
		"lib/stats.mut": statsSource,
	})
	s, mainURI := serverOver(t, root, "main.mut")
	statsPath := filepath.Join(root, "lib", "stats.mut")
	if seen := statsSeenFromMain(t, s, mainURI, statsPath); !seen.resolves() {
		t.Fatalf("before anything is taken away, stats does not resolve: %+v", seen)
	}
	return s, mainURI, statsPath
}

func openStatsBuffer(t *testing.T, s *Server, uri lsp.DocumentUri) {
	t.Helper()
	if err := s.didOpen(quietContext, &lsp.DidOpenTextDocumentParams{TextDocument: lsp.TextDocumentItem{
		URI: uri, LanguageID: "mutant", Version: 1, Text: statsSource,
	}}); err != nil {
		t.Fatal(err)
	}
}

func closeBuffer(t *testing.T, s *Server, uri lsp.DocumentUri) {
	t.Helper()
	if err := s.didClose(quietContext, &lsp.DidCloseTextDocumentParams{
		TextDocument: lsp.TextDocumentIdentifier{URI: uri},
	}); err != nil {
		t.Fatal(err)
	}
}

// deleteFromDisk removes path, then reports it the way the client's file
// watcher does, under the URI the client spells it with.
func deleteFromDisk(t *testing.T, s *Server, path string, reported lsp.DocumentUri) {
	t.Helper()
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := s.didChangeWatchedFiles(nil, &lsp.DidChangeWatchedFilesParams{Changes: []lsp.FileEvent{
		{URI: reported, Type: lsp.FileChangeTypeDeleted},
	}}); err != nil {
		t.Fatal(err)
	}
}

// The watcher reports the delete under the URI the scan filed the file by.
func TestADeletedModuleStopsResolving(t *testing.T) {
	s, mainURI, statsPath := scannedStats(t)
	deleteFromDisk(t, s, statsPath, pathToURI(statsPath))

	if seen := statsSeenFromMain(t, s, mainURI, statsPath); !seen.forgotten() {
		t.Errorf("after the Deleted event, the deleted module still resolves: %+v", seen)
	}
}

// VS Code reports the delete as file:///c%3A/..., which is not the string the
// scan filed the file under. Forgetting only the URI the client sent left both
// the workspace facts and the symbol-index entry in place.
func TestADeletedModuleStopsResolvingUnderTheClientsSpelling(t *testing.T) {
	s, mainURI, statsPath := scannedStats(t)
	spelled, ok := vsCodeSpellingOf(string(pathToURI(statsPath)))
	if !ok {
		t.Skip("this host's paths carry no drive letter, so the client and the scan spell a file " +
			"the same way, and TestADeletedModuleStopsResolving is that case")
	}
	deleteFromDisk(t, s, statsPath, lsp.DocumentUri(spelled))

	if seen := statsSeenFromMain(t, s, mainURI, statsPath); !seen.forgotten() {
		t.Errorf("after the Deleted event spelled %s, the deleted module still resolves: %+v", spelled, seen)
	}
}

// A buffer that was never saved files its declarations while it is open, and
// importers resolve against them. Closing it leaves nothing on disk to index
// in its place, so what it filed has to go with it.
func TestClosingABufferWithNoFileForgetsIt(t *testing.T) {
	root := writeModules(t, map[string]string{"main.mut": mainImportingStats})
	s, mainURI := serverOver(t, root, "main.mut")
	statsPath := filepath.Join(root, "lib", "stats.mut") // never written
	statsURI := pathToURI(statsPath)

	openStatsBuffer(t, s, statsURI)
	if seen := statsSeenFromMain(t, s, mainURI, statsPath); !seen.resolves() {
		t.Fatalf("while the unsaved buffer is open, stats does not resolve: %+v", seen)
	}

	closeBuffer(t, s, statsURI)
	if seen := statsSeenFromMain(t, s, mainURI, statsPath); !seen.forgotten() {
		t.Errorf("after the never-saved buffer closed, it still resolves: %+v", seen)
	}
}

// A file deleted on disk while the editor has it open stays known: the open
// buffer is authoritative, as it is for a Created or Changed event, so nothing
// is forgotten while it is open -- the symbol index included. Closing it then
// finds nothing on disk, and the module is forgotten.
func TestADeletedOpenModuleIsForgottenWhenClosed(t *testing.T) {
	s, mainURI, statsPath := scannedStats(t)
	statsURI := pathToURI(statsPath)
	openStatsBuffer(t, s, statsURI)
	deleteFromDisk(t, s, statsPath, statsURI)

	if seen := statsSeenFromMain(t, s, mainURI, statsPath); !seen.resolves() {
		t.Errorf("deleted on disk while open, stats stopped resolving: %+v; "+
			"the open buffer is authoritative until it closes", seen)
	}

	closeBuffer(t, s, statsURI)
	if seen := statsSeenFromMain(t, s, mainURI, statsPath); !seen.forgotten() {
		t.Errorf("after the buffer of a deleted file closed, it still resolves: %+v", seen)
	}
}

// The other side of the didClose change: a buffer whose file is still on disk
// is indexed from disk again when it closes, and importers go on resolving.
func TestClosingABufferWhoseFileIsOnDiskKeepsItResolving(t *testing.T) {
	s, mainURI, statsPath := scannedStats(t)
	statsURI := pathToURI(statsPath)
	openStatsBuffer(t, s, statsURI)
	closeBuffer(t, s, statsURI)

	if seen := statsSeenFromMain(t, s, mainURI, statsPath); !seen.resolves() {
		t.Errorf("after closing a buffer whose file is on disk, stats stopped resolving: %+v", seen)
	}
}

// And a file open under two spellings is not forgotten when one of them
// closes: the other still owns its declarations. The spelling closed here is
// the one that filed them last, so forgetting by its URI alone would take them.
func TestClosingOneOfTwoSpellingsOfAnOpenFileKeepsIt(t *testing.T) {
	s, mainURI, statsPath := scannedStats(t)
	statsURI := pathToURI(statsPath)
	spelled, ok := vsCodeSpellingOf(string(statsURI))
	if !ok {
		t.Skip("this host's paths carry no drive letter, so there is no second spelling to open the file under")
	}
	openStatsBuffer(t, s, statsURI)
	openStatsBuffer(t, s, lsp.DocumentUri(spelled))
	closeBuffer(t, s, lsp.DocumentUri(spelled))

	if seen := statsSeenFromMain(t, s, mainURI, statsPath); !seen.resolves() {
		t.Errorf("after closing %s while %s stays open, stats stopped resolving: %+v", spelled, statsURI, seen)
	}
}
