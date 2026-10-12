package builtin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mutant/object"
)

// recordSealRangeClassEdit copies a range record_classify_range returned and
// overwrites the named fields, the way a script that edits a range by hand
// would.
func recordSealRangeClassEdit(t *testing.T, base *object.Hash, edits map[string]object.Object) object.Object {
	t.Helper()
	fields := map[string]object.Object{}
	for _, name := range []string{"offset", "length", "label", "class"} {
		value, present := hashLookup(base, name)
		if !present {
			t.Fatalf("record_classify_range returned no %q", name)
		}
		fields[name] = value
	}
	for name, value := range edits {
		fields[name] = value
	}
	return makeHashObject(fields)
}

// A range's label and its tag must name one declared class. record_seal used
// to seal by the tag alone, so a range whose label was edited still sealed
// under its old class, and a tag no class carries sealed as a class nobody
// declared. Each is refused, and the refusal names the range by number.
func TestRecordSealRangeClassMustMatchADeclaredLabel(t *testing.T) {
	recordTestCase(t)
	source, _, _ := recordFixture(t)
	dir := t.TempDir()

	open := mustHash(t, RecordClassifyRange(intObj(0), intObj(64), stringObj("open")))
	restricted := mustHash(t, RecordClassifyRange(intObj(100), intObj(20), stringObj("restricted")))
	restrictedTag := mustHashStringValue(t, restricted, "class")
	openTag := mustHashStringValue(t, open, "class")

	cases := []struct {
		name  string
		edits map[string]object.Object
		want  string
	}{
		{
			// The over-disclosure case: the author believes bytes 0-64 are
			// restricted, and before the fix they were sealed open.
			name:  "label edited to another declared class",
			edits: map[string]object.Object{"label": stringObj("restricted")},
			want:  "range 2",
		},
		{
			name:  "tag no declared class carries",
			edits: map[string]object.Object{"class": stringObj(strings.Repeat("ab", 32))},
			want:  "range 2",
		},
		{
			name: "label nobody declared",
			edits: map[string]object.Object{
				"label": stringObj("ultra-secret"), "class": stringObj(restrictedTag),
			},
			want: "range 2",
		},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dest := filepath.Join(dir, "edited"+string(rune('a'+i))+".mrec")
			edited := recordSealRangeClassEdit(t, open, tc.edits)
			_, errObj := unwrapPairNoFatal(RecordSeal(stringObj(source), stringObj(dest),
				recordArray(restricted, edited), recordSealOpts(nil)))
			if errObj == nil {
				t.Fatalf("a range whose label and tag do not name one declared class was sealed")
			}
			if !strings.Contains(errObj.Message, tc.want) {
				t.Fatalf("the refusal does not name the range: %s", errObj.Message)
			}
			if _, err := os.Stat(dest); !os.IsNotExist(err) {
				t.Fatal("the refused seal left a file behind")
			}
			t.Logf("refused: %s", errObj.Message)
		})
	}

	// The same edit through record_seal_quantised is refused too: both seal
	// through one range reader.
	quantisedDest := filepath.Join(dir, "quantised.mrec")
	_, errObj := unwrapPairNoFatal(RecordSealQuantised(stringObj(source), stringObj(quantisedDest),
		recordArray(recordSealRangeClassEdit(t, open, map[string]object.Object{"label": stringObj("restricted")})),
		recordSealOpts(map[string]object.Object{"quantum": intObj(16), "rounds_to": stringObj("restricted")})))
	if errObj == nil || !strings.Contains(errObj.Message, "range 1") {
		t.Fatalf("record_seal_quantised sealed an edited range or did not name it: %v", errObj)
	}

	// A range spelled with an upper-case tag still names its class, and is
	// sealed under the tag the case declared, so it joins the adjacent span
	// of that class rather than starting a second one.
	upperDest := filepath.Join(dir, "upper.mrec")
	upper := recordSealRangeClassEdit(t, open, map[string]object.Object{
		"class": stringObj(strings.ToUpper(openTag)),
	})
	sealed := mustHash(t, RecordSeal(stringObj(source), stringObj(upperDest),
		recordArray(restricted, upper), recordSealOpts(nil)))
	spans, ok := mustHashValue(t, sealed, "spans").(*object.Array)
	if !ok {
		t.Fatal("spans is not an ARRAY")
	}
	// open 0-100 (the range and the default merge), restricted 100-120,
	// open 120-200.
	if len(spans.Elements) != 3 {
		t.Fatalf("an upper-case tag was sealed as a class of its own: %d spans", len(spans.Elements))
	}
	first, ok := spans.Elements[0].(*object.Hash)
	if !ok {
		t.Fatal("a span is not a HASH")
	}
	if got := mustHashStringValue(t, first, "class"); got != openTag {
		t.Fatalf("the range was sealed under %s, not the declared tag %s", got, openTag)
	}
}
