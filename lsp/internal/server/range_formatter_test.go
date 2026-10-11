package server

import (
	"strings"
	"testing"

	"mutant/ast"
	"mutant/lsp/internal/analyzer"
	localprotocol "mutant/lsp/internal/protocol"

	lsp "github.com/tliron/glsp/protocol_3_16"
)

func selection(startLine, endLine int) lsp.Range {
	return lsp.Range{
		Start: lsp.Position{Line: lsp.UInteger(startLine), Character: 0},
		End:   lsp.Position{Line: lsp.UInteger(endLine), Character: 0},
	}
}

// applyEdits applies non-overlapping line edits to text, last-first so that
// earlier ranges stay valid.
func applyEdits(t *testing.T, text string, edits []lsp.TextEdit) string {
	t.Helper()

	lines := splitLines(text)
	offsetOf := func(pos lsp.Position) int {
		offset := 0
		for i := 0; i < int(pos.Line) && i < len(lines); i++ {
			offset += len(lines[i])
		}
		return offset + int(pos.Character)
	}

	type resolved struct {
		start, end int
		newText    string
	}
	items := make([]resolved, 0, len(edits))
	for _, edit := range edits {
		items = append(items, resolved{offsetOf(edit.Range.Start), offsetOf(edit.Range.End), edit.NewText})
	}

	for i := len(items) - 1; i >= 0; i-- {
		item := items[i]
		if item.start > len(text) || item.end > len(text) || item.start > item.end {
			t.Fatalf("edit out of bounds: %+v for text of length %d", item, len(text))
		}
		text = text[:item.start] + item.newText + text[item.end:]
	}
	return text
}

func TestRangeFormattingOnlyTouchesSelectedLines(t *testing.T) {
	original := "let a=1;\nlet b=2;\nlet c=3;\n"
	formatted := "let a = 1;\nlet b = 2;\nlet c = 3;\n"

	// Select the middle line only.
	edits := rangeFormattingEdits(original, formatted, selection(1, 1))
	if len(edits) == 0 {
		t.Fatal("expected at least one edit")
	}

	got := applyEdits(t, original, edits)
	want := "let a=1;\nlet b = 2;\nlet c=3;\n"
	if got != want {
		t.Errorf("range formatted = %q, want %q", got, want)
	}
}

func TestRangeFormattingLeavesDocumentUnchangedForEmptySelection(t *testing.T) {
	original := "let a=1;\nlet b=2;\n"
	formatted := "let a = 1;\nlet b = 2;\n"

	// A selection covering no changed line yields no edits. Line 5 does not
	// exist in this document.
	if edits := rangeFormattingEdits(original, formatted, selection(5, 5)); len(edits) != 0 {
		t.Errorf("expected no edits, got %d", len(edits))
	}
}

func TestRangeFormattingCoveringWholeDocumentMatchesFullFormat(t *testing.T) {
	original := "let a=1;\nlet b=2;\nlet c=3;\n"
	formatted := "let a = 1;\nlet b = 2;\nlet c = 3;\n"

	edits := rangeFormattingEdits(original, formatted, selection(0, 10))
	if got := applyEdits(t, original, edits); got != formatted {
		t.Errorf("full-range formatting = %q, want %q", got, formatted)
	}
}

func TestRangeFormattingReturnsNilWhenAlreadyFormatted(t *testing.T) {
	text := "let a = 1;\n"
	if edits := rangeFormattingEdits(text, text, selection(0, 5)); edits != nil {
		t.Errorf("expected nil edits for already-formatted text, got %v", edits)
	}
}

func TestRangeFormattingHandlesLineCountChanges(t *testing.T) {
	// Formatting collapses three lines into one.
	original := "let a=1;\nif (x)\n{\ny;\n}\n"
	formatted := "let a = 1;\nif (x) {\n    y;\n}\n"

	// Selecting everything must reproduce the full format exactly.
	edits := rangeFormattingEdits(original, formatted, selection(0, 20))
	if got := applyEdits(t, original, edits); got != formatted {
		t.Errorf("formatted = %q, want %q", got, formatted)
	}
}

func TestRangeFormattingSkipsHunksStraddlingSelection(t *testing.T) {
	// A single hunk spans lines 1-3; selecting only line 2 must skip it
	// rather than applying half of it.
	original := "let a = 1;\nif (x)\n{\ny;\n}\n"
	formatted := "let a = 1;\nif (x) {\n    y;\n}\n"

	edits := rangeFormattingEdits(original, formatted, selection(2, 2))
	got := applyEdits(t, original, edits)

	// Whatever it did, it must not have corrupted lines outside the selection.
	if !strings.HasPrefix(got, "let a = 1;\n") {
		t.Errorf("line 0 was modified: %q", got)
	}
}

func TestSplitLinesRoundTrips(t *testing.T) {
	inputs := []string{
		"",
		"a",
		"a\n",
		"a\nb",
		"a\nb\n",
		"\n",
		"\n\n",
		"a\n\nb\n",
	}

	for _, input := range inputs {
		if got := strings.Join(splitLines(input), ""); got != input {
			t.Errorf("splitLines(%q) rejoined to %q", input, got)
		}
	}
}

func TestDiffLinesProducesNoHunksForIdenticalInput(t *testing.T) {
	lines := splitLines("a\nb\nc\n")
	if hunks := diffLines(lines, lines); len(hunks) != 0 {
		t.Errorf("expected no hunks, got %+v", hunks)
	}
}

func TestDiffLinesReconstructsTarget(t *testing.T) {
	cases := []struct{ from, to string }{
		{"a\nb\nc\n", "a\nB\nc\n"},
		{"a\nb\nc\n", "a\nc\n"},
		{"a\nc\n", "a\nb\nc\n"},
		{"a\n", "b\n"},
		{"a\nb\nc\nd\ne\n", "a\nx\ny\ne\n"},
		{"", "a\n"},
		{"a\n", ""},
		{"a\nb\n", "b\na\n"},
	}

	for _, tc := range cases {
		from := splitLines(tc.from)
		to := splitLines(tc.to)

		// Applying every hunk must reproduce the target exactly.
		hunks := diffLines(from, to)
		result := make([]string, 0, len(to))
		cursor := 0
		for _, hunk := range hunks {
			result = append(result, from[cursor:hunk.startLine]...)
			result = append(result, hunk.newLines...)
			cursor = hunk.endLine
		}
		result = append(result, from[cursor:]...)

		if got := strings.Join(result, ""); got != tc.to {
			t.Errorf("diff %q -> %q reconstructed %q", tc.from, tc.to, got)
		}
	}
}

// rangeFormatted is the text the client holds after range-formatting src over
// sel: the whole document is formatted, and only the edits the selection
// allows are applied.
func rangeFormatted(t *testing.T, src string, sel lsp.Range) string {
	t.Helper()
	return applyEdits(t, src, rangeFormattingEdits(src, format(t, src), sel))
}

// throughEndOf is the range an editor sends for a selection that runs from the
// start of line first to the end of the text on line last. Its end is inside
// line last, not at character 0 of the line after it, so the lines it names do
// not depend on how an end at character 0 is read (M26-LSP-039).
func throughEndOf(t *testing.T, src string, first, last int) lsp.Range {
	t.Helper()
	text := strings.TrimRight(splitLines(src)[last], "\n")
	if text == "" {
		t.Fatalf("line %d of %q is empty, so no range can end inside it", last, src)
	}
	return lsp.Range{
		Start: lsp.Position{Line: lsp.UInteger(first), Character: 0},
		End:   lsp.Position{Line: lsp.UInteger(last), Character: lsp.UInteger(localprotocol.UTF16Len(text))},
	}
}

// programDifference says how got differs from src as a program, or returns ""
// when it is the same program with the same comments. A clean parse is not
// enough: the parser accepts `f(1,\ng(2);` as `f(1, g(2))`, so a rewrite can
// parse and still be a different program.
func programDifference(src, got string) string {
	before := analyzer.New().Analyze(src)
	after := analyzer.New().Analyze(got)
	if len(after.ParseErrors) > 0 {
		return "does not parse: " + after.ParseErrors[0].Msg
	}
	if after.Program == nil {
		return "does not parse"
	}
	if !ast.Equal(before.Program, after.Program) {
		return "parses to a different program"
	}
	if strings.Join(commentBodies(src), "\x00") != strings.Join(commentBodies(got), "\x00") {
		return "changes the comments"
	}
	return ""
}

// TestRangeFormattingNeverAppliesPartOfAChangedRegion is M26-LSP-007. The edits
// come from a line diff, which knows nothing about statements. The formatter
// joins `max(\n    1,\n    2)` onto one line, and the diff pairs line 0 with
// the joined line and deletes lines 1 and 2 in a hunk of their own. Selecting
// line 0 applied the first hunk alone and left a program that did not parse;
// selecting lines 1 and 2 applied the second alone and left
// `let x = max(\nputln(x);`, which parses, as `let x = max(putln(x));`. Part
// of a change is applied now only when that leaves the same program with the
// same comments, so neither selection gets anything, and a selection that
// holds the whole statement still gets all of it.
func TestRangeFormattingNeverAppliesPartOfAChangedRegion(t *testing.T) {
	src := "let x = max(\n    1,\n    2);\nputln(x);\n"
	for _, tc := range []struct {
		first, last int
		want        string
	}{
		{0, 0, src},
		{1, 2, src},
		{0, 2, "let x = max(1, 2);\nputln(x);\n"},
	} {
		if got := rangeFormatted(t, src, throughEndOf(t, src, tc.first, tc.last)); got != tc.want {
			t.Errorf("range-formatting lines %d..%d of %q gave %q, want %q", tc.first, tc.last, src, got, tc.want)
		}
	}

	// Every range from the start of one line to the start of the same line or
	// a later one, which between them name every run of whole lines, has to
	// leave the same program with the same comments: untouched, partly
	// formatted or wholly formatted, but never broken and never another
	// program.
	for _, fixture := range []string{
		// The fixture above.
		src,
		// The same line count on both sides of the change (2 -> 2), so
		// splitting only where the counts match would still split it.
		"let a = f(1,\n  2); let b = 3;\n",
		// A comment inside an argument list, which the printer moves.
		"putln(1,\n  // mid\n  2);\n",
		"putln(1, // mid\n  2);\nputln(3);\n",
		// A comment after an opening brace, which the printer moves into the
		// body. Formatting the line it was on by itself deleted it, and
		// formatting the line below by itself wrote it twice: the same program
		// both times, so only the comments tell.
		"let f = fn(x) { // doubles x\n    return x * 2; };\n",
		// A hash literal written over several lines.
		"let h = {\n\"a\": 1,\n\"b\": 2\n};\nputln(h);\n",
		// A one-line block the printer opens up, inside a block it keeps.
		"if (a) {\nif (b) { c; }\n}\n",
		"let f = fn() { if (x) { y; }\n};\n",
		// Respacing only, where every line stands alone.
		"let a=1;\nlet b=2;\nlet c=3;\n",
		// The shape in examples/automation/pipeline_end_to_end.mut: selecting
		// the blank line under a one-line block inserted the block's body and
		// closing brace there, without the line that opens the block.
		"let score = 25;\nif (has_enc) { score = score + 60; }\n\nputln(score);\n",
	} {
		lines := len(splitLines(fixture))
		for start := 0; start < lines; start++ {
			for end := start; end <= lines; end++ {
				got := rangeFormatted(t, fixture, selection(start, end))
				if problem := programDifference(fixture, got); problem != "" {
					t.Errorf("range-formatting %q over %d:0-%d:0 gave %q, which %s", fixture, start, end, got, problem)
				}
			}
		}
	}
}

// TestRangeFormattingStillFormatsALineThatStandsAlone is the other half of
// M26-LSP-007's fix: refusing part of a change must not refuse a line that can
// be formatted by itself. One respaced line among others still formats alone.
// So does a line the diff puts in one run of changed lines with the block
// below it, which the formatter reshapes. And so does a line selected together
// with the first line of a statement that can only be formatted whole, which
// is left as it was.
func TestRangeFormattingStillFormatsALineThatStandsAlone(t *testing.T) {
	for _, tc := range []struct {
		src         string
		first, last int
		want        string
	}{
		{"let a=1;\nlet b=2;\nlet c=3;\n", 1, 1, "let a=1;\nlet b = 2;\nlet c=3;\n"},
		{"let a=1;\nif (x)\n{\ny;\n}\n", 0, 0, "let a = 1;\nif (x)\n{\ny;\n}\n"},
		{
			"let a=1;\nputln(a);\nlet x = max(\n    1,\n    2);\nputln(x);\n", 0, 2,
			"let a = 1;\nputln(a);\nlet x = max(\n    1,\n    2);\nputln(x);\n",
		},
	} {
		if got := rangeFormatted(t, tc.src, throughEndOf(t, tc.src, tc.first, tc.last)); got != tc.want {
			t.Errorf("range-formatting lines %d..%d of %q gave %q, want %q", tc.first, tc.last, tc.src, got, tc.want)
		}
	}
}

// TestRangeFormattingSelectionEndIsExclusive is M26-LSP-039. The end of an LSP
// range is exclusive, and a selection of whole lines arrives ending at
// character 0 of the line after the last one, so 0:0-1:0 is line 0 alone. It
// formatted line 1 as well. A range that ends inside line 1 does take it in,
// and one that starts and ends at the start of the same line still names that
// line, which every selection(n, n) in this file relies on.
func TestRangeFormattingSelectionEndIsExclusive(t *testing.T) {
	src := "let a=1;\nlet b=2;\n"
	at := func(line, character lsp.UInteger) lsp.Position {
		return lsp.Position{Line: line, Character: character}
	}
	for _, tc := range []struct {
		sel  lsp.Range
		want string
	}{
		{lsp.Range{Start: at(0, 0), End: at(1, 0)}, "let a = 1;\nlet b=2;\n"},
		{lsp.Range{Start: at(0, 0), End: at(1, 1)}, "let a = 1;\nlet b = 2;\n"},
		{lsp.Range{Start: at(1, 0), End: at(1, 0)}, "let a=1;\nlet b = 2;\n"},
	} {
		if got := rangeFormatted(t, src, tc.sel); got != tc.want {
			t.Errorf("range-formatting %q over %d:%d-%d:%d gave %q, want %q", src,
				tc.sel.Start.Line, tc.sel.Start.Character, tc.sel.End.Line, tc.sel.End.Character, got, tc.want)
		}
	}
}
