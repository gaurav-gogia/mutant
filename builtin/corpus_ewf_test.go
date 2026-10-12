package builtin

// Real-image tests for the ewf_* family, against libewf's own tools.
//
// WHY LIBEWF'S TOOLS. mutant reads EWF through github.com/aoiflux/libewf,
// which is a separate implementation of the Expert Witness format. ewfinfo,
// ewfverify and ewfexport (libewf 20140814) share no code and no reasoning
// with it, so where the two agree the agreement means something. A fixture
// cannot give this: it is built from the same reading of the format as the
// parser, so it cannot catch a misreading of the format.
//
// WHY NO MOUNT. None is needed, which is what makes this comparison possible
// on a machine with no root. ewfexport writes a byte range straight out of the
// set, and ewfverify decodes the whole media and hashes it. Both are read-only
// and neither needs a loop device.
//
// WHAT THE COMPARISON COVERS, AND WHY THE DIGEST IS THE WHOLE OF IT. The
// corpus set is two segment files holding 10,737,418,240 bytes of logical
// media between them. ewfverify computes a SHA-1 over the decoded media, and
// no SHA-1 is stored in these files -- so when ewf_verify's computed_sha1
// equals ewfverify's, both readers independently derived the same digest from
// the same 10 GiB, and a single wrong byte anywhere, including at the join
// between the two segment files, would change it. That one comparison covers
// every byte. The window rows cover something it cannot: ewf_read_at's offset
// arithmetic, which is a different code path from the sequential walk the
// verify does.
//
// WHAT IT FOUND. In the content path, nothing. Every media fact agrees with
// ewfinfo; the stored MD5, the computed MD5 and the computed SHA-1 all agree
// with ewfverify; nine windows agree with ewfexport byte for byte, including
// one straddling a chunk boundary, a megabyte spanning thirty-two chunks, and
// one that can only come from the second segment file. The chunk accounting on
// an incomplete set closes exactly: 141,800 chunks observed plus 185,880
// reported as unreadable is the 327,680 the header declares, and the first
// unreadable offset is exactly 141,800 chunks in.
//
// WHAT IT FOUND THAT IS WRITTEN DOWN AS A ROW RATHER THAN PINNED HERE. Three,
// and none is a wrong number:
//
//   - M26-FS1-024. On a set opened by ewf_open_partial with a segment missing,
//     an offset inside total_logical_bytes that falls in the absent segment
//     returns an empty buffer and no error -- the same answer as an offset past
//     the end of a complete image. ext_read_file_at's contract makes that case
//     an error naming both numbers, and says why: a short buffer would hide it.
//     The tests below therefore pin what is certainly right -- a short read
//     ends exactly where the data does, and the bytes returned are a prefix of
//     the full set's -- and leave the empty-or-error question to the fix.
//   - M26-FS1-025. md5_match and sha1_match are false both when a digest failed
//     to reproduce and when there was no digest to compare against. The
//     comparator is sound: against an image acquired with both digests stored,
//     both come back true. So the tests below require the match fields to agree
//     with the stored digests where there ARE stored digests, and do not
//     require a particular value where there are none.
//   - M26-FS1-026. compression_method is an integer whose values are named
//     nowhere in the tree.
//
// WHAT IS NOT COVERED, SAID PLAINLY. ewf_read_at_bytes is compared against
// ewf_read_at but not against ewfexport separately. A set with a hole in the
// MIDDLE of its numbering is not exercised, because the corpus set has two
// segments and a middle hole needs three; undecoded_segments is therefore
// never non-empty here. The checksum_policy argument is not exercised against
// an image with a bad checksum, because the corpus holds none.
//
// COST. TestEWFVerifyReproducesTheDigestsLibewfComputed decodes and hashes the
// whole 10 GiB and takes roughly two minutes. It runs only when -corpus.dir is
// given, which is never during an ordinary `go test ./...` or a release gate.
//
// THE ORACLE. Written by ewf_oracle.sh, which drives ewfinfo, ewfverify and
// ewfexport. It lives with the corpus rather than in this repository, as
// make_fat_oracle.py does in libfat, and its format is the tagged one used by
// the other oracles here:
//
//	M <bytes_per_sector> <number_of_sectors> <media_size> <sectors_per_chunk> <segments>
//	H <algorithm> <stored|computed> <hex or ->
//	W <offset> <length> <bytes_returned> <sha256>
//
// Nothing is pinned by value in this file. Every number compared comes either
// from the oracle beside the image or from another builtin reading the same
// image, because the corpus images are not byte-reproducible across a
// regeneration and a digest written into a test would outlive the volume it
// describes.

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"mutant/object"
)

// ewfCorpusImage is the first segment of the set. The others are found by the
// builtin's own discovery, which is part of what is being measured.
const ewfCorpusImage = "jean.e01"

// ewfCorpusMedia is what one EWF oracle says about the set.
type ewfCorpusMedia struct {
	BytesPerSector  int64
	NumberOfSectors int64
	MediaSize       int64
	SectorsPerChunk int64
	Segments        int64
}

// ewfCorpusDigest is one H row: an algorithm, whether the value was stored in
// the file or computed over the data, and the value.
//
// Stored and computed are kept apart rather than folded into one field because
// the difference is the whole point of a verify: one is what the acquisition
// tool claimed, the other is what the data says now.
type ewfCorpusDigest struct {
	Algorithm string
	Kind      string
	// Hex is empty when the oracle recorded "-", which is how it says the
	// file stores no such digest. An empty value is never compared as though
	// it were a digest that failed to match.
	Hex string
}

// ewfCorpusWindow is one W row: a byte range ewfexport wrote out, how many
// bytes it produced, and the digest of them.
type ewfCorpusWindow struct {
	Offset   int64
	Length   int64
	Returned int64
	SHA256   string
	Line     int
}

type ewfCorpusOracle struct {
	Media   ewfCorpusMedia
	Digests []ewfCorpusDigest
	Windows []ewfCorpusWindow
}

func (o ewfCorpusOracle) digest(algorithm, kind string) (string, bool) {
	for _, d := range o.Digests {
		if d.Algorithm == algorithm && d.Kind == kind {
			return d.Hex, d.Hex != "" && d.Hex != "-"
		}
	}
	return "", false
}

func readEWFCorpusOracle(t *testing.T, path string) ewfCorpusOracle {
	t.Helper()

	var oracle ewfCorpusOracle
	mediaRows := 0
	for _, row := range corpusOracleLines(t, path) {
		fields := row.fields
		switch fields[0] {
		case "M":
			requireOracleFields(t, path, row.line, fields, 6)
			mediaRows++
			oracle.Media = ewfCorpusMedia{
				BytesPerSector:  oracleInt(t, path, row.line, fields[1]),
				NumberOfSectors: oracleInt(t, path, row.line, fields[2]),
				MediaSize:       oracleInt(t, path, row.line, fields[3]),
				SectorsPerChunk: oracleInt(t, path, row.line, fields[4]),
				Segments:        oracleInt(t, path, row.line, fields[5]),
			}
		case "H":
			requireOracleFields(t, path, row.line, fields, 4)
			value := strings.TrimSpace(fields[3])
			if value == "-" {
				value = ""
			}
			oracle.Digests = append(oracle.Digests, ewfCorpusDigest{
				Algorithm: fields[1], Kind: fields[2], Hex: value,
			})
		case "W":
			requireOracleFields(t, path, row.line, fields, 5)
			oracle.Windows = append(oracle.Windows, ewfCorpusWindow{
				Offset:   oracleInt(t, path, row.line, fields[1]),
				Length:   oracleInt(t, path, row.line, fields[2]),
				Returned: oracleInt(t, path, row.line, fields[3]),
				SHA256:   strings.TrimSpace(fields[4]),
				Line:     row.line,
			})
		default:
			t.Fatalf("%s line %d: unknown record tag %q; the oracle format has M, H and W, "+
				"and a tag this test does not understand must not be skipped", path, row.line, fields[0])
		}
	}
	if mediaRows != 1 {
		t.Fatalf("%s holds %d M records, want exactly 1", path, mediaRows)
	}
	if _, ok := oracle.digest("md5", "computed"); !ok {
		t.Fatalf("%s records no computed MD5, so the whole-media comparison would assert nothing", path)
	}
	sort.Slice(oracle.Windows, func(i, j int) bool { return oracle.Windows[i].Offset < oracle.Windows[j].Offset })
	return oracle
}

// ewfCorpusOracleFor loads the oracle, or SKIPS.
//
// It skips rather than returning a flag the caller might quietly act on,
// because a test that returns early having compared nothing is reported as a
// pass, and a pass that measured nothing is worse than a skip: the skip says
// so in the log and the pass does not.
func ewfCorpusOracleFor(t *testing.T) ewfCorpusOracle {
	t.Helper()

	path, ok := corpusFile(t, ewfCorpusImage+".ewf-oracle.tsv")
	if !ok {
		t.Skipf("no oracle beside %s in this copy of the corpus, so there is nothing independent "+
			"to compare against; ewf_oracle.sh writes it", ewfCorpusImage)
	}
	return readEWFCorpusOracle(t, path)
}

// openCorpusEWF opens the whole set from its first segment, letting the
// builtin's own discovery find the rest.
func openCorpusEWF(t *testing.T) (string, bool) {
	t.Helper()

	image, ok := corpusFile(t, ewfCorpusImage)
	if !ok {
		return "", false
	}

	payload, errObj := unwrapPair(t, EWFOpen(stringObj(filepath.ToSlash(image))))
	if errObj != nil {
		t.Fatalf("ewf_open(%s): %s", image, errObj.Inspect())
	}
	openHash, okHash := payload.(*object.Hash)
	if !okHash {
		t.Fatalf("ewf_open payload is not HASH. got=%T", payload)
	}

	handle := mustHashStringValue(t, openHash, "handle")
	t.Cleanup(func() { EWFClose(stringObj(handle)) })
	return handle, true
}

// ewfCorpusMetadata returns the metadata hash for an open handle.
func ewfCorpusMetadata(t *testing.T, handle string) *object.Hash {
	t.Helper()

	payload, errObj := unwrapPair(t, EWFMetadata(stringObj(handle)))
	if errObj != nil {
		t.Fatalf("ewf_metadata: %s", errObj.Inspect())
	}
	return payload.(*object.Hash)
}

// ewfCorpusReadAt reads one window and returns the bytes, or says it refused.
//
// A refusal is returned rather than failed on, because which offsets refuse is
// one of the things these tests are reporting on rather than asserting.
func ewfCorpusReadAt(t *testing.T, handle string, offset, length int64) ([]byte, string) {
	t.Helper()

	payload, errObj := unwrapPairNoFatal(EWFReadAt(stringObj(handle), intObj(offset), intObj(length)))
	if errObj != nil {
		return nil, errObj.Message
	}
	text, ok := payload.(*object.String)
	if !ok {
		t.Fatalf("ewf_read_at at offset %d returned %s, want STRING", offset, payload.Type())
	}
	return []byte(text.Value), ""
}

// TestEWFMediaFactsAgreeWithLibewf compares every geometry number mutant
// publishes against what ewfinfo read from the same files.
func TestEWFMediaFactsAgreeWithLibewf(t *testing.T) {
	oracle := ewfCorpusOracleFor(t)
	handle, ok := openCorpusEWF(t)
	if !ok {
		return
	}

	meta := ewfCorpusMetadata(t, handle)
	for _, c := range []struct {
		field string
		want  int64
	}{
		{"bytes_per_sector", oracle.Media.BytesPerSector},
		{"number_of_sectors", oracle.Media.NumberOfSectors},
		{"total_logical_bytes", oracle.Media.MediaSize},
		{"sectors_per_chunk", oracle.Media.SectorsPerChunk},
	} {
		if got := mustHashIntValue(t, meta, c.field); got != c.want {
			t.Errorf("ewf_metadata %s is %d, ewfinfo says %d", c.field, got, c.want)
		}
	}

	// number_of_chunks is not something ewfinfo prints, so it is checked
	// against the geometry ewfinfo does print. A chunk count that does not
	// follow from the sector count would make every chunk-table offset wrong.
	wantChunks := oracle.Media.NumberOfSectors / oracle.Media.SectorsPerChunk
	if got := mustHashIntValue(t, meta, "number_of_chunks"); got != wantChunks {
		t.Errorf("ewf_metadata number_of_chunks is %d, but %d sectors of %d per chunk is %d",
			got, oracle.Media.NumberOfSectors, oracle.Media.SectorsPerChunk, wantChunks)
	}

	// The segment list, which is discovery rather than parsing: ewfinfo names
	// the files it opened, and mutant was given only the first of them.
	payload, errObj := unwrapPair(t, EWFSegments(stringObj(filepath.ToSlash(filepath.Join(*corpusDir, ewfCorpusImage)))))
	if errObj != nil {
		t.Fatalf("ewf_segments: %s", errObj.Inspect())
	}
	segments := payload.(*object.Hash)
	if got := mustHashIntValue(t, segments, "segment_count"); got != oracle.Media.Segments {
		t.Errorf("ewf_segments found %d segment(s), ewfinfo opened %d", got, oracle.Media.Segments)
	}
	if got := mustHashIntValue(t, segments, "present_count"); got != oracle.Media.Segments {
		t.Errorf("ewf_segments says %d present, ewfinfo opened %d", got, oracle.Media.Segments)
	}
	if !mustHashBoolValue(t, segments, "contiguous") {
		t.Errorf("ewf_segments says the set is not contiguous, but ewfinfo opened all %d of its files",
			oracle.Media.Segments)
	}
	// Guarded, because a closing line that says the two readers agreed is a
	// claim, and printing it under a failure would be its own small version of
	// the problem this file exists to catch.
	if !t.Failed() {
		t.Logf("agreed with ewfinfo about %d sectors of %d, %d bytes of media across %d segment(s)",
			oracle.Media.NumberOfSectors, oracle.Media.BytesPerSector,
			oracle.Media.MediaSize, oracle.Media.Segments)
	}
}

// TestEWFVerifyReproducesTheDigestsLibewfComputed is the whole-media
// comparison. It decodes and hashes every byte, which is the only check that
// covers the join between one segment file and the next.
func TestEWFVerifyReproducesTheDigestsLibewfComputed(t *testing.T) {
	oracle := ewfCorpusOracleFor(t)
	handle, ok := openCorpusEWF(t)
	if !ok {
		return
	}

	payload, errObj := unwrapPair(t, EWFVerify(stringObj(handle)))
	if errObj != nil {
		t.Fatalf("ewf_verify: %s", errObj.Inspect())
	}
	result := payload.(*object.Hash)

	if got := mustHashIntValue(t, result, "size"); got != oracle.Media.MediaSize {
		t.Errorf("ewf_verify size is %d, ewfinfo says the media is %d bytes", got, oracle.Media.MediaSize)
	}
	// bytes_hashed can exceed what was decodable, because a span that fails to
	// decode is zero-filled so hashing can continue and is listed in
	// bad_ranges. On a complete set there should be no such span at all, so
	// here the two numbers must agree and bad_ranges must be empty.
	badRanges, foundBad := ewfCorpusArray(t, result, "bad_ranges")
	if !foundBad {
		t.Fatalf("ewf_verify published no bad_ranges array")
	}
	if len(badRanges) != 0 {
		t.Errorf("ewf_verify reports %d unreadable span(s) on a complete set that ewfverify read "+
			"end to end without error; the first is at offset %d",
			len(badRanges), mustHashIntValue(t, badRanges[0].(*object.Hash), "offset"))
	}
	if got := mustHashIntValue(t, result, "bytes_hashed"); got != oracle.Media.MediaSize {
		t.Errorf("ewf_verify hashed %d bytes of a %d-byte media", got, oracle.Media.MediaSize)
	}

	// The digests. A computed value is the strong one: no SHA-1 is stored in
	// these files, so agreement there means both readers derived the same
	// digest from the same decoded bytes rather than both copying a header.
	compared := 0
	for _, algorithm := range []string{"md5", "sha1"} {
		wantComputed, haveComputed := oracle.digest(algorithm, "computed")
		if haveComputed {
			got := mustHashStringValue(t, result, "computed_"+algorithm)
			if got != wantComputed {
				t.Errorf("ewf_verify computed_%s is %s, ewfverify computed %s over the same media",
					algorithm, got, wantComputed)
			}
			compared++
		}

		wantStored, haveStored := oracle.digest(algorithm, "stored")
		hasStored := mustHashBoolValue(t, result, "has_stored_"+algorithm)
		if hasStored != haveStored {
			t.Errorf("ewf_verify has_stored_%s is %v, but ewfinfo %s a stored %s",
				algorithm, hasStored, map[bool]string{true: "lists", false: "lists no"}[haveStored],
				strings.ToUpper(algorithm))
		}
		if !haveStored {
			// Nothing stored, so nothing to compare: whether *_match is false
			// or absent here is M26-FS1-024's sibling question and is not
			// pinned. What IS required is that the stored field stay empty,
			// because a digest appearing from nowhere would be a fabrication.
			if got := mustHashStringValue(t, result, "stored_"+algorithm); got != "" {
				t.Errorf("ewf_verify stored_%s is %q, but ewfinfo lists no stored %s for this image",
					algorithm, got, strings.ToUpper(algorithm))
			}
			continue
		}
		if got := mustHashStringValue(t, result, "stored_"+algorithm); got != wantStored {
			t.Errorf("ewf_verify stored_%s is %s, ewfinfo read %s out of the file",
				algorithm, got, wantStored)
		}
		// Where a digest IS stored, the match field has a comparison to
		// report and must agree with it.
		wantMatch := wantStored == wantComputed
		if got := mustHashBoolValue(t, result, algorithm+"_match"); got != wantMatch {
			t.Errorf("ewf_verify %s_match is %v, but the stored %s and the computed one %s",
				algorithm, got, strings.ToUpper(algorithm),
				map[bool]string{true: "are the same", false: "differ"}[wantMatch])
		}
		compared++
	}
	if compared == 0 {
		t.Fatal("the oracle supplied no digest to compare, so this test asserted nothing")
	}

	// ok is the summary verdict, and the contract is explicit: true only when
	// every stored digest was reproduced and no span failed to decode.
	storedMD5, haveMD5 := oracle.digest("md5", "stored")
	computedMD5, _ := oracle.digest("md5", "computed")
	wantOK := haveMD5 && storedMD5 == computedMD5
	if got := mustHashBoolValue(t, result, "ok"); got != wantOK {
		t.Errorf("ewf_verify ok is %v, want %v for an image whose stored MD5 %s what libewf computed",
			got, wantOK, map[bool]string{true: "equals", false: "differs from"}[haveMD5 && storedMD5 == computedMD5])
	}
	if !t.Failed() {
		t.Logf("reproduced %d digest(s) over %d bytes, byte for byte with ewfverify",
			compared, oracle.Media.MediaSize)
	}
}

// TestEWFReadsTheBytesEwfexportRead compares ewf_read_at against ewfexport
// window by window. This is the offset arithmetic, which the sequential verify
// never exercises.
func TestEWFReadsTheBytesEwfexportRead(t *testing.T) {
	oracle := ewfCorpusOracleFor(t)
	handle, ok := openCorpusEWF(t)
	if !ok {
		return
	}
	if len(oracle.Windows) == 0 {
		t.Fatal("the oracle records no windows, so this test would assert nothing")
	}

	compared := 0
	for _, want := range oracle.Windows {
		got, refusal := ewfCorpusReadAt(t, handle, want.Offset, want.Length)
		if refusal != "" {
			t.Errorf("ewf_read_at(%d, %d) refused with %q, and ewfexport wrote %d byte(s) for the "+
				"same range", want.Offset, want.Length, refusal, want.Returned)
			continue
		}
		if int64(len(got)) != want.Returned {
			t.Errorf("ewf_read_at(%d, %d) returned %d bytes, ewfexport wrote %d (oracle line %d)",
				want.Offset, want.Length, len(got), want.Returned, want.Line)
			continue
		}
		sum := sha256.Sum256(got)
		if digest := hex.EncodeToString(sum[:]); digest != want.SHA256 {
			t.Errorf("ewf_read_at(%d, %d): mutant's bytes hash to %s, ewfexport's to %s (oracle line %d)",
				want.Offset, want.Length, digest, want.SHA256, want.Line)
			continue
		}
		compared++
	}
	if compared == 0 {
		t.Fatalf("not one of the %d oracle windows was compared, so a green result here would mean "+
			"nothing", len(oracle.Windows))
	}
	t.Logf("compared %d of %d windows against ewfexport, byte for byte", compared, len(oracle.Windows))
}

// TestTheEWFFamilyAgreesWithItselfAboutTheMedia needs no oracle: it holds the
// builtins to each other and to arithmetic, so it still runs where only the
// images have been copied.
func TestTheEWFFamilyAgreesWithItselfAboutTheMedia(t *testing.T) {
	image, ok := corpusFile(t, ewfCorpusImage)
	if !ok {
		return
	}
	path := filepath.ToSlash(image)

	payload, errObj := unwrapPair(t, EWFSegments(stringObj(path)))
	if errObj != nil {
		t.Fatalf("ewf_segments: %s", errObj.Inspect())
	}
	segments := payload.(*object.Hash)
	segmentList := ewfCorpusStrings(t, segments, "segments")
	if int64(len(segmentList)) != mustHashIntValue(t, segments, "segment_count") {
		t.Errorf("ewf_segments lists %d path(s) and calls it a segment_count of %d",
			len(segmentList), mustHashIntValue(t, segments, "segment_count"))
	}
	missing, _ := ewfCorpusArray(t, segments, "missing_segments")
	if mustHashBoolValue(t, segments, "contiguous") && len(missing) != 0 {
		t.Errorf("ewf_segments calls the set contiguous and names %d missing segment(s)", len(missing))
	}

	handle, ok := openCorpusEWF(t)
	if !ok {
		return
	}
	payload, errObj = unwrapPair(t, EWFOpen(stringObj(path)))
	if errObj != nil {
		t.Fatalf("ewf_open a second time: %s", errObj.Inspect())
	}
	second := payload.(*object.Hash)
	t.Cleanup(func() { EWFClose(stringObj(mustHashStringValue(t, second, "handle"))) })
	openList := ewfCorpusStrings(t, second, "segments")
	if strings.Join(openList, "|") != strings.Join(segmentList, "|") {
		t.Errorf("ewf_open decoded %v and ewf_segments described %v for the same path", openList, segmentList)
	}

	meta := ewfCorpusMetadata(t, handle)
	sectors := mustHashIntValue(t, meta, "number_of_sectors")
	perSector := mustHashIntValue(t, meta, "bytes_per_sector")
	perChunk := mustHashIntValue(t, meta, "sectors_per_chunk")
	chunks := mustHashIntValue(t, meta, "number_of_chunks")
	logical := mustHashIntValue(t, meta, "total_logical_bytes")

	if perSector <= 0 || perChunk <= 0 {
		t.Fatalf("ewf_metadata reports %d bytes per sector and %d sectors per chunk, and a read "+
			"cannot be placed with either at zero", perSector, perChunk)
	}
	if sectors*perSector != logical {
		t.Errorf("ewf_metadata says %d sectors of %d bytes, which is %d, and calls the media %d bytes",
			sectors, perSector, sectors*perSector, logical)
	}
	if sectors/perChunk != chunks {
		t.Errorf("ewf_metadata says %d sectors at %d per chunk, which is %d chunks, and reports %d",
			sectors, perChunk, sectors/perChunk, chunks)
	}
	// sector_size sits beside bytes_per_sector. Two names for one quantity is
	// a wart rather than a defect, but they must not disagree: a caller that
	// picked the other one would place every read differently.
	if got := mustHashIntValue(t, meta, "sector_size"); got != perSector {
		t.Errorf("ewf_metadata publishes sector_size %d and bytes_per_sector %d for one volume",
			got, perSector)
	}
	// observed_chunk_count is what it actually walked. On a complete set it
	// must reach the declared count; it must never exceed it.
	observed := mustHashIntValue(t, meta, "observed_chunk_count")
	if observed > chunks {
		t.Errorf("ewf_metadata observed %d chunks in a set whose header declares %d", observed, chunks)
	}
	if !mustHashBoolValue(t, second, "partial") && observed != chunks {
		t.Errorf("ewf_metadata observed %d of the %d chunks the header declares, on a set ewf_open "+
			"did not call partial", observed, chunks)
	}
	// A digest field and its presence flag have to say the same thing, or a
	// report prints a digest the image does not carry.
	for _, algorithm := range []string{"md5", "sha1"} {
		has := mustHashBoolValue(t, meta, "has_"+algorithm+"_digest")
		value := mustHashStringValue(t, meta, algorithm+"_digest")
		if has && value == "" {
			t.Errorf("ewf_metadata has_%s_digest is true and %s_digest is empty", algorithm, algorithm)
		}
		if !has && value != "" {
			t.Errorf("ewf_metadata has_%s_digest is false and %s_digest is %q", algorithm, algorithm, value)
		}
	}

	// The two read builtins differ only in what they wrap the bytes in, so
	// they must carry the same bytes. If the STRING form were lossy for binary
	// data, every digest this file compares would be worthless.
	window := perChunk * perSector
	if window > 1<<20 {
		window = 1 << 20
	}
	asString, refusal := ewfCorpusReadAt(t, handle, 0, window)
	if refusal != "" {
		t.Fatalf("ewf_read_at(0, %d) refused with %q", window, refusal)
	}
	payload, errObj = unwrapPair(t, EWFReadAtBytes(stringObj(handle), intObj(0), intObj(window)))
	if errObj != nil {
		t.Fatalf("ewf_read_at_bytes(0, %d): %s", window, errObj.Inspect())
	}
	asBytes, okBytes := payload.(*object.Bytes)
	if !okBytes {
		t.Fatalf("ewf_read_at_bytes returned %s, want BYTES", payload.Type())
	}
	if string(asBytes.Value) != string(asString) {
		t.Errorf("ewf_read_at and ewf_read_at_bytes returned different bytes for the same %d-byte "+
			"window: %d bytes hashing to %x against %d hashing to %x",
			window, len(asString), sha256.Sum256(asString),
			len(asBytes.Value), sha256.Sum256(asBytes.Value))
	}
	t.Logf("%d sectors of %d in %d chunks, %d bytes of media, %d segment(s), and the two read "+
		"builtins agree over the first %d bytes", sectors, perSector, chunks, logical, len(segmentList), window)
}

// TestEWFRefusesAnIncompleteSetAndSaysWhenItIsPartial pins the refusal and the
// labelling, which three fixed rows were about: M26-FS1-009 (partial reported
// false for a set whose last segment has no done section), M26-FS1-010 (one
// path of a holed set opening only that file) and M26-FS1-018 (a one-element
// array expanded by discovery, contradicting its own documentation).
func TestEWFRefusesAnIncompleteSetAndSaysWhenItIsPartial(t *testing.T) {
	image, ok := corpusFile(t, ewfCorpusImage)
	if !ok {
		return
	}
	path := filepath.ToSlash(image)

	payload, errObj := unwrapPair(t, EWFSegments(stringObj(path)))
	if errObj != nil {
		t.Fatalf("ewf_segments: %s", errObj.Inspect())
	}
	if count := mustHashIntValue(t, payload.(*object.Hash), "segment_count"); count < 2 {
		t.Skipf("the set at %s has %d segment file(s), and an incomplete set needs at least two",
			ewfCorpusImage, count)
	}

	// A one-element array is taken as given and NOT expanded by discovery,
	// which is what M26-FS1-018 settled. So this names a set with everything
	// after the first segment missing.
	listed := &object.Array{Elements: []object.Object{stringObj(path)}}

	if _, errObj := unwrapPairNoFatal(EWFOpen(listed)); errObj == nil {
		t.Errorf("ewf_open accepted a set missing every segment after the first; it is supposed to " +
			"refuse and point at ewf_open_partial, because a digest computed over an incomplete set " +
			"describes something other than what was acquired")
	} else if !strings.Contains(errObj.Message, "incomplete") && !strings.Contains(errObj.Message, "missing") {
		t.Errorf("ewf_open refused the incomplete set with %q, which names neither what is missing "+
			"nor that the set is incomplete", errObj.Message)
	}

	payload, errObj = unwrapPair(t, EWFOpenPartial(listed))
	if errObj != nil {
		t.Fatalf("ewf_open_partial on the same one-element list: %s", errObj.Inspect())
	}
	openHash := payload.(*object.Hash)
	handle := mustHashStringValue(t, openHash, "handle")
	t.Cleanup(func() { EWFClose(stringObj(handle)) })

	if !mustHashBoolValue(t, openHash, "partial") {
		t.Errorf("ewf_open_partial opened a set missing every segment after the first and reported " +
			"partial false, which is what M26-FS1-009 was")
	}
	if got := mustHashIntValue(t, openHash, "segment_count"); got != 1 {
		t.Errorf("ewf_open_partial was given one path and decoded %d segment(s)", got)
	}

	meta := ewfCorpusMetadata(t, handle)
	observed := mustHashIntValue(t, meta, "observed_chunk_count")
	declared := mustHashIntValue(t, meta, "number_of_chunks")
	if observed >= declared {
		t.Errorf("on a set holding only its first segment, ewf_metadata says it observed %d of the "+
			"%d chunks the header declares; a count that reaches the header's figure would mean "+
			"nothing says the set is short", observed, declared)
	}
	// The hash section lives in the last segment file, so an image opened
	// without it carries no stored digest. Reporting one would be a
	// fabrication, and the absence is what makes ok false.
	if mustHashBoolValue(t, meta, "has_md5_digest") {
		t.Errorf("ewf_metadata reports a stored MD5 for a set whose last segment, which is where " +
			"the hash section lives, is missing")
	}
	t.Logf("ewf_open refused the one-segment set; ewf_open_partial opened it as partial, observing "+
		"%d of the %d chunks declared", observed, declared)
}

// TestEWFPartialReadsAreAPrefixOfTheWholeSetAndNeverPadded is the strongest
// check this file makes that needs no oracle.
//
// It reads the same offsets through two handles -- the complete set and the
// same set with everything after the first segment missing -- and requires that
// whatever the partial one produces is a byte-exact PREFIX of what the complete
// one produces at that offset. A reader that padded a short chunk with zeros,
// or served a neighbouring chunk's bytes, would fail here even though both
// handles would still agree about every length.
//
// It deliberately does not pin whether an offset in the absent segment refuses
// or answers empty. That is M26-FS1-024, and it is the fix's to decide.
func TestEWFPartialReadsAreAPrefixOfTheWholeSetAndNeverPadded(t *testing.T) {
	image, ok := corpusFile(t, ewfCorpusImage)
	if !ok {
		return
	}
	path := filepath.ToSlash(image)

	full, ok := openCorpusEWF(t)
	if !ok {
		return
	}
	fullMeta := ewfCorpusMetadata(t, full)
	media := mustHashIntValue(t, fullMeta, "total_logical_bytes")
	chunk := mustHashIntValue(t, fullMeta, "sectors_per_chunk") * mustHashIntValue(t, fullMeta, "bytes_per_sector")
	if mustHashIntValue(t, fullMeta, "number_of_chunks") < 2 {
		t.Skipf("the set holds %d chunk(s), and the boundary cases need more than one",
			mustHashIntValue(t, fullMeta, "number_of_chunks"))
	}

	listed := &object.Array{Elements: []object.Object{stringObj(path)}}
	payload, errObj := unwrapPairNoFatal(EWFOpenPartial(listed))
	if errObj != nil {
		t.Skipf("ewf_open_partial on a one-element list refused (%s), so there is no partial handle "+
			"to compare against", errObj.Message)
	}
	partial := mustHashStringValue(t, payload.(*object.Hash), "handle")
	t.Cleanup(func() { EWFClose(stringObj(partial)) })

	// The edge is discovered, not assumed: it is where the chunks this handle
	// actually observed run out. Pinning an offset would tie the test to one
	// generation of the corpus.
	observed := mustHashIntValue(t, ewfCorpusMetadata(t, partial), "observed_chunk_count")
	edge := observed * chunk
	if edge <= 0 || edge >= media {
		t.Skipf("the partial handle observed %d chunk(s), putting its edge at %d of %d bytes, which "+
			"is not a boundary this test can use", observed, edge, media)
	}

	type window struct {
		why    string
		offset int64
		length int64
	}
	windows := []window{
		{"the first chunk, which both handles hold", 0, chunk},
		{"wholly inside what the partial handle holds", edge - chunk, chunk},
		{"straddling the edge", edge - chunk/2, chunk},
		{"one byte before the edge", edge - 1, 1},
		{"at the edge", edge, chunk},
		{"well past the edge, still inside the declared media", edge + chunk, chunk},
	}

	comparedBytes := int64(0)
	compared, short := 0, 0
	for _, w := range windows {
		if w.offset < 0 || w.offset >= media {
			continue
		}
		fullBytes, fullRefusal := ewfCorpusReadAt(t, full, w.offset, w.length)
		if fullRefusal != "" {
			t.Errorf("ewf_read_at(%d, %d) on the COMPLETE set refused with %q, and every one of "+
				"these offsets is inside its %d bytes of media [%s]",
				w.offset, w.length, fullRefusal, media, w.why)
			continue
		}
		partialBytes, partialRefusal := ewfCorpusReadAt(t, partial, w.offset, w.length)
		if partialRefusal != "" {
			// A refusal is a perfectly good answer for an offset the handle
			// cannot serve, and it is one of the two answers M26-FS1-024 says
			// are defensible. Nothing to compare.
			continue
		}
		if len(partialBytes) > len(fullBytes) {
			t.Errorf("ewf_read_at(%d, %d) returned %d bytes from the set missing its later segments "+
				"and %d from the complete one [%s]",
				w.offset, w.length, len(partialBytes), len(fullBytes), w.why)
			continue
		}
		if len(partialBytes) == 0 {
			continue
		}
		if string(partialBytes) != string(fullBytes[:len(partialBytes)]) {
			t.Errorf("ewf_read_at(%d, %d): the partial set's %d bytes are not a prefix of the "+
				"complete set's. partial hashes to %x, the complete set's first %d bytes to %x [%s]",
				w.offset, w.length, len(partialBytes), sha256.Sum256(partialBytes),
				len(partialBytes), sha256.Sum256(fullBytes[:len(partialBytes)]), w.why)
			continue
		}
		// A short answer is only defensible if it stopped exactly where the
		// data stopped. Anything else is a truncation with no reason given.
		if int64(len(partialBytes)) < w.length {
			short++
			if end := w.offset + int64(len(partialBytes)); end != edge && end != media {
				t.Errorf("ewf_read_at(%d, %d) returned %d bytes, ending at %d, which is neither the "+
					"edge of what it holds (%d) nor the end of the media (%d) [%s]",
					w.offset, w.length, len(partialBytes), end, edge, media, w.why)
			}
		}
		compared++
		comparedBytes += int64(len(partialBytes))
	}
	if compared == 0 {
		t.Fatal("not one window was compared between the two handles, so this test asserted nothing")
	}
	t.Logf("compared %d windows and %d bytes between the complete set and the same set missing its "+
		"later segments; %d answer(s) were short, each ending exactly where the data does, and the "+
		"partial edge is at %d of %d bytes", compared, comparedBytes, short, edge, media)
}

// --- small readers over a result hash ---------------------------------------

func ewfCorpusArray(t *testing.T, h *object.Hash, field string) ([]object.Object, bool) {
	t.Helper()

	for _, pair := range h.Pairs {
		key, ok := pair.Key.(*object.String)
		if !ok || key.Value != field {
			continue
		}
		arr, okArr := pair.Value.(*object.Array)
		if !okArr {
			t.Fatalf("%s is %s, want ARRAY", field, pair.Value.Type())
		}
		return arr.Elements, true
	}
	return nil, false
}

func ewfCorpusStrings(t *testing.T, h *object.Hash, field string) []string {
	t.Helper()

	elements, found := ewfCorpusArray(t, h, field)
	if !found {
		t.Fatalf("no %s in the result", field)
	}
	out := make([]string, 0, len(elements))
	for _, el := range elements {
		text, ok := el.(*object.String)
		if !ok {
			t.Fatalf("%s holds a %s, want STRING", field, el.Type())
		}
		out = append(out, text.Value)
	}
	return out
}
