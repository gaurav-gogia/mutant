package builtin

// The NTFS readers measured against a real volume.
//
// What this file is for: two defects found on 2026-10-10 by walking a 1 GiB
// NTFS image and comparing every builtin that reports a length or a digest
// against every other one. Both were cases of a confident wrong answer rather
// than a refusal, which is the class this project treats as the serious one.
//
// M26-FS1-022. A path whose only $DATA attributes are named ones was served an
// alternate stream's bytes as its own. libntfs picks the attribute a whole-file
// read follows in FindPrimaryDataAttribute, which prefers the unnamed $DATA but
// returns the first named one when the entry has none, and nothing downstream
// could see that it had fallen back: ntfs_hash_file reported the alternate
// stream's digest under the file's name with truncated false and located_bytes
// equal to size, which is every indicator this project has for "the answer is
// whole" saying so at once. $Secure is in that state on a stock Windows volume;
// the case that matters is content moved into an alternate stream so that a
// whole-file read will not show it.
//
// M26-FS1-023. ntfs_list_files published the length the directory index keeps,
// which NTFS updates lazily. On this volume four live files listed as empty
// while holding 4,259,940 bytes between them, and $MFT listed 16,384 against an
// actual 2,097,152. M26-FS2-002 had already fixed the same stale field on the
// recovery path, where it truncated recovered files; its location did not cover
// the live listing.
//
// Three things shape how the assertions are written.
//
// Nothing is pinned. The corpus images are regenerated rather than archived, so
// a digest, a byte range or a named path could change for a reason that is not a
// defect -- and img3_ntfs.dd is the one corpus volume with no oracle file, so
// there is no recorded truth to compare against even if it were stable. Every
// test here discovers what the volume holds, classifies it through ntfs_streams,
// and asserts relations that hold for any NTFS volume: that the readers agree
// with each other, that a refusal is given where no answer exists, and that a
// stream which exists stays reachable by name.
//
// A count is asserted at the end of each walk. A wrong -corpus.dir, a volume
// without the shape under test, or a walk that silently reached nothing must not
// read as a green suite, so each test fails when it examined nothing and says
// which shape it was looking for.
//
// The population is checked, not just the example. A refusal that is right for
// $Secure would be wrong for /$Extend/$ObjId, /$Extend/$Quota and
// /$Extend/$Reparse, which have no $DATA attribute at all: they address nothing,
// read as zero bytes, and must keep doing so. Measured on this volume: of 1,680
// files exactly one has named $DATA streams and no unnamed one, three have no
// $DATA at all, and 1,676 are ordinary. TestNoUnnamedStreamRefusalDoesNotReach
// EntriesWithNoDataAtAll is what holds that line.

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"strings"
	"testing"

	"mutant/object"
)

const ntfsCorpusImage = "img3_ntfs.dd"

// ntfsCorpusEntry is one file as ntfs_list_files described it.
type ntfsCorpusEntry struct {
	path       string
	size       int64
	sizeSource string
}

func openCorpusNTFS(t *testing.T) (string, bool) {
	t.Helper()

	image, ok := corpusFile(t, ntfsCorpusImage)
	if !ok {
		return "", false
	}

	payload, errObj := unwrapPair(t, NtfsOpen(stringObj(image)))
	if errObj != nil {
		t.Fatalf("ntfs_open(%s): %s", image, errObj.Inspect())
	}
	openHash, okHash := payload.(*object.Hash)
	if !okHash {
		t.Fatalf("ntfs_open payload is not HASH. got=%T", payload)
	}

	handle := mustHashStringValue(t, openHash, "handle")
	t.Cleanup(func() { NtfsClose(stringObj(handle)) })

	return handle, true
}

// ntfsField reads one key of a result hash, for the keys a test has to ask
// about rather than require.
func ntfsField(h *object.Hash, key string) (object.Object, bool) {
	pair, found := h.Pairs[(&object.String{Value: key}).HashKey()]
	if !found {
		return nil, false
	}
	return pair.Value, true
}

// ntfsCorpusWalk descends the volume through ntfs_list_files, which is both the
// way to find every file and one of the two things under test. Deleted entries
// are left out: their length cannot be resolved from a $DATA attribute that the
// path no longer reaches, and M26-FS2-002 owns the recovery path.
func ntfsCorpusWalk(t *testing.T, handle, dir string, depth int, files *[]ntfsCorpusEntry, dirs *[]string) {
	t.Helper()

	if depth > 64 {
		t.Fatalf("ntfs_list_files descended past 64 levels at %s, which a 1 GiB volume has no reason to do", dir)
	}

	payload, errObj := unwrapPair(t, NtfsListFiles(stringObj(handle), stringObj(dir)))
	if errObj != nil {
		t.Fatalf("ntfs_list_files(%s): %s", dir, errObj.Inspect())
	}
	rows, okArr := payload.(*object.Array)
	if !okArr {
		t.Fatalf("ntfs_list_files payload is not ARRAY. got=%T", payload)
	}

	for _, element := range rows.Elements {
		row, okRow := element.(*object.Hash)
		if !okRow {
			t.Fatalf("ntfs_list_files element is not HASH. got=%T", element)
		}

		name := mustHashStringValue(t, row, "name")
		if name == "." || name == ".." {
			continue
		}
		if mustHashBoolValue(t, row, "deleted") {
			continue
		}

		path := mustHashStringValue(t, row, "path")
		if mustHashBoolValue(t, row, "is_dir") {
			*dirs = append(*dirs, path)
			ntfsCorpusWalk(t, handle, path, depth+1, files, dirs)
			continue
		}

		*files = append(*files, ntfsCorpusEntry{
			path:       path,
			size:       mustHashIntValue(t, row, "size"),
			sizeSource: mustHashStringValue(t, row, "size_source"),
		})
	}
}

// ntfsStreamShape is what ntfs_streams says about one path: how many $DATA
// attributes it has and whether any of them is the unnamed one.
type ntfsStreamShape struct {
	total          int64
	alternate      int64
	defaultPresent bool
	names          []string
}

func ntfsShapeOf(t *testing.T, handle, path string) (ntfsStreamShape, bool) {
	t.Helper()

	payload, errObj := unwrapPair(t, NtfsStreams(stringObj(handle), stringObj(path)))
	if errObj != nil {
		return ntfsStreamShape{}, false
	}
	scan, okScan := payload.(*object.Hash)
	if !okScan {
		t.Fatalf("ntfs_streams payload is not HASH. got=%T", payload)
	}

	shape := ntfsStreamShape{
		total:          mustHashIntValue(t, scan, "stream_count"),
		alternate:      mustHashIntValue(t, scan, "alternate_count"),
		defaultPresent: mustHashBoolValue(t, scan, "default_present"),
	}

	if streams, found := ntfsField(scan, "streams"); found {
		if arr, okArr := streams.(*object.Array); okArr {
			for _, element := range arr.Elements {
				if entry, okEntry := element.(*object.Hash); okEntry {
					if mustHashBoolValue(t, entry, "is_alternate") {
						shape.names = append(shape.names, mustHashStringValue(t, entry, "name"))
					}
				}
			}
		}
	}

	return shape, true
}

// TestNoUnnamedStreamIsRefusedRatherThanAnswered is M26-FS1-022's regression
// test. Before the fix every one of these readers answered for $Secure, and the
// answer was the $SDS stream.
func TestNoUnnamedStreamIsRefusedRatherThanAnswered(t *testing.T) {
	handle, ok := openCorpusNTFS(t)
	if !ok {
		t.Skipf("%s is not in this copy of the corpus", ntfsCorpusImage)
	}

	var files []ntfsCorpusEntry
	var dirs []string
	ntfsCorpusWalk(t, handle, "/", 0, &files, &dirs)

	examined := 0
	for _, file := range files {
		shape, okShape := ntfsShapeOf(t, handle, file.path)
		if !okShape || shape.defaultPresent || shape.total == 0 {
			continue
		}
		examined++

		t.Run(file.path, func(t *testing.T) {
			// Every whole-file reader has to refuse, and for this reason
			// rather than by accident: the message names the streams that do
			// exist, because a refusal that does not say what to ask for
			// instead reads as "this file is unreadable".
			for _, reader := range []struct {
				name string
				call func() object.Object
			}{
				{"ntfs_read_file", func() object.Object {
					return NtfsReadFile(stringObj(handle), stringObj(file.path))
				}},
				{"ntfs_read_file_bytes", func() object.Object {
					return NtfsReadFileBytes(stringObj(handle), stringObj(file.path))
				}},
				{"ntfs_read_file_at", func() object.Object {
					return NtfsReadFileAt(stringObj(handle), stringObj(file.path), intObj(0), intObj(32))
				}},
				{"ntfs_hash_file", func() object.Object {
					return NtfsHashFile(stringObj(handle), stringObj(file.path), stringObj("sha256"))
				}},
				{"ntfs_extract_file", func() object.Object {
					dest := filepath.Join(t.TempDir(), "unnamed.bin")
					return NtfsExtractFile(stringObj(handle), stringObj(file.path), stringObj(dest))
				}},
			} {
				value, errObj := unwrapPair(t, reader.call())
				if errObj == nil {
					t.Errorf("%s(%s) answered instead of refusing: %s -- the path has %d $DATA attributes and none of them is the unnamed one, so whatever it returned belongs to a named stream",
						reader.name, file.path, value.Inspect(), shape.total)
					continue
				}
				message := errObj.Inspect()
				for _, want := range []string{"unnamed $DATA", "ntfs_read_stream"} {
					if !strings.Contains(message, want) {
						t.Errorf("%s(%s) refused without mentioning %q, so the caller is not told what to ask for instead: %s",
							reader.name, file.path, want, message)
					}
				}
				for _, name := range shape.names {
					if !strings.Contains(message, name) {
						t.Errorf("%s(%s) refused without naming the stream %q that holds its bytes: %s",
							reader.name, file.path, name, message)
					}
				}
			}

			// ntfs_metadata describes rather than reads, so it answers -- but
			// about the path, which addresses no content.
			payload, errObj := unwrapPair(t, NtfsMetadata(stringObj(handle), stringObj(file.path)))
			if errObj != nil {
				t.Fatalf("ntfs_metadata(%s) refused; it describes a path and this path exists: %s", file.path, errObj.Inspect())
			}
			meta, okMeta := payload.(*object.Hash)
			if !okMeta {
				t.Fatalf("ntfs_metadata payload is not HASH. got=%T", payload)
			}
			if size := mustHashIntValue(t, meta, "size"); size != 0 {
				t.Errorf("ntfs_metadata(%s) size = %d, want 0: the path names no content of its own, and a non-zero size here is the named stream's length under the file's name",
					file.path, size)
			}
			if mustHashBoolValue(t, meta, "has_data") {
				t.Errorf("ntfs_metadata(%s) has_data = true, want false: libntfs reports it true because it selected a named $DATA attribute", file.path)
			}
			if blocking := mustHashStringValue(t, meta, "blocking_error"); blocking == "" {
				t.Errorf("ntfs_metadata(%s) blocking_error is empty, so nothing in the result says why size is 0", file.path)
			}

			// The stream itself must stay reachable. A fix that made the
			// content unreadable would pass every assertion above and lose
			// evidence, which is the failure this guards against.
			for _, name := range shape.names {
				streamPayload, streamErr := unwrapPair(t, NtfsStreams(stringObj(handle), stringObj(file.path)))
				if streamErr != nil {
					t.Fatalf("ntfs_streams(%s): %s", file.path, streamErr.Inspect())
				}
				scan, okScan := streamPayload.(*object.Hash)
				if !okScan {
					t.Fatalf("ntfs_streams payload is not HASH. got=%T", streamPayload)
				}

				want := ntfsDeclaredStreamSize(t, scan, name)
				readPayload, readErr := unwrapPair(t, NtfsReadStream(
					stringObj(handle), stringObj(file.path), stringObj(name),
					intObj(0), intObj(want)))
				if readErr != nil {
					t.Fatalf("ntfs_read_stream(%s, %s) refused, so the fix made the content unreachable: %s",
						file.path, name, readErr.Inspect())
				}
				buffer, okBytes := readPayload.(*object.Bytes)
				if !okBytes {
					t.Fatalf("ntfs_read_stream payload is not BYTES. got=%T", readPayload)
				}
				if int64(len(buffer.Value)) != want {
					t.Errorf("ntfs_read_stream(%s, %s) returned %d bytes, want the %d it declares",
						file.path, name, len(buffer.Value), want)
				}

				// And the digest of those bytes is reproducible, which is what
				// makes the stream usable as evidence now that the path itself
				// refuses.
				sum := sha256.Sum256(buffer.Value)
				if first, second := hex.EncodeToString(sum[:]), ntfsStreamDigest(t, handle, file.path, name, want); first != second {
					t.Errorf("ntfs_read_stream(%s, %s) gave two different digests on two reads: %s then %s",
						file.path, name, first, second)
				}
			}
		})
	}

	if examined == 0 {
		t.Fatalf("no file on %s has $DATA attributes with no unnamed one among them, which is the shape this test exists for -- on a stock Windows volume /$Secure is in it, so either the corpus image changed or the walk reached nothing",
			ntfsCorpusImage)
	}
	t.Logf("examined %d path(s) with named $DATA streams and no unnamed one, out of %d files in %d directories",
		examined, len(files), len(dirs))
}

// TestNoUnnamedStreamRefusalDoesNotReachEntriesWithNoDataAtAll is the
// population half of M26-FS1-022. An entry with no $DATA attribute addresses
// nothing and reads as empty; turning that into a refusal would be a second
// defect wearing the first one's fix.
func TestNoUnnamedStreamRefusalDoesNotReachEntriesWithNoDataAtAll(t *testing.T) {
	handle, ok := openCorpusNTFS(t)
	if !ok {
		t.Skipf("%s is not in this copy of the corpus", ntfsCorpusImage)
	}

	var files []ntfsCorpusEntry
	var dirs []string
	ntfsCorpusWalk(t, handle, "/", 0, &files, &dirs)

	const emptySHA256 = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

	examined := 0
	for _, file := range files {
		shape, okShape := ntfsShapeOf(t, handle, file.path)
		if !okShape || shape.total != 0 {
			continue
		}
		examined++

		t.Run(file.path, func(t *testing.T) {
			payload, errObj := unwrapPair(t, NtfsReadFile(stringObj(handle), stringObj(file.path)))
			if errObj != nil {
				t.Fatalf("ntfs_read_file(%s) refused, but the path has no $DATA attribute at all: it addresses nothing and reading it is answerable with zero bytes. %s",
					file.path, errObj.Inspect())
			}
			text, okText := payload.(*object.String)
			if !okText {
				t.Fatalf("ntfs_read_file payload is not STRING. got=%T", payload)
			}
			if len(text.Value) != 0 {
				t.Errorf("ntfs_read_file(%s) returned %d bytes for a path with no $DATA attribute", file.path, len(text.Value))
			}

			hashPayload, hashErr := unwrapPair(t, NtfsHashFile(stringObj(handle), stringObj(file.path), stringObj("sha256")))
			if hashErr != nil {
				t.Fatalf("ntfs_hash_file(%s) refused a path that reads as empty: %s", file.path, hashErr.Inspect())
			}
			result, okResult := hashPayload.(*object.Hash)
			if !okResult {
				t.Fatalf("ntfs_hash_file payload is not HASH. got=%T", hashPayload)
			}
			if size := mustHashIntValue(t, result, "size"); size != 0 {
				t.Errorf("ntfs_hash_file(%s) size = %d, want 0", file.path, size)
			}
			if digest := mustHashStringValue(t, result, "digest"); digest != emptySHA256 {
				t.Errorf("ntfs_hash_file(%s) digest = %s, want the digest of no bytes %s", file.path, digest, emptySHA256)
			}
			if mustHashBoolValue(t, result, "truncated") {
				t.Errorf("ntfs_hash_file(%s) truncated = true for an empty read", file.path)
			}
		})
	}

	if examined == 0 {
		t.Fatalf("no file on %s has zero $DATA attributes, which is the population this test protects -- /$Extend/$ObjId, /$Extend/$Quota and /$Extend/$Reparse are in that state on a stock volume",
			ntfsCorpusImage)
	}
	t.Logf("examined %d path(s) with no $DATA attribute at all", examined)
}

// TestDirectoriesStillRefuseForBeingDirectories keeps the no-unnamed-stream
// check from reaching directories. A directory never has an unnamed $DATA and
// can carry named streams, so a test for the missing unnamed stream alone would
// refuse every directory with the wrong reason -- and ntfs_metadata has to keep
// describing them.
func TestDirectoriesStillRefuseForBeingDirectories(t *testing.T) {
	handle, ok := openCorpusNTFS(t)
	if !ok {
		t.Skipf("%s is not in this copy of the corpus", ntfsCorpusImage)
	}

	var files []ntfsCorpusEntry
	var dirs []string
	ntfsCorpusWalk(t, handle, "/", 0, &files, &dirs)

	examined := 0
	for _, dir := range append([]string{"/"}, dirs...) {
		_, errObj := unwrapPair(t, NtfsReadFile(stringObj(handle), stringObj(dir)))
		if errObj == nil {
			t.Errorf("ntfs_read_file(%s) answered for a directory", dir)
			continue
		}
		if !strings.Contains(errObj.Inspect(), "is a directory") {
			t.Errorf("ntfs_read_file(%s) refused a directory for the wrong reason: %s", dir, errObj.Inspect())
		}

		payload, metaErr := unwrapPair(t, NtfsMetadata(stringObj(handle), stringObj(dir)))
		if metaErr != nil {
			t.Errorf("ntfs_metadata(%s) refused a directory it should describe: %s", dir, metaErr.Inspect())
			continue
		}
		meta, okMeta := payload.(*object.Hash)
		if !okMeta {
			t.Fatalf("ntfs_metadata payload is not HASH. got=%T", payload)
		}
		if !mustHashBoolValue(t, meta, "is_dir") {
			t.Errorf("ntfs_metadata(%s) is_dir = false for a directory", dir)
		}
		examined++
	}

	if examined == 0 {
		t.Fatalf("no directory was reached on %s", ntfsCorpusImage)
	}
	t.Logf("examined %d directories", examined)
}

// TestListedSizeIsTheDataAttributeAndNotTheIndexCopy is M26-FS1-023's
// regression test. Before the fix /$MFT listed 16,384 against an actual
// 2,097,152 and four live files listed as empty while holding 4,259,940 bytes.
func TestListedSizeIsTheDataAttributeAndNotTheIndexCopy(t *testing.T) {
	handle, ok := openCorpusNTFS(t)
	if !ok {
		t.Skipf("%s is not in this copy of the corpus", ntfsCorpusImage)
	}

	var files []ntfsCorpusEntry
	var dirs []string
	ntfsCorpusWalk(t, handle, "/", 0, &files, &dirs)

	bySource := map[string]int{}
	disagreements := 0

	for _, file := range files {
		switch file.sizeSource {
		case ntfsSizeFromData, ntfsSizeNoData, ntfsSizeFromIndex:
			bySource[file.sizeSource]++
		default:
			t.Errorf("ntfs_list_files(%s) size_source = %q, which is not one of %q, %q, %q -- a caller cannot tell whether the size is authoritative",
				file.path, file.sizeSource, ntfsSizeFromData, ntfsSizeNoData, ntfsSizeFromIndex)
			continue
		}

		payload, errObj := unwrapPair(t, NtfsMetadata(stringObj(handle), stringObj(file.path)))
		if errObj != nil {
			t.Errorf("ntfs_metadata(%s): %s", file.path, errObj.Inspect())
			continue
		}
		meta, okMeta := payload.(*object.Hash)
		if !okMeta {
			t.Fatalf("ntfs_metadata payload is not HASH. got=%T", payload)
		}
		metaSize := mustHashIntValue(t, meta, "size")

		// The listing and the metadata reader are two routes to one number,
		// and the whole of M26-FS1-023 is that they took it from different
		// places. They have to agree for every live file.
		if file.size != metaSize {
			disagreements++
			if disagreements <= 20 {
				t.Errorf("ntfs_list_files(%s) size = %d but ntfs_metadata says %d (size_source %q)",
					file.path, file.size, metaSize, file.sizeSource)
			}
			continue
		}

		// And the label has to match what was actually available.
		switch {
		case file.sizeSource == ntfsSizeNoData && file.size != 0:
			t.Errorf("ntfs_list_files(%s) says %q yet reports %d bytes", file.path, ntfsSizeNoData, file.size)
		case file.sizeSource == ntfsSizeFromData && !mustHashBoolValue(t, meta, "has_data"):
			t.Errorf("ntfs_list_files(%s) says the size came from a $DATA attribute, but ntfs_metadata reports has_data false",
				file.path)
		}
	}

	// A directory has no $DATA attribute, so it says so and reports nothing.
	for _, dir := range dirs {
		payload, errObj := unwrapPair(t, NtfsListFiles(stringObj(handle), stringObj(dir)))
		if errObj != nil {
			t.Fatalf("ntfs_list_files(%s): %s", dir, errObj.Inspect())
		}
		rows, okArr := payload.(*object.Array)
		if !okArr {
			t.Fatalf("ntfs_list_files payload is not ARRAY. got=%T", payload)
		}
		for _, element := range rows.Elements {
			row, okRow := element.(*object.Hash)
			if !okRow || !mustHashBoolValue(t, row, "is_dir") {
				continue
			}
			path := mustHashStringValue(t, row, "path")
			if source := mustHashStringValue(t, row, "size_source"); source != ntfsSizeNoData {
				t.Errorf("ntfs_list_files listed the directory %s with size_source %q, want %q", path, source, ntfsSizeNoData)
			}
			if size := mustHashIntValue(t, row, "size"); size != 0 {
				t.Errorf("ntfs_list_files listed the directory %s with size %d, want 0", path, size)
			}
		}
	}

	if len(files) == 0 {
		t.Fatalf("no file was listed on %s", ntfsCorpusImage)
	}
	if bySource[ntfsSizeFromData] == 0 {
		t.Errorf("not one listed file reported a size resolved from its $DATA attribute, so the listing is still publishing the index copy for everything")
	}
	t.Logf("%d files in %d directories: %d resolved from $DATA, %d with no $DATA, %d from the index copy",
		len(files), len(dirs), bySource[ntfsSizeFromData], bySource[ntfsSizeNoData], bySource[ntfsSizeFromIndex])
}

// ntfsDeclaredStreamSize is the length ntfs_streams gives for one named stream.
func ntfsDeclaredStreamSize(t *testing.T, scan *object.Hash, name string) int64 {
	t.Helper()

	streams, found := ntfsField(scan, "streams")
	if !found {
		t.Fatalf("ntfs_streams result carries no streams array")
	}
	arr, okArr := streams.(*object.Array)
	if !okArr {
		t.Fatalf("ntfs_streams streams is not ARRAY. got=%T", streams)
	}
	for _, element := range arr.Elements {
		entry, okEntry := element.(*object.Hash)
		if !okEntry {
			continue
		}
		if mustHashStringValue(t, entry, "name") == name {
			return mustHashIntValue(t, entry, "size")
		}
	}
	t.Fatalf("ntfs_streams does not list a stream named %q", name)
	return 0
}

// ntfsStreamDigest re-reads one stream and digests it, so a caller can compare
// two independent reads of the same bytes.
func ntfsStreamDigest(t *testing.T, handle, path, name string, length int64) string {
	t.Helper()

	payload, errObj := unwrapPair(t, NtfsReadStream(
		stringObj(handle), stringObj(path), stringObj(name), intObj(0), intObj(length)))
	if errObj != nil {
		t.Fatalf("ntfs_read_stream(%s, %s) refused on re-read: %s", path, name, errObj.Inspect())
	}
	buffer, okBytes := payload.(*object.Bytes)
	if !okBytes {
		t.Fatalf("ntfs_read_stream payload is not BYTES. got=%T", payload)
	}
	sum := sha256.Sum256(buffer.Value)
	return hex.EncodeToString(sum[:])
}
