package builtin

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"mutant/object"
)

// bin_yara_scan with caseInsensitive=true must report byte offsets into the file,
// as the case-sensitive scan does (M26-ART-027). It used to lower the whole file
// with strings.ToLower and report offsets into the lowered text, which is a
// different length from the file: strings.ToLower writes the three bytes of U+FFFD
// for each byte that is not valid UTF-8, and some runes lower to a rune of another
// length. The same U+FFFD also let a rule byte that is not valid UTF-8 match every
// such byte in the file.

// The non-ASCII runes the tests use, built from their numbers so that nothing on
// the way to disk can rewrite an escape. The first three lower to a rune of
// another UTF-8 length.
var (
	yaraFoldKelvin       = string(rune(0x212A)) // KELVIN SIGN, 3 bytes; lowers to "k"
	yaraFoldDottedI      = string(rune(0x0130)) // LATIN CAPITAL I WITH DOT ABOVE, 2 bytes; lowers to "i"
	yaraFoldAStroke      = string(rune(0x023A)) // LATIN CAPITAL A WITH STROKE, 2 bytes; lowers to U+2C65
	yaraFoldAStrokeLower = string(rune(0x2C65)) // LATIN SMALL A WITH STROKE, 3 bytes
	yaraFoldEAcute       = string(rune(0x00E9))
	yaraFoldEAcuteUpper  = string(rune(0x00C9))
)

// yaraFoldScan writes data to dir/scan.bin, scans it with rules, and returns each
// matched rule's offsets and the scan's total_hits.
func yaraFoldScan(t *testing.T, dir string, data []byte, caseInsensitive bool, rules ...string) (map[string][]int, int) {
	t.Helper()
	path := filepath.Join(dir, "scan.bin")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	elems := make([]object.Object, len(rules))
	for i, rule := range rules {
		elems[i] = stringObj(rule)
	}
	args := []object.Object{stringObj(path), &object.Array{Elements: elems}}
	if caseInsensitive {
		args = append(args, boolObj(true))
	}
	res, errObj := unwrapPair(t, BinYaraScan(args...))
	if errObj != nil {
		t.Fatalf("bin_yara_scan error: %s", errObj.Inspect())
	}
	h, ok := res.(*object.Hash)
	if !ok {
		t.Fatalf("bin_yara_scan result is %T, want *object.Hash", res)
	}
	hits, ok := mustHashValue(t, h, "hits").(*object.Array)
	if !ok {
		t.Fatalf("hits is not an ARRAY")
	}
	out := map[string][]int{}
	for _, hit := range hits.Elements {
		hh, ok := hit.(*object.Hash)
		if !ok {
			t.Fatalf("hit is %T, want *object.Hash", hit)
		}
		offs, ok := mustHashValue(t, hh, "offsets").(*object.Array)
		if !ok {
			t.Fatalf("offsets is not an ARRAY")
		}
		got := make([]int, len(offs.Elements))
		for i, o := range offs.Elements {
			n, ok := o.(*object.Integer)
			if !ok {
				t.Fatalf("offset is %T, want *object.Integer", o)
			}
			got[i] = int(n.Value)
		}
		if count := mustHashIntValue(t, hh, "count"); count != int64(len(got)) {
			t.Fatalf("count %d disagrees with %d offsets", count, len(got))
		}
		out[mustHashStringValue(t, hh, "rule")] = got
	}
	return out, int(mustHashIntValue(t, h, "total_hits"))
}

func yaraFoldWantOffsets(t *testing.T, what string, got []int, want ...int) {
	t.Helper()
	got, want = yaraFoldNilIfEmpty(got), yaraFoldNilIfEmpty(want)
	if reflect.DeepEqual(got, want) {
		return
	}
	if len(got) <= 10 && len(want) <= 10 {
		t.Errorf("%s: offsets = %v, want %v", what, got, want)
		return
	}
	i := 0
	for i < len(got) && i < len(want) && got[i] == want[i] {
		i++
	}
	t.Errorf("%s: %d offsets, want %d; from index %d they are %v, want %v",
		what, len(got), len(want), i, got[i:min(i+5, len(got))], want[i:min(i+5, len(want))])
}

func TestBinYaraScanFoldReportsFileOffsets(t *testing.T) {
	dir := t.TempDir()
	// The row's fixture: FF FE 00 'ABC' 80 'abc'. Three bytes that are not valid
	// UTF-8 come before the second match, two before the first.
	data := []byte{0xFF, 0xFE, 0x00, 'A', 'B', 'C', 0x80, 'a', 'b', 'c'}

	cs, _ := yaraFoldScan(t, dir, data, false, "abc")
	yaraFoldWantOffsets(t, "case-sensitive abc", cs["abc"], 7)

	ci, total := yaraFoldScan(t, dir, data, true, "abc", "ABC", "aBc")
	yaraFoldWantOffsets(t, "case-insensitive abc", ci["abc"], 3, 7)
	yaraFoldWantOffsets(t, "case-insensitive ABC", ci["ABC"], 3, 7)
	yaraFoldWantOffsets(t, "case-insensitive aBc", ci["aBc"], 3, 7)
	if total != 6 {
		t.Errorf("total_hits = %d, want 6", total)
	}
}

func TestBinYaraScanFoldMatchesRuleBytesExactly(t *testing.T) {
	dir := t.TempDir()
	data := []byte{0xFF, 0xFE, 0x00, 'A', 'B', 'C', 0x80, 'a', 'b', 'c'}
	ff := string([]byte{0xFF})
	x80 := string([]byte{0x80})
	feNul := string([]byte{0xFE, 0x00})

	// Each of these bytes is in the file once. Lowering with strings.ToLower
	// turned FF, FE and 80, in the rules and in the file, into U+FFFD, so a lone
	// FF or 80 matched all three of them, and every offset was counted in the
	// lowered text.
	ci, total := yaraFoldScan(t, dir, data, true, ff, x80, x80+"ABC", feNul+"a")
	yaraFoldWantOffsets(t, "case-insensitive FF", ci[ff], 0)
	yaraFoldWantOffsets(t, "case-insensitive 80", ci[x80], 6)
	yaraFoldWantOffsets(t, "case-insensitive 80 'ABC'", ci[x80+"ABC"], 6)
	yaraFoldWantOffsets(t, "case-insensitive FE 00 'a'", ci[feNul+"a"], 1)
	if total != 4 {
		t.Errorf("total_hits = %d, want 4", total)
	}

	// U+FFFD in a rule is a rune like any other: it matches a U+FFFD written in
	// the file, never a byte that is not valid UTF-8.
	replacement := string(utf8.RuneError)
	ci, total = yaraFoldScan(t, dir, data, true, replacement)
	if total != 0 {
		t.Errorf("U+FFFD matched bytes that are not valid UTF-8: %v", ci[replacement])
	}
	withReplacement := append(append([]byte{0xFF, 'a'}, replacement...), 'A')
	ci, _ = yaraFoldScan(t, dir, withReplacement, true, replacement, replacement+"a")
	yaraFoldWantOffsets(t, "case-insensitive U+FFFD", ci[replacement], 2)
	yaraFoldWantOffsets(t, "case-insensitive U+FFFD 'a'", ci[replacement+"a"], 2)

	// A rule byte matches the same byte inside a valid sequence of the file,
	// as the case-sensitive scan does: 'x' then U+00E9, written as C3 A9.
	inside := append([]byte("x"), yaraFoldEAcute...)
	lead, cont := string([]byte{0xC3}), string([]byte{0xA9})
	cs, _ := yaraFoldScan(t, dir, inside, false, lead, cont)
	ci, _ = yaraFoldScan(t, dir, inside, true, lead, cont)
	for _, rule := range []string{lead, cont} {
		yaraFoldWantOffsets(t, "case-insensitive lone byte", ci[rule], cs[rule]...)
	}
	yaraFoldWantOffsets(t, "case-insensitive C3", ci[lead], 1)
	yaraFoldWantOffsets(t, "case-insensitive A9", ci[cont], 2)
}

func TestBinYaraScanFoldWidthChangingRunes(t *testing.T) {
	dir := t.TempDir()
	// Offsets: KELVIN SIGN 0-2, "XYZ" 3-5, "xyz" 6-8, U+0130 9-10, "x" 11,
	// U+023A 12-13, "XYZ" 14-16. All of it is valid UTF-8. The lowered text runs
	// two bytes behind the file after the KELVIN SIGN, three after U+0130, and
	// two again after U+023A, whose lowercase is a byte longer.
	data := []byte(yaraFoldKelvin + "XYZxyz" + yaraFoldDottedI + "x" + yaraFoldAStroke + "XYZ")

	ci, _ := yaraFoldScan(t, dir, data, true, "xyz", "k", "ix", yaraFoldAStrokeLower+"xyz", yaraFoldKelvin)
	yaraFoldWantOffsets(t, "xyz", ci["xyz"], 3, 6, 14)
	yaraFoldWantOffsets(t, "k (the KELVIN SIGN lowers to k)", ci["k"], 0)
	yaraFoldWantOffsets(t, "ix (U+0130 lowers to i)", ci["ix"], 9)
	yaraFoldWantOffsets(t, "U+2C65 xyz", ci[yaraFoldAStrokeLower+"xyz"], 12)
	yaraFoldWantOffsets(t, "KELVIN SIGN", ci[yaraFoldKelvin], 0)
}

// Thousands of runes that change length, so the lowered text drifts far from the
// file in both directions, with matches spread over many kilobytes: far apart,
// and packed together.
func TestBinYaraScanFoldMapsMatchesFarIntoTheFile(t *testing.T) {
	dir := t.TempDir()
	var sb strings.Builder
	sb.WriteString(strings.Repeat(yaraFoldKelvin, 1500))       // 0-4499, lowers to 1500 bytes
	sb.WriteString("abc")                                      // 4500
	sb.WriteString(strings.Repeat(yaraFoldAStroke, 2000))      // 4503-8502, lowers to 6000 bytes
	sb.WriteString("ABC")                                      // 8503
	sb.WriteString(strings.Repeat(string([]byte{0xFF}), 3000)) // 8506-11505, kept as they are
	sb.WriteString("aBcAbC")                                   // 11506 and 11509
	data := []byte(sb.String())

	kk := yaraFoldKelvin + yaraFoldKelvin
	cAStroke := "c" + yaraFoldAStrokeLower
	ffA := string([]byte{0xFF}) + "A"
	ci, _ := yaraFoldScan(t, dir, data, true, "abc", kk, cAStroke, ffA)
	yaraFoldWantOffsets(t, "abc", ci["abc"], 4500, 8503, 11506, 11509)
	// Two KELVIN SIGNs lower to "kk": 750 matches, one every six bytes.
	every6 := make([]int, 750)
	for i := range every6 {
		every6[i] = 6 * i
	}
	yaraFoldWantOffsets(t, "two KELVIN SIGNs", ci[kk], every6...)
	yaraFoldWantOffsets(t, "c U+2C65", ci[cAStroke], 4502)
	yaraFoldWantOffsets(t, "FF 'A'", ci[ffA], 11505)
	for _, rule := range []string{"abc", kk, cAStroke, ffA} {
		yaraFoldWantOffsets(t, fmt.Sprintf("rule % x against the definition", rule), ci[rule], yaraFoldDefinition(data, rule)...)
	}
}

// The lowering itself is the one strings.ToLower applies, so text that was
// matched correctly before is matched the same way now.
func TestBinYaraScanFoldKeepsUnicodeLowering(t *testing.T) {
	dir := t.TempDir()
	// U+00E9 "cole " U+00C9 "COLE": the second word starts at byte 7, as each
	// accented letter is two bytes.
	data := []byte(yaraFoldEAcute + "cole " + yaraFoldEAcuteUpper + "COLE")
	ci, total := yaraFoldScan(t, dir, data, true, yaraFoldEAcuteUpper+"COLE", yaraFoldEAcute+"cole")
	yaraFoldWantOffsets(t, "U+00C9 COLE", ci[yaraFoldEAcuteUpper+"COLE"], 0, 7)
	yaraFoldWantOffsets(t, "U+00E9 cole", ci[yaraFoldEAcute+"cole"], 0, 7)
	if total != 4 {
		t.Errorf("total_hits = %d, want 4", total)
	}
}

// yaraFoldDefinition is what a case-insensitive scan has to find, worked out the
// slow way: at each offset, walk the rule one piece at a time. A rule byte that
// is not part of valid UTF-8 matches only that same byte of the file. A rule rune
// matches a valid UTF-8 sequence of the file whose rune lowers (unicode.ToLower)
// to the rune the rule's rune lowers to. Matches do not overlap.
func yaraFoldDefinition(data []byte, rule string) []int {
	var out []int
	for at := 0; at < len(data); {
		end, ok := yaraFoldDefinitionAt(data, at, rule)
		if !ok {
			at++
			continue
		}
		out = append(out, at)
		at = end
	}
	return out
}

func yaraFoldDefinitionAt(data []byte, at int, rule string) (int, bool) {
	pos := at
	for k := 0; k < len(rule); {
		if pos >= len(data) {
			return 0, false
		}
		r, width := utf8.DecodeRuneInString(rule[k:])
		if r == utf8.RuneError && width == 1 {
			if data[pos] != rule[k] {
				return 0, false
			}
			pos, k = pos+1, k+1
			continue
		}
		d, dataWidth := utf8.DecodeRune(data[pos:])
		if d == utf8.RuneError && dataWidth == 1 {
			return 0, false
		}
		if unicode.ToLower(d) != unicode.ToLower(r) {
			return 0, false
		}
		pos, k = pos+dataWidth, k+width
	}
	return pos, true
}

func yaraFoldHasWidthChange(data []byte) bool {
	for len(data) > 0 {
		r, width := utf8.DecodeRune(data)
		if utf8.RuneLen(unicode.ToLower(r)) != width {
			return true
		}
		data = data[width:]
	}
	return false
}

// TestBinYaraScanFoldAgreesWithDefinition checks the scan against
// yaraFoldDefinition on files made of the pieces that broke the old lowering --
// bytes that are not valid UTF-8, a written U+FFFD, runes whose lowercase has
// another length -- and of letters with more than two forms. One rule per file
// is two random pieces; the rest are cut from the file at any byte, so many are
// not valid UTF-8 either, and have their ASCII case flipped at random.
func TestBinYaraScanFoldAgreesWithDefinition(t *testing.T) {
	dir := t.TempDir()
	pieces := []string{
		"a", "A", "b", "B", "k", "K", "i", "I", "s", "S", " ",
		yaraFoldKelvin, yaraFoldDottedI, yaraFoldAStroke, yaraFoldAStrokeLower,
		yaraFoldEAcute, yaraFoldEAcuteUpper,
		// LATIN SMALL LONG S, which lowers to itself, not to s.
		string(rune(0x017F)),
		// Capital, small and final sigma.
		string(rune(0x03A3)), string(rune(0x03C3)), string(rune(0x03C2)),
		string(utf8.RuneError),
		string([]byte{0xFF}), string([]byte{0x80}), string([]byte{0xC3}), string([]byte{0xA9}),
		string([]byte{0xE2, 0x84}), string([]byte{0x00}),
	}
	rng := rand.New(rand.NewSource(27))
	checked, unchangedChecked := 0, 0
	for c := 0; c < 150; c++ {
		var sb strings.Builder
		for n := rng.Intn(20); n > 0; n-- {
			sb.WriteString(pieces[rng.Intn(len(pieces))])
		}
		data := []byte(sb.String())

		rules := []string{pieces[rng.Intn(len(pieces))] + pieces[rng.Intn(len(pieces))]}
		for n := 0; n < 3 && len(data) > 0; n++ {
			i := rng.Intn(len(data))
			j := i + 1 + rng.Intn(len(data)-i)
			cut := append([]byte(nil), data[i:j]...)
			for x, b := range cut {
				if b < utf8.RuneSelf && unicode.IsLetter(rune(b)) && rng.Intn(2) == 0 {
					cut[x] ^= 0x20
				}
			}
			rules = append(rules, string(cut))
		}

		got, _ := yaraFoldScan(t, dir, data, true, rules...)
		for _, rule := range rules {
			want := yaraFoldDefinition(data, rule)
			if !reflect.DeepEqual(yaraFoldNilIfEmpty(got[rule]), yaraFoldNilIfEmpty(want)) {
				t.Fatalf("data % x, rule % x: offsets = %v, want %v", data, rule, got[rule], want)
			}
			checked++
			// Where lowering keeps every length and the text is valid UTF-8,
			// offsets into strings.ToLower's output were already file offsets:
			// those answers must not have changed.
			if utf8.Valid(data) && utf8.ValidString(rule) && !yaraFoldHasWidthChange(data) {
				old := allSubstringOffsets(strings.ToLower(string(data)), strings.ToLower(rule))
				if !reflect.DeepEqual(yaraFoldNilIfEmpty(got[rule]), yaraFoldNilIfEmpty(old)) {
					t.Fatalf("data % x, rule % x: offsets = %v, strings.ToLower gave %v", data, rule, got[rule], old)
				}
				unchangedChecked++
			}
		}
	}
	t.Logf("%d rules checked against the definition, %d of them against strings.ToLower", checked, unchangedChecked)
	if checked < 450 || unchangedChecked < 20 {
		t.Fatalf("too few cases exercised: %d rules, %d against strings.ToLower", checked, unchangedChecked)
	}
}

func yaraFoldNilIfEmpty(v []int) []int {
	if len(v) == 0 {
		return nil
	}
	return v
}
