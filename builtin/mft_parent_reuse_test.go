package builtin

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	libntfs "github.com/aoiflux/libntfs"

	"mutant/object"
)

// M26-ART-023's regression tests. An NTFS file reference is a record number and
// that record's sequence number, and NTFS reuses records: freeing one adds one
// to its sequence number. mft_parse kept the record number of each $FILE_NAME
// parent reference and dropped the sequence number, so a deleted file whose
// directory had since been deleted, and its record given to a new directory,
// was reported inside the new one -- \NewDir\evil.exe for a file that was never
// in NewDir.

// parentReuseRecord is one record of the $MFT these tests build: its number,
// sequence number and in-use flag, and the parent reference its $FILE_NAME
// carries.
type parentReuseRecord struct {
	num       uint32
	seq       uint16
	inUse     bool
	dir       bool
	name      string
	parent    uint64
	parentSeq uint16
}

// writeParentReuseMFT writes recs as a standalone $MFT of 1024-byte records,
// each at its own number. Record 0 must be among them: a standalone $MFT is
// recognised by the FILE signature it starts with.
func writeParentReuseMFT(t *testing.T, recs []parentReuseRecord) string {
	t.Helper()
	const recordSize = 1024
	last := uint32(0)
	for _, r := range recs {
		last = max(last, r.num)
	}
	mft := make([]byte, int(last+1)*recordSize)
	ft := mftNTFSTime(1500000000)
	for _, r := range recs {
		rec := buildMFTRecord(r.num, r.seq, r.dir, libntfs.MakeFileReference(r.parent, r.parentSeq), r.name, 10, ft)
		if !r.inUse {
			flags := binary.LittleEndian.Uint16(rec[0x16:])
			binary.LittleEndian.PutUint16(rec[0x16:], flags&^libntfs.MFTFlagInUse)
		}
		copy(mft[int(r.num)*recordSize:], rec)
	}
	path := filepath.Join(t.TempDir(), "$MFT")
	if err := os.WriteFile(path, mft, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// parentReuseVolume is the $MFT behind most of the cases below. Record 6 held
// a directory at sequence 2; it was deleted (freeing it made the number 3) and
// the record now holds NewDir. The other records try the same comparison from
// each side: a parent deleted and left alone, a chain that breaks higher up,
// the counter wrapping, and references with nothing to compare against.
var parentReuseVolume = []parentReuseRecord{
	{num: 0, seq: 1, inUse: true, name: "$MFT", parent: 5, parentSeq: 5},
	{num: 5, seq: 5, inUse: true, dir: true, name: ".", parent: 5, parentSeq: 5},
	// The row: evil.exe names record 6 at sequence 2, and record 6 is NewDir.
	{num: 6, seq: 3, inUse: true, dir: true, name: "NewDir", parent: 5, parentSeq: 5},
	{num: 7, seq: 2, name: "evil.exe", parent: 6, parentSeq: 2},
	// A deleted directory nothing has taken: freeing it moved 3 to 4, and the
	// file inside it still names 3. Deleting a directory with its files in it
	// leaves this shape.
	{num: 8, seq: 4, dir: true, name: "OldDir", parent: 5, parentSeq: 5},
	{num: 9, seq: 2, name: "gone.txt", parent: 8, parentSeq: 3},
	// A deleted directory whose number did not move, and a file naming it.
	{num: 10, seq: 6, dir: true, name: "KeptDir", parent: 5, parentSeq: 5},
	{num: 11, seq: 2, name: "kept.txt", parent: 10, parentSeq: 6},
	// The chain breaks one level up: Sub is itself still where deep.txt says,
	// but Sub's own parent was record 6 at sequence 2.
	{num: 12, seq: 3, dir: true, name: "Sub", parent: 6, parentSeq: 2},
	{num: 13, seq: 2, name: "deep.txt", parent: 12, parentSeq: 2},
	// The counter skips zero: freeing a record at 65535 makes it 1. Taken
	// again after that, as LiveWrap's record was, it holds another file.
	{num: 14, seq: 1, dir: true, name: "WrapDir", parent: 5, parentSeq: 5},
	{num: 15, seq: 2, name: "wrap.txt", parent: 14, parentSeq: 0xFFFF},
	{num: 16, seq: 1, inUse: true, dir: true, name: "LiveWrap", parent: 5, parentSeq: 5},
	{num: 17, seq: 2, name: "stale.txt", parent: 16, parentSeq: 0xFFFF},
	// Sequence number 0 is NTFS's "no sequence number": nothing to compare.
	{num: 18, seq: 2, name: "nosequence.txt", parent: 6, parentSeq: 0},
	// A parent the table does not hold: nothing to compare either.
	{num: 19, seq: 2, name: "lost.txt", parent: 40, parentSeq: 7},
	// The root is checked like any other parent when the table holds it.
	{num: 20, seq: 1, inUse: true, name: "top.txt", parent: 5, parentSeq: 5},
	{num: 21, seq: 2, name: "otherroot.txt", parent: 5, parentSeq: 3},
}

func TestMftParseParentReuseIsNotTheOriginalDirectory(t *testing.T) {
	payload, errObj := unwrapPair(t, MftParse(stringObj(writeParentReuseMFT(t, parentReuseVolume))))
	if errObj != nil {
		t.Fatalf("mft_parse: %s", errObj.Inspect())
	}
	entries := hashValueByKey(payload.(*object.Hash), "entries").(*object.Array)

	cases := []struct {
		name           string
		path           string
		parentRecord   int64
		parentSequence int64
		checked        bool
		reused         bool
	}{
		{"evil.exe", `?\evil.exe`, 6, 2, true, true},
		{"NewDir", `\NewDir`, 5, 5, true, false},
		{"gone.txt", `\OldDir\gone.txt`, 8, 3, true, false},
		{"kept.txt", `\KeptDir\kept.txt`, 10, 6, true, false},
		{"Sub", `?\Sub`, 6, 2, true, true},
		{"deep.txt", `?\Sub\deep.txt`, 12, 2, true, false},
		{"wrap.txt", `\WrapDir\wrap.txt`, 14, 0xFFFF, true, false},
		{"stale.txt", `?\stale.txt`, 16, 0xFFFF, true, true},
		{"nosequence.txt", `\NewDir\nosequence.txt`, 6, 0, false, false},
		{"lost.txt", `?\lost.txt`, 40, 7, false, false},
		{"top.txt", `\top.txt`, 5, 5, true, false},
		{"otherroot.txt", `?\otherroot.txt`, 5, 3, true, true},
	}
	for _, c := range cases {
		e := mftEntryByName(t, entries, c.name)
		if got := hStr(t, e, "path"); got != c.path {
			t.Errorf("%s: path = %q, want %q", c.name, got, c.path)
		}
		if got := hInt(t, e, "parent_record"); got != c.parentRecord {
			t.Errorf("%s: parent_record = %d, want %d", c.name, got, c.parentRecord)
		}
		if got := hInt(t, e, "parent_sequence"); got != c.parentSequence {
			t.Errorf("%s: parent_sequence = %d, want %d", c.name, got, c.parentSequence)
		}
		if got := hBoolAt(t, e, "parent_checked"); got != c.checked {
			t.Errorf("%s: parent_checked = %v, want %v", c.name, got, c.checked)
		}
		if got := hBoolAt(t, e, "parent_reused"); got != c.reused {
			t.Errorf("%s: parent_reused = %v, want %v", c.name, got, c.reused)
		}
	}
}

// fs_deleted reconstructs its paths with the same code, and placed evil.exe in
// NewDir the same way.
func TestFsDeletedParentReuseIsNotTheOriginalDirectory(t *testing.T) {
	payload, errObj := unwrapPair(t, FsDeleted(stringObj(writeParentReuseMFT(t, parentReuseVolume))))
	if errObj != nil {
		t.Fatalf("fs_deleted: %s", errObj.Inspect())
	}
	want := map[string]string{
		"evil.exe": `?\evil.exe`,
		"gone.txt": `\OldDir\gone.txt`,
		"deep.txt": `?\Sub\deep.txt`,
	}
	for _, el := range hashValueByKey(payload.(*object.Hash), "entries").(*object.Array).Elements {
		e := el.(*object.Hash)
		name := hStr(t, e, "name")
		if path, ok := want[name]; ok {
			if got := hStr(t, e, "path"); got != path {
				t.Errorf("fs_deleted %s: path = %q, want %q", name, got, path)
			}
			delete(want, name)
		}
	}
	for name := range want {
		t.Errorf("fs_deleted reported no entry for %s", name)
	}
}

// parentReuseSetSequences sets the sequence number of a record built by
// ntfsVolRecord, and the parent sequence number its $FILE_NAME carries, which
// ntfsVolRecord writes as 1 for both. Neither field sits at a sector's end,
// so the fixups the record already carries are untouched.
func parentReuseSetSequences(t *testing.T, rec []byte, seq, parentSeq uint16) []byte {
	t.Helper()
	libntfs.WriteUint16LE(rec, 16, seq)
	off := int(libntfs.ReadUint16LE(rec, 20))
	for off+24 <= len(rec) {
		attrType := libntfs.ReadUint32LE(rec, off)
		length := int(libntfs.ReadUint32LE(rec, off+4))
		if attrType == 0xFFFFFFFF || length <= 0 {
			break
		}
		if attrType == libntfs.AttrTypeFileName {
			value := off + int(libntfs.ReadUint16LE(rec, off+20))
			libntfs.WriteUint16LE(rec, value+6, parentSeq)
			return rec
		}
		off += length
	}
	t.Fatal("the record has no $FILE_NAME")
	return nil
}

// parentReuseNTFSImage is the same reuse on a volume image: record 7 is NewDir
// at sequence 3, and the deleted gone.txt in record 6 names record 7 at
// sequence 2.
func parentReuseNTFSImage(t *testing.T) []byte {
	t.Helper()
	image := buildNTFSVolume(2)
	place := func(n int, record []byte) {
		copy(image[ntfsVolMFTCluster*ntfsVolClusterSize+n*ntfsVolRecordSize:], record)
	}
	place(6, parentReuseSetSequences(t, ntfsVolRecord(0, "gone.txt", 7, 4096, ntfsVolData(10, 1, 4096)), 2, 2))
	place(7, parentReuseSetSequences(t, ntfsVolRecord(libntfs.MFTFlagInUse|0x0002, "NewDir", 5, 0), 3, 1))
	return image
}

// ntfs_deleted sweeps the MFT itself and reconstructs paths with the same code.
func TestNtfsDeletedParentReuseIsNotTheOriginalDirectory(t *testing.T) {
	handle := openNTFSVolume(t, parentReuseNTFSImage(t))
	payload, errObj := unwrapPair(t, NtfsDeleted(handle))
	if errObj != nil {
		t.Fatalf("ntfs_deleted: %s", errObj.Inspect())
	}
	found := false
	for _, el := range mustHashArrayValue(t, payload.(*object.Hash), "entries") {
		e := el.(*object.Hash)
		if mustHashStringValue(t, e, "name") != "gone.txt" {
			continue
		}
		found = true
		if got, want := mustHashStringValue(t, e, "path"), `?\gone.txt`; got != want {
			t.Errorf("ntfs_deleted: gone.txt path = %q, want %q", got, want)
		}
	}
	if !found {
		t.Errorf("ntfs_deleted reported no entry for gone.txt: %s", payload.Inspect())
	}
}

// mft_parse walks a volume image through libntfs's volume reader rather than
// record by record, and reaches the same path code.
func TestMftParseOverAVolumeParentReuseIsNotTheOriginalDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ntfs.img")
	if err := os.WriteFile(path, parentReuseNTFSImage(t), 0o600); err != nil {
		t.Fatal(err)
	}
	payload, errObj := unwrapPair(t, MftParse(stringObj(path)))
	if errObj != nil {
		t.Fatalf("mft_parse: %s", errObj.Inspect())
	}
	if got := hStr(t, payload.(*object.Hash), "source_type"); got != "volume" {
		t.Fatalf("source_type = %q, want volume", got)
	}
	gone := mftEntryByName(t, hashValueByKey(payload.(*object.Hash), "entries").(*object.Array), "gone.txt")
	if got, want := hStr(t, gone, "path"), `?\gone.txt`; got != want {
		t.Errorf("gone.txt path = %q, want %q", got, want)
	}
	if !hBoolAt(t, gone, "parent_reused") {
		t.Errorf("gone.txt parent_reused = false: %s", gone.Inspect())
	}
}
