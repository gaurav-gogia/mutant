package builtin

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

// releasedRegistries are the builtin registries the release tags define, as
// the OpGetBuiltin ordinals of their artifacts index them: the Builtins literal
// of `git show <tag>:builtin/builtin.go` in order, each name read through that
// tag's builtin/names.go, then whatever its init() appended -- help, from
// v2.3.0. Each is pinned by its length and by the SHA-256 of its names, each
// followed by a NUL, which is the digest legacyOrdinalsDigest uses.
//
// They are the oracle the frozen tables are held to, and they were read from
// the tags rather than from the tables, which is the point. The v2.4.0 entry is
// legacyOrdinalsDigest itself. v2.3.0's is the first 399 entries of the v2.3.x
// table, which differs from v2.4.0's only in slot 398 (help, where v2.4.0 has
// pmap); v2.2.0's is the first 211 entries of either. That entry pins a
// registry fact only: v2.2.0's artifacts were compiled for a one-byte
// OpGetBuiltin operand, which this runtime misreads before either table is
// consulted (compiler/bytecode_version.go says how). Releases before v2.2.0
// numbered a different registry and are not listed: neither table describes
// them.
//
// A table may run past its release's registry. No artifact of that release
// holds an ordinal there, so the entries there cannot misbind one of its calls;
// the v2.3.x table keeps v2.4.x's entries there for the reason legacy_ordinals.go
// gives, which TestTheV23TableKeepsV24sEntriesPast398 pins.
var releasedRegistries = []struct {
	tag    string
	count  int
	digest string
	table  func() []string
}{
	{"v2.2.0", 211, "884513437358f7dbdc80401be5a370948e100c1094fd89435d71e65f0dbcd80d", legacyOrdinalNamesV23},
	{"v2.3.0", 399, "160803d1dade3e10c2d5235cf4835f146da219dc4b5dd92d8171572b311f3f00", legacyOrdinalNamesV23},
	{"v2.4.0", 409, "f489245dc33c4ff172a02a93186e04b96adff36e58a1389566808959a5dedc3b", func() []string { return legacyBuiltinOrdinals }},
}

func registryDigest(names []string) string {
	h := sha256.New()
	for _, name := range names {
		h.Write([]byte(name))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// TestLegacyOrdinalTablesMatchTheReleasedRegistries is M26-BLT-014: one frozen
// table served every versionless artifact, and it was v2.4.0's, so a v2.3.0
// artifact's help call -- ordinal 398 -- ran pmap. Each release's registry must
// be reproduced exactly by the table that release's artifacts are resolved
// through.
func TestLegacyOrdinalTablesMatchTheReleasedRegistries(t *testing.T) {
	for _, release := range releasedRegistries {
		table := release.table()
		if len(table) < release.count {
			t.Errorf("%s: its artifacts resolve through a table of %d entries, but its registry had %d",
				release.tag, len(table), release.count)
			continue
		}
		if got := registryDigest(table[:release.count]); got != release.digest {
			t.Errorf("%s: the table its artifacts resolve through hashes to %s over its %d entries, want %s; "+
				"an ordinal one of its artifacts carries names a different builtin than %s compiled it against",
				release.tag, got, release.count, release.digest, release.tag)
		}
	}
}

// TestResolveLegacyOrdinalsV23DiffersOnlyAtHelp resolves both tables and
// compares the functions rather than the names: they must be the same length
// and agree everywhere except slot 398, where v2.3.x called help and v2.4.x
// calls pmap.
func TestResolveLegacyOrdinalsV23DiffersOnlyAtHelp(t *testing.T) {
	v24, err := ResolveLegacyOrdinals()
	if err != nil {
		t.Fatalf("ResolveLegacyOrdinals: %v", err)
	}
	v23, err := ResolveLegacyOrdinalsV23()
	if err != nil {
		t.Fatalf("ResolveLegacyOrdinalsV23: %v", err)
	}
	if len(v23) != len(v24) {
		t.Fatalf("resolved %d entries for v2.3.x and %d for v2.4.x; the tables differ only at 398", len(v23), len(v24))
	}
	for ordinal, fn := range v23 {
		if fn == nil {
			t.Fatalf("v2.3.x ordinal %d resolved to nil", ordinal)
		}
		if ordinal == 398 {
			if fn != GetBuiltinByName(BuiltinNameHelp) {
				t.Error("v2.3.x ordinal 398 does not resolve to help")
			}
			if v24[ordinal] != GetBuiltinByName(BuiltinNamePMap) {
				t.Error("v2.4.x ordinal 398 does not resolve to pmap")
			}
			continue
		}
		if fn != v24[ordinal] {
			t.Errorf("ordinal %d resolves differently for v2.3.x and v2.4.x; only 398 may", ordinal)
		}
	}
}

// TestTheV23TableKeepsV24sEntriesPast398 pins the v2.3.x table's tail. No
// v2.3.x artifact reaches past 398, but artifacts from 821bece and 31182f3,
// development builds that had v2.4.0's registry and no OpcodeMap, read as
// v2.3.x. Cutting the table at 399 refused their calls to peach, spawn, the
// task_ and chan_ builtins and help, which every runtime before the cut ran
// (M26-BLT-014, repair round 1).
func TestTheV23TableKeepsV24sEntriesPast398(t *testing.T) {
	v23 := legacyOrdinalNamesV23()
	if len(v23) != len(legacyBuiltinOrdinals) {
		t.Fatalf("the v2.3.x table has %d entries; it must keep all %d of v2.4.x's", len(v23), len(legacyBuiltinOrdinals))
	}
	for ordinal := 399; ordinal < len(legacyBuiltinOrdinals); ordinal++ {
		if v23[ordinal] != legacyBuiltinOrdinals[ordinal] {
			t.Errorf("v2.3.x ordinal %d is %q; v2.4.x's is %q", ordinal, v23[ordinal], legacyBuiltinOrdinals[ordinal])
		}
	}
	if v23[403] != BuiltinNameChanNew || v23[408] != BuiltinNameHelp {
		t.Errorf("v2.3.x ordinals 403 and 408 are %q and %q, want chan_new and help", v23[403], v23[408])
	}
}
