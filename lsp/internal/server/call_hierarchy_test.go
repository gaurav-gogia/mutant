package server

import (
	"sort"
	"testing"

	lsp "github.com/tliron/glsp/protocol_3_16"
)

// The call hierarchy, driven through the real handlers.
//
// The file-local half is the graph read as a call graph. The cross-file half is
// the part no single file can answer, and it is the reason the feature is worth
// having: the caller you are looking for is usually in a file nobody has open.

func prepareAt(t *testing.T, s *Server, uri lsp.DocumentUri, line, character int) lsp.CallHierarchyItem {
	t.Helper()
	items, err := s.prepareCallHierarchy(nil, &lsp.CallHierarchyPrepareParams{
		TextDocumentPositionParams: lsp.TextDocumentPositionParams{
			TextDocument: lsp.TextDocumentIdentifier{URI: uri},
			Position:     lsp.Position{Line: lsp.UInteger(line), Character: lsp.UInteger(character)},
		},
	})
	if err != nil {
		t.Fatalf("prepareCallHierarchy: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("prepareCallHierarchy returned %d items, want 1: %+v", len(items), items)
	}
	return items[0]
}

func incomingNames(t *testing.T, s *Server, item lsp.CallHierarchyItem) []string {
	t.Helper()
	calls, err := s.callHierarchyIncomingCalls(nil, &lsp.CallHierarchyIncomingCallsParams{Item: item})
	if err != nil {
		t.Fatalf("incomingCalls: %v", err)
	}
	names := make([]string, 0, len(calls))
	for _, call := range calls {
		if len(call.FromRanges) == 0 {
			t.Fatalf("caller %q came back with no call sites, so the editor has "+
				"nothing to highlight", call.From.Name)
		}
		names = append(names, call.From.Name)
	}
	sort.Strings(names)
	return names
}

func outgoingNames(t *testing.T, s *Server, item lsp.CallHierarchyItem) []string {
	t.Helper()
	calls, err := s.callHierarchyOutgoingCalls(nil, &lsp.CallHierarchyOutgoingCallsParams{Item: item})
	if err != nil {
		t.Fatalf("outgoingCalls: %v", err)
	}
	names := make([]string, 0, len(calls))
	for _, call := range calls {
		names = append(names, call.To.Name)
	}
	sort.Strings(names)
	return names
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestTheCallersOfAFunctionInOneFile(t *testing.T) {
	root := writeModules(t, map[string]string{
		"main.mut": "let target = fn() { return 1; };\n" +
			"let first = fn() { return target(); };\n" +
			"let second = fn() { return target() + target(); };\n" +
			"let mentions = fn() { return target; };\n" +
			"first();\n",
	})
	s, uri := serverOver(t, root, "main.mut")

	item := prepareAt(t, s, uri, 0, 4)
	if item.Name != "target" {
		t.Fatalf("prepare resolved to %q, want target", item.Name)
	}

	got := incomingNames(t, s, item)
	want := []string{"first", "second"}
	if !equalStrings(got, want) {
		t.Fatalf("callers = %v, want %v -- `mentions` names target without "+
			"calling it, which is a reference and not an edge", got, want)
	}
}

// Two calls from one function are one caller with two sites, not two callers.
func TestTwoCallsFromOneFunctionAreOneCaller(t *testing.T) {
	root := writeModules(t, map[string]string{
		"main.mut": "let target = fn() { return 1; };\n" +
			"let twice = fn() { return target() + target(); };\n" +
			"twice();\n",
	})
	s, uri := serverOver(t, root, "main.mut")

	calls, err := s.callHierarchyIncomingCalls(nil, &lsp.CallHierarchyIncomingCallsParams{
		Item: prepareAt(t, s, uri, 0, 4),
	})
	if err != nil {
		t.Fatalf("incomingCalls: %v", err)
	}
	if len(calls) != 1 {
		t.Fatalf("callers = %d, want 1", len(calls))
	}
	if len(calls[0].FromRanges) != 2 {
		t.Fatalf("call sites = %d, want 2", len(calls[0].FromRanges))
	}
}

func TestWhatAFunctionCalls(t *testing.T) {
	root := writeModules(t, map[string]string{
		"main.mut": "let a = fn() { return 1; };\n" +
			"let b = fn() { return 2; };\n" +
			"let caller = fn() { return a() + b() + a(); };\n" +
			"caller();\n",
	})
	s, uri := serverOver(t, root, "main.mut")

	got := outgoingNames(t, s, prepareAt(t, s, uri, 2, 4))
	want := []string{"a", "b"}
	if !equalStrings(got, want) {
		t.Fatalf("callees = %v, want %v", got, want)
	}
}

// A recursive call is an edge and belongs in the hierarchy. Hiding it would
// hide the thing a reader opened the view to find.
func TestARecursiveCallIsAnEdge(t *testing.T) {
	root := writeModules(t, map[string]string{
		"main.mut": "let down = fn(n) { return down(n); };\ndown(1);\n",
	})
	s, uri := serverOver(t, root, "main.mut")

	item := prepareAt(t, s, uri, 0, 4)
	if got := outgoingNames(t, s, item); !equalStrings(got, []string{"down"}) {
		t.Fatalf("callees = %v, want [down]", got)
	}
	// `down(1);` at the top level is a call too, made by the file.
	if got := incomingNames(t, s, item); !equalStrings(got, []string{"down", "main.mut"}) {
		t.Fatalf("callers = %v, want [down main.mut]", got)
	}
}

// The half a single file cannot answer. other.mut is scanned and never opened,
// which is the ordinary case: the caller you are hunting is in a file you have
// not got on screen.
func TestTheCallersOfAModuleMemberReachUnopenedFiles(t *testing.T) {
	root := writeModules(t, map[string]string{
		"lib.mut": "let mean = fn(xs) { return 0; };\n",
		"main.mut": "import stats \"lib.mut\";\n" +
			"let report = fn() { return stats.mean([1]); };\n" +
			"report();\n",
		"other.mut": "import s2 \"lib.mut\";\n" +
			"let summary = fn() { return s2.mean([2]); };\n" +
			"summary();\n",
		"unrelated.mut": "let mean = fn(xs) { return 99; };\nmean([3]);\n",
	})
	s, _ := serverOver(t, root, "main.mut")

	libURI := pathToURI(root + "/lib.mut")
	item := prepareAt(t, s, libURI, 0, 4)
	if item.Name != "mean" {
		t.Fatalf("prepare resolved to %q, want mean", item.Name)
	}

	got := incomingNames(t, s, item)
	want := []string{"report", "summary"}
	if !equalStrings(got, want) {
		t.Fatalf("callers = %v, want %v -- `summary` is in a file that was "+
			"scanned and never opened, and `mean` in unrelated.mut is a "+
			"different function that nothing imports", got, want)
	}
}

// A mention that is not a call must not become an edge across files either.
func TestAModuleMemberNamedButNotCalledIsNotACaller(t *testing.T) {
	root := writeModules(t, map[string]string{
		"lib.mut": "let mean = fn(xs) { return 0; };\n",
		"main.mut": "import stats \"lib.mut\";\n" +
			"let held = fn() { return stats.mean; };\n" +
			"held();\n",
	})
	s, _ := serverOver(t, root, "main.mut")

	item := prepareAt(t, s, pathToURI(root+"/lib.mut"), 0, 4)
	if got := incomingNames(t, s, item); len(got) != 0 {
		t.Fatalf("callers = %v, want none: stats.mean is named, never called", got)
	}
}

// A call at a file's top level has no enclosing declaration. The graph says so
// rather than inventing one, and the file is what makes the call.
func TestACallAtTheTopLevelOfAnotherFileIsAttributedToTheFile(t *testing.T) {
	root := writeModules(t, map[string]string{
		"lib.mut": "let mean = fn(xs) { return 0; };\n",
		"main.mut": "import stats \"lib.mut\";\n" +
			"stats.mean([1]);\n",
	})
	s, _ := serverOver(t, root, "main.mut")

	calls, err := s.callHierarchyIncomingCalls(nil, &lsp.CallHierarchyIncomingCallsParams{
		Item: prepareAt(t, s, pathToURI(root+"/lib.mut"), 0, 4),
	})
	if err != nil {
		t.Fatalf("incomingCalls: %v", err)
	}
	if len(calls) != 1 {
		t.Fatalf("callers = %d, want 1: %+v", len(calls), calls)
	}
	if calls[0].From.Kind != lsp.SymbolKindFile {
		t.Fatalf("the caller is a %v, want a file -- nothing in main.mut "+
			"declares the call", calls[0].From.Kind)
	}
	if calls[0].From.Name != "main.mut" {
		t.Fatalf("the caller is named %q, want main.mut", calls[0].From.Name)
	}
}

// A type is not callable, so a hierarchy rooted on one would be an empty view
// offered for no reason.
func TestATypeHasNoCallHierarchy(t *testing.T) {
	root := writeModules(t, map[string]string{
		"main.mut": "struct Point { x };\nlet p = Point{x: 1};\np;\n",
	})
	s, uri := serverOver(t, root, "main.mut")

	items, err := s.prepareCallHierarchy(nil, &lsp.CallHierarchyPrepareParams{
		TextDocumentPositionParams: lsp.TextDocumentPositionParams{
			TextDocument: lsp.TextDocumentIdentifier{URI: uri},
			Position:     lsp.Position{Line: 0, Character: 7},
		},
	})
	if err != nil {
		t.Fatalf("prepareCallHierarchy: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("a struct offered a call hierarchy: %+v", items)
	}
}

// Prepare works from a call site as well as from the declaration: "who else
// calls this" is asked with the cursor on a call at least as often.
func TestPrepareWorksFromACallSite(t *testing.T) {
	root := writeModules(t, map[string]string{
		"main.mut": "let target = fn() { return 1; };\n" +
			"let caller = fn() { return target(); };\n" +
			"caller();\n",
	})
	s, uri := serverOver(t, root, "main.mut")

	item := prepareAt(t, s, uri, 1, 28)
	if item.Name != "target" {
		t.Fatalf("prepare from a call site resolved to %q, want target", item.Name)
	}
	if got := incomingNames(t, s, item); !equalStrings(got, []string{"caller"}) {
		t.Fatalf("callers = %v, want [caller]", got)
	}
}

// Rooting the hierarchy on a name this file does not declare. The cursor is on
// `stats.mean` in the importing file; the item has to be the declaration in the
// other file, because that is what "who calls this" is about -- rooting it on
// the alias would ask who calls a namespace.
func TestPrepareFollowsAModuleMemberToItsDeclaration(t *testing.T) {
	root := writeModules(t, map[string]string{
		"lib.mut": "let mean = fn(xs) { return 0; };\n",
		"main.mut": "import stats \"lib.mut\";\n" +
			"let report = fn() { return stats.mean([1]); };\n" +
			"report();\n",
	})
	s, uri := serverOver(t, root, "main.mut")

	item := prepareAt(t, s, uri, 1, 34)
	if item.Name != "mean" {
		t.Fatalf("prepare on stats.mean resolved to %q, want mean", item.Name)
	}
	if item.URI != pathToURI(root+"/lib.mut") {
		t.Fatalf("the item points at %s, want lib.mut", item.URI)
	}
	if got := incomingNames(t, s, item); !equalStrings(got, []string{"report"}) {
		t.Fatalf("callers = %v, want [report]", got)
	}
}

// Two modules, each declaring `mean`, each imported under the same alias. The
// alias is what scopes a member use, so this is the shape that would cross the
// wires if it did not: the spelling `stats.mean(` is identical in both files
// and refers to different functions.
func TestCallersDoNotCrossBetweenModulesWithTheSameMemberName(t *testing.T) {
	root := writeModules(t, map[string]string{
		"a.mut": "let mean = fn(xs) { return 1; };\n",
		"b.mut": "let mean = fn(xs) { return 2; };\n",
		"callsA.mut": "import stats \"a.mut\";\n" +
			"let fromA = fn() { return stats.mean([1]); };\n" +
			"fromA();\n",
		"callsB.mut": "import stats \"b.mut\";\n" +
			"let fromB = fn() { return stats.mean([2]); };\n" +
			"fromB();\n",
	})
	s, _ := serverOver(t, root, "callsA.mut")

	if got := incomingNames(t, s, prepareAt(t, s, pathToURI(root+"/a.mut"), 0, 4)); !equalStrings(got, []string{"fromA"}) {
		t.Fatalf("callers of a.mut's mean = %v, want [fromA]", got)
	}
	if got := incomingNames(t, s, prepareAt(t, s, pathToURI(root+"/b.mut"), 0, 4)); !equalStrings(got, []string{"fromB"}) {
		t.Fatalf("callers of b.mut's mean = %v, want [fromB]", got)
	}
}

// A call written inside an anonymous function belongs to the declaration around
// it. The literal declares nothing, so naming it as the caller would hand the
// editor an item with nowhere to go -- and the user is looking for the function
// they can open, not for the closure they passed to map.
func TestACallInsideAnAnonymousFunctionIsAttributedToTheDeclarationAroundIt(t *testing.T) {
	root := writeModules(t, map[string]string{
		"lib.mut": "let mean = fn(xs) { return 0; };\n",
		"main.mut": "import stats \"lib.mut\";\n" +
			"let report = fn(rows) { return map(rows, fn(row) { return stats.mean(row); }); };\n" +
			"report([[1]]);\n",
	})
	s, _ := serverOver(t, root, "main.mut")

	calls, err := s.callHierarchyIncomingCalls(nil, &lsp.CallHierarchyIncomingCallsParams{
		Item: prepareAt(t, s, pathToURI(root+"/lib.mut"), 0, 4),
	})
	if err != nil {
		t.Fatalf("incomingCalls: %v", err)
	}
	if len(calls) != 1 {
		t.Fatalf("callers = %d, want 1: %+v", len(calls), calls)
	}
	if calls[0].From.Name != "report" {
		t.Fatalf("the caller is %q (a %v), want report -- the anonymous function "+
			"declares nothing to navigate to", calls[0].From.Name, calls[0].From.Kind)
	}
}

// An item's Range is the declaration; its SelectionRange is the name inside it,
// which the protocol requires the range to contain. Both come from the graph,
// and they were the same few characters -- FullRange held the name rather than
// the statement -- so the peek an editor opens on a call hierarchy entry framed
// six characters of a function instead of the function.
func TestACallHierarchyItemCoversTheWholeDeclarationNotJustTheName(t *testing.T) {
	root := writeModules(t, map[string]string{
		"main.mut": "let target = fn() {\n\tlet n = 1;\n\treturn n;\n};\n" +
			"target();\n",
	})
	s, uri := serverOver(t, root, "main.mut")

	item := prepareAt(t, s, uri, 0, 4)
	if item.Name != "target" {
		t.Fatalf("prepare resolved to %q, want target", item.Name)
	}

	if item.SelectionRange.Start.Line != 0 || item.SelectionRange.Start.Character != 4 {
		t.Fatalf("selectionRange should be the name `target`; got %+v", item.SelectionRange)
	}
	if item.Range == item.SelectionRange {
		t.Fatalf("range and selectionRange are both %+v -- an item's range is the "+
			"declaration the name sits in, and a range equal to the name tells the "+
			"editor the function is six characters long", item.Range)
	}
	// `};` closes the function on the fourth line, which is line 3 to the protocol.
	if item.Range.End.Line < 3 {
		t.Fatalf("the declaration ends on line 3 and the item's range ends on line "+
			"%d, so it stops short of the body", item.Range.End.Line)
	}
	if item.Range.Start.Line > item.SelectionRange.Start.Line ||
		item.Range.End.Line < item.SelectionRange.End.Line {
		t.Fatalf("selectionRange %+v is not contained by range %+v, which the "+
			"protocol requires", item.SelectionRange, item.Range)
	}
}

// A file's top level is the caller of most calls in a script. The graph records
// those with From == nil, and the analyzer's IncomingCalls skipped every one of
// them while its comment said they were reported under the file
// (M26-SEM-012). The server names that caller after the file, as it already
// did for another file's top level.

func incomingCallsOf(t *testing.T, s *Server, item lsp.CallHierarchyItem) []lsp.CallHierarchyIncomingCall {
	t.Helper()
	calls, err := s.callHierarchyIncomingCalls(nil, &lsp.CallHierarchyIncomingCallsParams{Item: item})
	if err != nil {
		t.Fatalf("incomingCalls: %v", err)
	}
	return calls
}

// fileLevelCaller is the one caller among calls that is a file, and how many
// such callers there were.
func fileLevelCaller(calls []lsp.CallHierarchyIncomingCall) (lsp.CallHierarchyIncomingCall, int) {
	var file lsp.CallHierarchyIncomingCall
	count := 0
	for _, call := range calls {
		if call.From.Kind == lsp.SymbolKindFile {
			file = call
			count++
		}
	}
	return file, count
}

func TestACallAtTheTopLevelOfTheSameFileIsAttributedToTheFile(t *testing.T) {
	root := writeModules(t, map[string]string{
		"main.mut": "let helper = fn() { return 1; };\n" +
			"let main = fn() { return helper(); };\n" +
			"helper();\n" +
			"helper();\n",
	})
	s, uri := serverOver(t, root, "main.mut")

	item := prepareAt(t, s, uri, 0, 4)
	if item.Name != "helper" {
		t.Fatalf("prepare resolved to %q, want helper", item.Name)
	}
	calls := incomingCallsOf(t, s, item)

	total := 0
	for _, call := range calls {
		total += len(call.FromRanges)
	}
	if total != 3 {
		t.Errorf("call sites = %d, want 3: helper is called once inside main and twice at the top level", total)
	}
	file, files := fileLevelCaller(calls)
	if files != 1 {
		t.Fatalf("file-level callers = %d, want 1: the two top-level calls are made by main.mut", files)
	}
	if file.From.Name != "main.mut" || file.From.URI != uri || len(file.FromRanges) != 2 {
		t.Errorf("file caller = %q at %s with %d sites, want main.mut at %s with 2",
			file.From.Name, file.From.URI, len(file.FromRanges), uri)
	}
	// The sites are the two `helper` names on lines 2 and 3.
	for i, line := range []lsp.UInteger{2, 3} {
		if i < len(file.FromRanges) && (file.FromRanges[i].Start.Line != line || file.FromRanges[i].Start.Character != 0) {
			t.Errorf("site %d = %+v, want line %d character 0", i, file.FromRanges[i], line)
		}
	}
}

// TestACallInsideATopLevelClosureIsAttributedToTheFile: an anonymous function
// declares nothing, so a call inside one written at the top level belongs to
// the file, as one inside a declaration belongs to the declaration.
func TestACallInsideATopLevelClosureIsAttributedToTheFile(t *testing.T) {
	root := writeModules(t, map[string]string{
		"main.mut": "let helper = fn(x) { return x; };\n" +
			"map([1], fn(x) { return helper(x); });\n",
	})
	s, uri := serverOver(t, root, "main.mut")

	calls := incomingCallsOf(t, s, prepareAt(t, s, uri, 0, 4))
	if _, files := fileLevelCaller(calls); len(calls) != 1 || files != 1 {
		t.Errorf("callers = %d (%d file-level), want exactly one, the file", len(calls), files)
	}
}

// TestTopLevelCallsOnTwoLinesOfAnotherFileAreOneCaller: the cross-file half
// grouped a file caller by the line of each call, so one file calling from two
// lines was listed as two callers of the same name.
func TestTopLevelCallsOnTwoLinesOfAnotherFileAreOneCaller(t *testing.T) {
	root := writeModules(t, map[string]string{
		"lib.mut": "let mean = fn(xs) { return 0; };\n",
		"main.mut": "import stats \"lib.mut\";\n" +
			"stats.mean([1]);\n" +
			"stats.mean([2]);\n",
	})
	s, _ := serverOver(t, root, "main.mut")

	calls := incomingCallsOf(t, s, prepareAt(t, s, pathToURI(root+"/lib.mut"), 0, 4))
	if len(calls) != 1 || len(calls[0].FromRanges) != 2 {
		t.Errorf("callers = %d, want 1 (main.mut) with 2 sites", len(calls))
	}
}

// TestAFileCallerIsCalledByNothingAndCallsWhatItsTopLevelCalls: the editor
// hands a caller item back to expand it. A file item's range is a call site, so
// resolving it by position answered for the function called there: its callers
// came back as the file's callers, and its callees as the file's callees.
func TestAFileCallerIsCalledByNothingAndCallsWhatItsTopLevelCalls(t *testing.T) {
	root := writeModules(t, map[string]string{
		"lib.mut": "let mean = fn(xs) { return 0; };\n",
		"main.mut": "import stats \"lib.mut\";\n" +
			"let other = fn() { return 1; };\n" +
			"let helper = fn() { return other(); };\n" +
			"stats.mean([1]);\n" +
			"helper();\n",
	})
	s, _ := serverOver(t, root, "main.mut")

	calls := incomingCallsOf(t, s, prepareAt(t, s, pathToURI(root+"/lib.mut"), 0, 4))
	file, files := fileLevelCaller(calls)
	if files != 1 {
		t.Fatalf("file-level callers of mean = %d, want 1", files)
	}

	if in := incomingCallsOf(t, s, file.From); len(in) != 0 {
		t.Errorf("the file item has %d incoming callers, want none: nothing calls a file", len(in))
	}

	out, err := s.callHierarchyOutgoingCalls(nil, &lsp.CallHierarchyOutgoingCallsParams{Item: file.From})
	if err != nil {
		t.Fatalf("outgoingCalls: %v", err)
	}
	names := make([]string, 0, len(out))
	for _, call := range out {
		names = append(names, call.To.Name)
	}
	// The top level calls helper, a declaration of this file; other is called
	// by helper, not by the file. stats.mean is a cross-module callee, which
	// outgoing calls do not list for any item yet.
	if len(names) != 1 || names[0] != "helper" {
		t.Errorf("the file item's callees = %v, want [helper]", names)
	}
}
