package builtin

import (
	"strings"
	"testing"
	"unicode/utf8"

	"mutant/object"
)

// A position in a string means one of two things in this library. len and
// text_index count bytes; s[i], for-in, str_substr, str_char_at and the
// str_pad width count runes. The two agree up to the first character that
// takes more than one byte and differ after it. A byte that is not valid
// UTF-8 decodes as one rune, one byte wide, so it does not separate them. A
// program that hands a position from one side to the other cuts text in the
// wrong place once they differ, and nothing fails when it does (M26-BLT-012).
// Which unit text_index ought to count is a question for the owner. What
// cannot be open is that help and hover name the unit each one does count, and
// say where it parts from the other.

// blt012Text holds one two-byte character before the colon, so the colon is
// byte 5 and rune 4, and the whole string is 12 bytes and 11 runes.
const blt012Text = "José: admin"

// blt012Claim lists the phrases with which a summary and a return doc say what
// their own builtin counts. Both summaries name the other unit as well, to
// warn about s[i] and the str_* family, so a doc that merely mentions bytes or
// runes proves nothing. A doc that claims its unit in other words has to have
// them added here.
type blt012Claim struct{ summary, returns []string }

var blt012Claims = map[string]map[string]blt012Claim{
	BuiltinNameTextIndex: {
		"bytes": {summary: []string{"byte offset"}, returns: []string{"byte offset"}},
		"runes": {summary: []string{"rune offset", "character offset"}, returns: []string{"rune offset", "character offset"}},
	},
	BuiltinNameLen: {
		"bytes": {summary: []string{"measured in bytes"}, returns: []string{"bytes for a string"}},
		"runes": {summary: []string{"measured in runes", "measured in characters"}, returns: []string{"runes for a string", "characters for a string"}},
	},
}

// TestStringUnitsBLT012SummariesNameTheUnitTheyCount works out from the
// implementation which unit text_index and len count, then requires the
// summary and the return doc of each to claim that unit and not the other. It
// is held to the implementation rather than to either choice: change what one
// of them counts and both of its docs fail until they claim the new unit.
func TestStringUnitsBLT012SummariesNameTheUnitTheyCount(t *testing.T) {
	for _, name := range []string{BuiltinNameTextIndex, BuiltinNameLen} {
		unit, got := blt012UnitOf(t, name)
		other := "runes"
		if unit == "runes" {
			other = "bytes"
		}
		_, summary, _, ok := TeachingDoc(name)
		if !ok {
			t.Fatalf("%s has no teaching doc", name)
		}
		returns, ok := ReturnSpec(name)
		if !ok {
			t.Fatalf("%s has no return contract", name)
		}
		own, wrong := blt012Claims[name][unit], blt012Claims[name][other]
		for _, doc := range []struct {
			where, text string
			own, wrong  []string
		}{
			{"summary", summary, own.summary, wrong.summary},
			{"return doc", returns.Doc, own.returns, wrong.returns},
		} {
			if blt012FirstOf(doc.text, doc.own) == "" {
				t.Errorf("%s on %q answers %d, which counts %s, and its %s does not say so (it has none of %q): %q",
					name, blt012Text, got, unit, doc.where, doc.own, doc.text)
			}
			if phrase := blt012FirstOf(doc.text, doc.wrong); phrase != "" {
				t.Errorf("%s on %q answers %d, which counts %s, and its %s says it counts %s (%q): %q",
					name, blt012Text, got, unit, doc.where, other, phrase, doc.text)
			}
		}
	}
}

// TestStringUnitsBLT012DocsSayWhereTheUnitsPart checks what the summaries say
// about where bytes and runes part -- at a character that takes more than one
// byte, and not at a byte that is not valid UTF-8 -- against the composition
// the row is about: str_substr at a text_index position, and str_char_at at
// len(s) - 1. Each builtin that counts bytes must agree with the rune builtins
// exactly when the text it measures holds no character that takes more than
// one byte, and its summary must say both halves of that. A builtin that
// counts runes agrees with s[i] everywhere, so it is skipped.
func TestStringUnitsBLT012DocsSayWhereTheUnitsPart(t *testing.T) {
	cases := []struct {
		what, text string
		// wide is whether the text holds a character that takes more than one
		// byte. Each such character comes before the colon, so the answer is
		// the same for the text before the match and for the whole string.
		wide bool
	}{
		{"ASCII", "admin: x", false},
		{"a lead byte with no continuation", "h" + string([]byte{0xc3}) + "llo: x", false},
		{"two bytes that begin no sequence", string([]byte{0xff, 0xfe}) + ": x", false},
		{"a two-byte character", blt012Text, true},
		{"a four-byte character", "a" + string(rune(0x1D11E)) + "b: x", true},
	}
	for _, name := range []string{BuiltinNameTextIndex, BuiltinNameLen} {
		if unit, _ := blt012UnitOf(t, name); unit != "bytes" {
			continue
		}
		for _, c := range cases {
			var got object.Object
			var agrees bool
			switch name {
			case BuiltinNameTextIndex:
				// The character at text_index's answer is the colon only
				// while the byte offset is also the rune offset.
				got = StrSubstr(stringObj(c.text), TextIndex(stringObj(c.text), stringObj(":")), intObj(1))
				s, ok := got.(*object.String)
				agrees = ok && s.Value == ":"
			case BuiltinNameLen:
				// len(s) - 1 names a character only while len(s) is also the
				// rune count.
				n, ok := Len(stringObj(c.text)).(*object.Integer)
				if !ok {
					t.Fatalf("len on %q is not an INTEGER", c.text)
				}
				got = StrCharAt(stringObj(c.text), intObj(n.Value-1))
				_, failed := got.(*object.Error)
				agrees = !failed
			}
			switch {
			case c.wide && agrees:
				t.Errorf("%s on %s (%q): the rune builtins agree with it (%q), though a character that takes more than one byte comes first",
					name, c.what, c.text, got.Inspect())
			case !c.wide && !agrees:
				t.Errorf("%s on %s (%q): the rune builtins disagree with it (%q), though no character that takes more than one byte comes first",
					name, c.what, c.text, got.Inspect())
			}
		}
		_, summary, _, _ := TeachingDoc(name)
		for _, phrase := range []string{"more than one byte", "not valid utf-8"} {
			if !strings.Contains(strings.ToLower(summary), phrase) {
				t.Errorf("%s counts bytes, and its summary does not say where that parts from the rune count (it lacks %q): %q",
					name, phrase, summary)
			}
		}
	}
}

// blt012UnitOf runs name on blt012Text and reports whether its answer counts
// bytes or runes. Any other answer fails the test.
func blt012UnitOf(t *testing.T, name string) (unit string, answer int64) {
	t.Helper()
	colonByte := strings.Index(blt012Text, ":")
	colonRune := utf8.RuneCountInString(blt012Text[:colonByte])
	if colonByte == colonRune {
		t.Fatalf("%q has no multi-byte character before the colon, so it cannot tell bytes from runes", blt012Text)
	}
	var got object.Object
	var nBytes, nRunes int
	switch name {
	case BuiltinNameTextIndex:
		got, nBytes, nRunes = TextIndex(stringObj(blt012Text), stringObj(":")), colonByte, colonRune
	case BuiltinNameLen:
		got, nBytes, nRunes = Len(stringObj(blt012Text)), len(blt012Text), utf8.RuneCountInString(blt012Text)
	default:
		t.Fatalf("blt012UnitOf does not know %s", name)
	}
	n, ok := got.(*object.Integer)
	if !ok {
		t.Fatalf("%s on %q = %s, want an INTEGER", name, blt012Text, got.Inspect())
	}
	switch n.Value {
	case int64(nBytes):
		return "bytes", n.Value
	case int64(nRunes):
		return "runes", n.Value
	}
	t.Fatalf("%s on %q = %d, which is neither the byte count %d nor the rune count %d",
		name, blt012Text, n.Value, nBytes, nRunes)
	return "", 0
}

// blt012FirstOf returns the first of phrases that text contains, ignoring
// case, or "" when it contains none of them.
func blt012FirstOf(text string, phrases []string) string {
	lower := strings.ToLower(text)
	for _, phrase := range phrases {
		if strings.Contains(lower, strings.ToLower(phrase)) {
			return phrase
		}
	}
	return ""
}
