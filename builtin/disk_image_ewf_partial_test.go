package builtin

import (
	"encoding/binary"
	"hash/adler32"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aoiflux/libewf/types"

	"mutant/object"
)

// An EWF v1 segment writer, after libewf v0.2.1's own (reader/open_test.go):
// a file header, a volume section in the first segment only, one sectors
// section, a table and its table2 copy, and a terminating "next" or "done".
const (
	ewfSetFileHeader   = 13
	ewfSetDescriptor   = 76
	ewfSetVolume       = 1052
	ewfSetTableHeader  = 24
	ewfSetTableEntry   = 4
	ewfSetTableTrailer = 4
)

type ewfSetSegment struct {
	number         uint16
	terminal       string
	chunks         [][]byte
	withVolume     bool
	declaredChunks uint32
	sectors        uint64
}

func ewfSetStored(payload []byte) []byte {
	var sum [4]byte
	binary.LittleEndian.PutUint32(sum[:], adler32.Checksum(payload))
	return append(append([]byte{}, payload...), sum[:]...)
}

func ewfSetTableSize(entries int) uint64 {
	return uint64(ewfSetDescriptor + ewfSetTableHeader + entries*ewfSetTableEntry + ewfSetTableTrailer)
}

func ewfSetDescribe(buf []byte, offset int, kind string, size uint64) {
	copy(buf[offset:offset+16], kind)
	binary.LittleEndian.PutUint64(buf[offset+16:offset+24], uint64(offset)+size)
	binary.LittleEndian.PutUint64(buf[offset+24:offset+32], size)
	binary.LittleEndian.PutUint32(buf[offset+72:offset+76], adler32.Checksum(buf[offset:offset+72]))
}

func ewfSetTable(buf []byte, tableOffset, base int, stored [][]byte) {
	td := tableOffset + ewfSetDescriptor
	binary.LittleEndian.PutUint32(buf[td:td+4], uint32(len(stored)))
	binary.LittleEndian.PutUint64(buf[td+8:td+16], uint64(base))
	binary.LittleEndian.PutUint32(buf[td+20:td+24], adler32.Checksum(buf[td:td+20]))
	start := td + ewfSetTableHeader
	rel := uint32(0)
	for i, c := range stored {
		binary.LittleEndian.PutUint32(buf[start+i*4:start+i*4+4], rel)
		rel += uint32(len(c))
	}
	end := start + len(stored)*ewfSetTableEntry
	binary.LittleEndian.PutUint32(buf[end:end+4], adler32.Checksum(buf[start:end]))
}

func (s ewfSetSegment) bytes() []byte {
	stored := make([][]byte, len(s.chunks))
	total := 0
	for i, c := range s.chunks {
		stored[i] = ewfSetStored(c)
		total += len(stored[i])
	}
	offset := ewfSetFileHeader
	volume := offset
	if s.withVolume {
		offset += ewfSetDescriptor + ewfSetVolume
	}
	sectors := offset
	data := sectors + ewfSetDescriptor
	table := data + total
	table2 := table + int(ewfSetTableSize(len(stored)))
	terminal := table2 + int(ewfSetTableSize(len(stored)))
	buf := make([]byte, terminal+ewfSetDescriptor)

	copy(buf[0:8], types.SignatureEVFv1[:])
	buf[8] = 0x01
	binary.LittleEndian.PutUint16(buf[9:11], s.number)
	if s.withVolume {
		ewfSetDescribe(buf, volume, "volume", uint64(ewfSetDescriptor+ewfSetVolume))
		v := volume + ewfSetDescriptor
		buf[v] = types.MediaTypeFixed
		binary.LittleEndian.PutUint32(buf[v+4:v+8], s.declaredChunks)
		binary.LittleEndian.PutUint32(buf[v+8:v+12], 1) // sectors per chunk
		binary.LittleEndian.PutUint32(buf[v+12:v+16], 8)
		binary.LittleEndian.PutUint64(buf[v+16:v+24], s.sectors)
	}
	ewfSetDescribe(buf, sectors, "sectors", uint64(ewfSetDescriptor+total))
	at := data
	for _, c := range stored {
		copy(buf[at:], c)
		at += len(c)
	}
	ewfSetDescribe(buf, table, "table", ewfSetTableSize(len(stored)))
	ewfSetTable(buf, table, data, stored)
	ewfSetDescribe(buf, table2, "table2", ewfSetTableSize(len(stored)))
	ewfSetTable(buf, table2, data, stored)
	ewfSetDescribe(buf, terminal, s.terminal, 0)
	return buf
}

// writeEWFSet writes the named members of a three-segment set -- one 8-byte
// chunk each, AAAAAAAA, BBBBBBBB, CCCCCCCC -- into a fresh directory.
func writeEWFSet(t *testing.T, keep ...int) string {
	t.Helper()
	dir := t.TempDir()
	payloads := [][]byte{[]byte("AAAAAAAA"), []byte("BBBBBBBB"), []byte("CCCCCCCC")}
	for _, n := range keep {
		segment := ewfSetSegment{number: uint16(n), terminal: "next", chunks: [][]byte{payloads[n-1]},
			withVolume: n == 1, declaredChunks: 3, sectors: 3}
		if n == 3 {
			segment.terminal = "done"
		}
		if err := os.WriteFile(filepath.Join(dir, "img.E0"+string(rune('0'+n))), segment.bytes(), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

type ewfOpened struct {
	partial  bool
	segments []string
	device   []byte
}

func openEWFSet(t *testing.T, open func(...object.Object) object.Object, arg object.Object) (ewfOpened, *object.Error) {
	t.Helper()
	payload, errObj := unwrapPair(t, open(arg))
	if errObj != nil {
		return ewfOpened{}, errObj
	}
	result := payload.(*object.Hash)
	handle := mustHashValue(t, result, "handle")
	defer EWFClose(handle)
	out := ewfOpened{partial: mustHashBoolValue(t, result, "partial")}
	for _, s := range mustHashArrayValue(t, result, "segments") {
		out.segments = append(out.segments, filepath.Base(s.(*object.String).Value))
	}
	if data, errObj := unwrapPairNoFatal(EWFReadAtBytes(handle, intObj(0), intObj(24))); errObj == nil {
		out.device = data.(*object.Bytes).Value
	}
	return out, nil
}

func ewfList(dir string, names ...string) object.Object {
	elements := make([]object.Object, 0, len(names))
	for _, name := range names {
		elements = append(elements, stringObj(filepath.Join(dir, name)))
	}
	return &object.Array{Elements: elements}
}

// A whole set opened either way is not partial and decodes every chunk. The
// guard the fixes below must not trip.
func TestAWholeEWFSetIsNotPartial(t *testing.T) {
	dir := writeEWFSet(t, 1, 2, 3)
	got, errObj := openEWFSet(t, EWFOpen, stringObj(filepath.Join(dir, "img.E02")))
	if errObj != nil {
		t.Fatalf("ewf_open of a whole set: %s", errObj.Inspect())
	}
	if got.partial || len(got.segments) != 3 || string(got.device) != "AAAAAAAABBBBBBBBCCCCCCCC" {
		t.Fatalf("a whole set opened as %+v", got)
	}
}

// TestAnEWFSetTheReaderFindsIncompleteIsPartial is M26-FS1-009's regression
// test. partial came from discovery alone, which sees holes in the numbering
// and nothing else: a set cut off after its last present segment opened under
// ewf_open_partial with partial false -- the field the docs say carries the
// caveat into any report.
func TestAnEWFSetTheReaderFindsIncompleteIsPartial(t *testing.T) {
	cutShort := writeEWFSet(t, 1, 2)
	got, errObj := openEWFSet(t, EWFOpenPartial, stringObj(filepath.Join(cutShort, "img.E01")))
	if errObj != nil {
		t.Fatalf("ewf_open_partial of a set cut short: %s", errObj.Inspect())
	}
	if !got.partial {
		t.Errorf("a set whose last segment ends in next, not done, opened with partial false: %+v", got)
	}
	// Up to the cut every chunk is where it was acquired.
	if string(got.device) != "AAAAAAAABBBBBBBB" {
		t.Errorf("the set cut short decodes to %q", got.device)
	}
}

// A list is opened as given, and libewf places chunks by the order of the
// segments it is handed: [E01, E03] decoded segment 3's chunk at segment 2's
// offset, with partial false. A list with a hole is refused.
func TestAnEWFListWithAHoleIsRefused(t *testing.T) {
	holed := writeEWFSet(t, 1, 3)
	if got, errObj := openEWFSet(t, EWFOpenPartial, ewfList(holed, "img.E01", "img.E03")); errObj == nil {
		t.Errorf("ewf_open_partial([E01, E03]) opened, decoding %q at offset 0: %+v", got.device, got)
	} else if !strings.Contains(errObj.Message, "segment 2 is not among them") {
		t.Errorf("the refusal does not name the missing segment: %s", errObj.Message)
	}
}

// TestAHoledEWFSetDecodesUpToItsFirstGap is M26-FS1-010's regression test.
// Named by E01, a set holding segments 1 and 3 decoded E01 and never said E03
// was there; named by E03 it decoded E03 alone, with no volume section to read.
// Decoding E03 after the hole would put its chunks at segment 2's offsets, so
// the set decodes from segment 1 to the gap, the same whichever member names
// it, and says which present file it left out.
func TestAHoledEWFSetDecodesUpToItsFirstGap(t *testing.T) {
	dir := writeEWFSet(t, 1, 3)
	for _, name := range []string{"img.E01", "img.E03"} {
		payload, errObj := unwrapPair(t, EWFOpenPartial(stringObj(filepath.Join(dir, name))))
		if errObj != nil {
			t.Errorf("ewf_open_partial named by %s: %s", name, errObj.Inspect())
			continue
		}
		result := payload.(*object.Hash)
		handle := mustHashValue(t, result, "handle")
		segments := mustHashArrayValue(t, result, "segments")
		if len(segments) != 1 || filepath.Base(segments[0].(*object.String).Value) != "img.E01" {
			t.Errorf("named by %s, the set decoded from %s, want img.E01 alone", name, result.Inspect())
		}
		undecoded := mustHashArrayValue(t, result, "undecoded_segments")
		if len(undecoded) != 1 || filepath.Base(undecoded[0].(*object.String).Value) != "img.E03" {
			t.Errorf("named by %s, undecoded_segments does not name img.E03: %s", name, result.Inspect())
		}
		if !mustHashBoolValue(t, result, "partial") {
			t.Errorf("named by %s, partial is false", name)
		}
		data, errObj := unwrapPair(t, EWFReadAtBytes(handle, intObj(0), intObj(24)))
		if errObj != nil || string(data.(*object.Bytes).Value) != "AAAAAAAA" {
			t.Errorf("named by %s, the device reads %v, %v; want segment 1's chunk alone", name, data, errObj)
		}
		EWFClose(handle)
	}
}

// A set with no segment 1 holds the volume section nowhere and nothing in it
// can be placed on the device.
func TestAnEWFSetWithoutSegmentOneIsRefused(t *testing.T) {
	dir := writeEWFSet(t, 2, 3)
	if got, errObj := openEWFSet(t, EWFOpenPartial, stringObj(filepath.Join(dir, "img.E02"))); errObj == nil {
		t.Errorf("ewf_open_partial of segments 2 and 3 opened: %+v", got)
	} else if !strings.Contains(errObj.Message, "no segment 1") {
		t.Errorf("the refusal does not say why: %s", errObj.Message)
	}
}

// A list that is a run from segment 1 opens as given, aligned.
func TestAnEWFListThatIsARunOpensAligned(t *testing.T) {
	dir := writeEWFSet(t, 1, 2, 3)
	got, errObj := openEWFSet(t, EWFOpenPartial, ewfList(dir, "img.E02", "img.E01"))
	if errObj != nil {
		t.Fatalf("ewf_open_partial([E02, E01]): %s", errObj.Inspect())
	}
	if !got.partial || string(got.device) != "AAAAAAAABBBBBBBB" {
		t.Errorf("[E02, E01] of three opened as %+v", got)
	}
}

// TestAListOfOneEWFSegmentIsOpenedAsGiven is M26-FS1-018's regression test.
// The docs say a list of paths is taken as given and nothing is discovered;
// the code discovered whenever exactly one path came in, list or not.
func TestAListOfOneEWFSegmentIsOpenedAsGiven(t *testing.T) {
	dir := writeEWFSet(t, 1, 2, 3)
	got, errObj := openEWFSet(t, EWFOpenPartial, ewfList(dir, "img.E01"))
	if errObj != nil {
		t.Fatalf("ewf_open_partial([img.E01]): %s", errObj.Inspect())
	}
	if len(got.segments) != 1 || got.segments[0] != "img.E01" {
		t.Errorf("[img.E01] decoded from %v, want the one file given", got.segments)
	}
	if !got.partial {
		t.Errorf("one segment of three, opened as given, reported partial false")
	}
}

// --- the 2026-10-10 EWF rows ------------------------------------------------

// ewfPartialHandle opens one named member of a set and hands back the handle
// and its metadata, so the three tests below can each start from what the
// image declares rather than from a number typed here.
func ewfPartialHandle(t *testing.T, dir string, name string) (object.Object, *object.Hash) {
	t.Helper()

	payload, errObj := unwrapPair(t, EWFOpenPartial(stringObj(filepath.Join(dir, name))))
	if errObj != nil {
		t.Fatalf("ewf_open_partial(%s): %s", name, errObj.Inspect())
	}
	handle := mustHashValue(t, payload.(*object.Hash), "handle")
	t.Cleanup(func() { EWFClose(handle) })

	meta, errObj := unwrapPair(t, EWFMetadata(handle))
	if errObj != nil {
		t.Fatalf("ewf_metadata: %s", errObj.Inspect())
	}
	return handle, meta.(*object.Hash)
}

// TestAnEWFReadInsideTheDeclaredMediaThatIsAbsentIsRefused is M26-FS1-024's
// regression test. A set holding segments 1 and 3 declares three chunks and
// can place only the first, so every offset from 8 to 23 is inside the media
// the image declares and inside the segment the set does not have. Those reads
// came back as an empty buffer -- which is also how a read past the end of the
// media comes back -- so a caller walking windows until one came back empty
// read a 24-byte image as an 8-byte one and had nothing to tell it otherwise.
//
// The three cases are now distinct, and this test pins all three, because
// fixing one by breaking another is the way this goes wrong: a window that
// starts inside the data and runs past it still returns what is there (that is
// M26-FS1-010's case, above), and an offset at or past the declared media
// still returns empty, which is how a walk ends.
func TestAnEWFReadInsideTheDeclaredMediaThatIsAbsentIsRefused(t *testing.T) {
	dir := writeEWFSet(t, 1, 3)
	handle, meta := ewfPartialHandle(t, dir, "img.E01")

	declared := mustHashIntValue(t, meta, "total_logical_bytes")
	chunks := mustHashIntValue(t, meta, "number_of_chunks")
	observed := mustHashIntValue(t, meta, "observed_chunk_count")
	if chunks <= observed || observed <= 0 {
		t.Fatalf("the holed set observed %d of %d chunks, which is not the shortfall this test needs",
			observed, chunks)
	}
	// The edge is derived from what the image publishes, not typed in, so the
	// test still means something if the fixture's geometry changes.
	perChunk := declared / chunks
	edge := observed * perChunk
	if edge <= 0 || edge >= declared {
		t.Fatalf("the edge came out at %d of %d declared bytes, which is not a boundary", edge, declared)
	}

	// Inside what the set located: the bytes are there, and nothing changes.
	inside, errObj := unwrapPairNoFatal(EWFReadAtBytes(handle, intObj(0), intObj(perChunk)))
	if errObj != nil {
		t.Fatalf("ewf_read_at_bytes(0, %d) on data the set holds: %s", perChunk, errObj.Message)
	}
	if got := inside.(*object.Bytes).Value; int64(len(got)) != perChunk {
		t.Errorf("ewf_read_at_bytes(0, %d) returned %d bytes from the chunk the set holds", perChunk, len(got))
	}

	// Starting inside and running past the edge: short, not padded, no error.
	straddle, errObj := unwrapPairNoFatal(EWFReadAtBytes(handle, intObj(0), intObj(declared)))
	if errObj != nil {
		t.Fatalf("ewf_read_at_bytes(0, %d) straddling the edge was refused (%s); a window that "+
			"starts inside the data must still return what is there", declared, errObj.Message)
	}
	if got := int64(len(straddle.(*object.Bytes).Value)); got != edge {
		t.Errorf("ewf_read_at_bytes(0, %d) returned %d bytes, want the %d it located and no padding",
			declared, got, edge)
	}

	// At the edge and inside the declared media: refused, naming both numbers.
	for _, offset := range []int64{edge, declared - 1} {
		got, errObj := unwrapPairNoFatal(EWFReadAtBytes(handle, intObj(offset), intObj(perChunk)))
		if errObj == nil {
			t.Errorf("ewf_read_at_bytes(%d, %d) answered with %d byte(s) for an offset inside the %d "+
				"bytes this image declares and inside the segment the set is missing; an empty "+
				"answer there is the same answer as the end of the media",
				offset, perChunk, len(got.(*object.Bytes).Value), declared)
			continue
		}
		for _, number := range []string{itoa(offset), itoa(declared)} {
			if !strings.Contains(errObj.Message, number) {
				t.Errorf("the refusal at offset %d does not name %s, so a caller cannot tell how far "+
					"short of the declared media it stopped: %s", offset, number, errObj.Message)
			}
		}
	}

	// At and past the declared media: empty, and no error. This is how a
	// caller walks off the end, and the fix above must not take it away.
	for _, offset := range []int64{declared, declared + perChunk} {
		got, errObj := unwrapPairNoFatal(EWFReadAtBytes(handle, intObj(offset), intObj(perChunk)))
		if errObj != nil {
			t.Errorf("ewf_read_at_bytes(%d, %d) past the %d bytes declared was refused (%s), and "+
				"walking off the end is how a read loop is supposed to finish",
				offset, perChunk, declared, errObj.Message)
			continue
		}
		if n := len(got.(*object.Bytes).Value); n != 0 {
			t.Errorf("ewf_read_at_bytes(%d, %d) past the end returned %d bytes", offset, perChunk, n)
		}
	}
}

// TestAnEWFv1ImageDoesNotPublishACompressionMethodItNeverRead is
// M26-FS1-026's regression test. compression_method is libewf's enum -- 0
// none, 1 deflate, 2 bzip2 -- but EWF v1's file header has no such field, so
// libewf fills the struct with 0 for every v1 image. mutant published that 0
// on its own, which says "stored uncompressed" about an EnCase 6 image whose
// chunks are deflate: ewfinfo prints "deflate" for the same file, and an image
// acquired here with deflate at best compression, 100 MiB down to 62 MB, still
// read as 0. has_compression_method is what tells the two apart.
//
// Only the v1 side is measured, here and anywhere: the libewf build available
// refuses to write EWF2 ("Unsupported EWF format defaulting to: encase6"), so
// nothing proves the v2 path publishes the header's own value. It reads it --
// libewf parses it at reader/open.go's parseFileHeaderV2 -- and that is all
// that can honestly be said.
func TestAnEWFv1ImageDoesNotPublishACompressionMethodItNeverRead(t *testing.T) {
	dir := writeEWFSet(t, 1, 2, 3)
	_, meta := ewfPartialHandle(t, dir, "img.E01")

	if major := mustHashIntValue(t, meta, "major_version"); major != 1 {
		t.Fatalf("the fixture wrote a major version %d image; this test is about v1", major)
	}
	if mustHashBoolValue(t, meta, "has_compression_method") {
		t.Errorf("a v1 image reports has_compression_method true, but v1's file header has no such "+
			"field and the %d published for it is libewf's placeholder",
			mustHashIntValue(t, meta, "compression_method"))
	}
}

// TestEWFVerifyNeverCallsAnAbsentDigestAMismatch is M26-FS1-025's regression
// test. md5_match and sha1_match read false both when a stored digest failed
// to reproduce and when there was no stored digest to compare against, so a
// comparison that never happened looked like one that failed. The pair is the
// contract -- has_stored_md5 says whether anything was compared, md5_match
// says how it came out -- and this pins it in both directions on an image that
// stores no digest at all: the match fields must not claim a match, and ok
// must not be true, because there is nothing here to verify against.
func TestEWFVerifyNeverCallsAnAbsentDigestAMismatch(t *testing.T) {
	dir := writeEWFSet(t, 1, 2, 3)
	handle, _ := ewfPartialHandle(t, dir, "img.E01")

	payload, errObj := unwrapPair(t, EWFVerify(handle))
	if errObj != nil {
		t.Fatalf("ewf_verify: %s", errObj.Inspect())
	}
	result := payload.(*object.Hash)

	compared := 0
	for _, pair := range []struct{ has, match, stored, computed string }{
		{"has_stored_md5", "md5_match", "stored_md5", "computed_md5"},
		{"has_stored_sha1", "sha1_match", "stored_sha1", "computed_sha1"},
	} {
		has := mustHashBoolValue(t, result, pair.has)
		match := mustHashBoolValue(t, result, pair.match)
		if has {
			// Nothing to prove here on this image, but if a fixture ever does
			// store a digest the pair still has to agree with itself.
			if want := mustHashStringValue(t, result, pair.stored) == mustHashStringValue(t, result, pair.computed); match != want {
				t.Errorf("%s is %v while %s and %s %s", pair.match, match, pair.stored, pair.computed,
					map[bool]string{true: "are equal", false: "differ"}[want])
			}
			continue
		}
		compared++
		if match {
			t.Errorf("%s is true on an image where %s is false, so a comparison that never happened "+
				"is reported as one that succeeded", pair.match, pair.has)
		}
		if stored := mustHashStringValue(t, result, pair.stored); stored != "" {
			t.Errorf("%s is false but %s carries %q", pair.has, pair.stored, stored)
		}
	}
	if compared == 0 {
		t.Fatal("this image stores both digests, so the absent-digest case went untested")
	}
	if mustHashBoolValue(t, result, "ok") {
		t.Error("ok is true for an image that stores no digest, so there was nothing to verify against")
	}
}
