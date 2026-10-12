package builtin

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"time"

	libntfs "github.com/aoiflux/libntfs"
	"mutant/object"
)

// mftRootRecord is the MFT record number of the volume root directory (".").
const mftRootRecord = 5

// MftParse decodes an NTFS Master File Table into a per-record timeline. It
// auto-detects its input: a standalone $MFT file (records beginning with the
// "FILE" signature, as extracted by KAPE/FTK/icat) is streamed record-by-record
// via libntfs.ParseMFTRecord; otherwise the path is opened as a full NTFS volume
// image and walked via libntfs EachMFTEntry.
//
// Each entry carries both the $STANDARD_INFORMATION and $FILE_NAME MAC times
// (unix + ISO-8601), the reconstructed full path, size, sequence number, and
// hard-link count, and the $FILE_NAME parent reference: parent_record and
// parent_sequence as the reference holds them (5 and 0 for a record with no
// $FILE_NAME), and parent_checked and parent_reused for what comparing it with
// the record now in that slot showed (see mftLinkToParent). Returns
// {source_type, record_size, count, skipped, entries} paired with an error;
// skipped counts records that would not parse.
func MftParse(args ...object.Object) (result object.Object) {
	defer func() {
		if r := recover(); r != nil {
			result = resultAndError(nil, newError("mft_parse: panic during parse: %v", r))
		}
	}()

	if len(args) != 1 {
		return resultAndError(nil, newError("wrong number of arguments. got=%d, want=1", len(args)))
	}
	path, errObj := requireStringArg(BuiltinNameMftParse, args[0], 1)
	if errObj != nil {
		return resultAndError(nil, errObj)
	}

	var rows []mftRow
	walk, err := walkMFT(path, func(n uint64, e *libntfs.MFTEntry) {
		rows = append(rows, mftRowFromEntry(e, n))
	})
	if err != nil {
		return resultAndError(nil, newError("mft_parse: %s", err.Error()))
	}

	paths := reconstructMFTPaths(rows)
	entries := make([]object.Object, len(rows))
	for i, r := range rows {
		entries[i] = r.toHash(paths[r.record])
	}

	return resultAndError(makeHashObject(map[string]object.Object{
		"source_type": stringObj(walk.sourceType),
		"record_size": intObj(int64(walk.recordSize)),
		"count":       intObj(int64(len(rows))),
		"skipped":     intObj(int64(walk.skipped)),
		"entries":     &object.Array{Elements: entries},
	}), nil)
}

// mftWalkResult describes what walkMFT found while iterating an MFT.
type mftWalkResult struct {
	// sourceType is "mft" for a standalone $MFT stream, "volume" for an image.
	sourceType string
	recordSize int
	// skipped counts records that would not parse and were stepped over. Some
	// unparsable records are normal in an extracted $MFT, but a count that
	// dwarfs the entry count means the table is damaged, and a caller cannot
	// tell that from the entries alone.
	skipped int
}

// walkMFT auto-detects the input (a standalone $MFT file whose records start with
// the "FILE" signature vs a full NTFS volume image) and invokes fn for every
// parsable MFT entry. Shared by mft_parse and fs_deleted.
func walkMFT(path string, fn func(recordNum uint64, e *libntfs.MFTEntry)) (mftWalkResult, error) {
	f, err := os.Open(path)
	if err != nil {
		return mftWalkResult{}, err
	}
	defer f.Close()

	peek := make([]byte, 4)
	if _, err := io.ReadFull(f, peek); err != nil {
		return mftWalkResult{}, err
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return mftWalkResult{}, err
	}

	if string(peek) == "FILE" {
		data, rerr := io.ReadAll(f)
		if rerr != nil {
			return mftWalkResult{}, rerr
		}
		rs := detectMFTRecordSize(data)
		res := mftWalkResult{sourceType: "mft", recordSize: rs}
		// Each record's fixup modifies its slice in place, which is safe because
		// every record is parsed exactly once.
		for off := 0; off+rs <= len(data); off += rs {
			entryNum := uint64(off / rs)
			e, perr := libntfs.ParseMFTRecord(data[off:off+rs], entryNum, uint32(rs))
			if perr != nil {
				// Unparsable records are expected in an extracted table; the
				// library's own guidance is to skip them and continue.
				res.skipped++
				continue
			}
			fn(entryNum, e)
		}
		return res, nil
	}

	vol, verr := libntfs.Open(f)
	if verr != nil {
		return mftWalkResult{}, fmt.Errorf("not a standalone $MFT (no FILE signature) and not a mountable NTFS volume: %s. For a full-disk image, locate the NTFS partition offset with table_* first", verr.Error())
	}
	defer vol.Close()

	res := mftWalkResult{sourceType: "volume"}
	if rs := vol.MFTRecordSize(); rs > 0 {
		res.recordSize = int(rs)
	}
	// Returning nil from the callback keeps the walk going past a bad record,
	// which is the library's documented idiom. A non-nil error here means the
	// walk itself aborted, so the entry list is truncated and reporting success
	// would misrepresent a damaged MFT as a complete one.
	if werr := vol.EachMFTEntry(func(n uint64, e *libntfs.MFTEntry) error {
		fn(n, e)
		return nil
	}); werr != nil {
		return res, werr
	}
	return res, nil
}

// detectMFTRecordSize reads the record size from the first record's own header
// rather than assuming it. libntfs.DefaultMFTRecordSize is the size ParseMFTRecord
// *assumes* when the caller passes 0; a volume formatted with 4096-byte records
// is legal, and striding it at 1024 lands three out of every four reads inside a
// record body and numbers the survivors wrongly.
func detectMFTRecordSize(data []byte) int {
	const allocatedSizeOffset = 0x1C // uint32 LE in the MFT record header
	if len(data) >= allocatedSizeOffset+4 {
		allocated := binary.LittleEndian.Uint32(data[allocatedSizeOffset:])
		// Power of two, sector-aligned, and within the range NTFS can express.
		if allocated >= 512 && allocated <= 65536 && allocated&(allocated-1) == 0 {
			return int(allocated)
		}
	}
	return libntfs.DefaultMFTRecordSize
}

// mftRow is the extracted, Volume-independent view of one MFT record. It copies
// out everything needed for output so results survive ParseMFTRecordInto reuse
// and volume-cache eviction.
type mftRow struct {
	record    uint64
	parent    uint64
	inUse     bool
	isDir     bool
	name      string
	size      uint64
	allocated uint64
	sequence  uint16
	hardLinks uint16
	fileAttrs uint32
	si        *libntfs.StandardInformation
	fn        *libntfs.FileName

	// parentSequence is the sequence number the $FILE_NAME parent reference
	// carries beside parent, which says which of the files that have held that
	// record was this one's directory. It is zero when the record has no
	// $FILE_NAME, and zero in a reference is NTFS's "no sequence number",
	// which asks for no check.
	parentSequence uint16
}

func mftRowFromEntry(e *libntfs.MFTEntry, recordNum uint64) mftRow {
	row := mftRow{
		record:    recordNum,
		parent:    mftRootRecord, // fallback until a $FILE_NAME says otherwise
		inUse:     e.IsInUse(),
		isDir:     e.IsDirectory(),
		sequence:  e.SequenceNum,
		hardLinks: e.HardLinkCount,
	}
	if si, err := e.GetStandardInformation(); err == nil && si != nil {
		row.si = si
		row.fileAttrs = si.FileAttributes
	}
	if fn, err := e.GetFileName(); err == nil && fn != nil {
		row.fn = fn
		row.name = fn.Name
		// libntfs hands the reference over in two fields: ParentDirectory holds
		// the record number and ParentSeqNum the sequence number. The second is
		// what tells the directory this file was in apart from a later file
		// that took over its record (M26-ART-023).
		row.parent = libntfs.FileReferenceEntry(fn.ParentDirectory)
		row.parentSequence = fn.ParentSeqNum
		row.size = fn.RealSize
		row.allocated = fn.AllocatedSize
	}
	// The size is the file's default $DATA stream's wherever this record holds
	// the start of it. $FILE_NAME carries a copy that NTFS updates lazily, and it
	// is routinely stale and often zero, so a file's size -- and the length a
	// recovery of a deleted one is cut to -- came from a figure the filesystem
	// does not keep current (M26-FS2-002). A stream whose first extent lives in
	// another record leaves the $FILE_NAME figure, the only one this record has.
	if data := mftDefaultDataStream(e); data != nil {
		switch {
		case data.Resident != nil:
			row.size = uint64(data.Resident.ValueLength)
		case data.NonResident != nil && data.NonResident.StartingVCN == 0:
			row.size, row.allocated = data.NonResident.RealSize, data.NonResident.AllocatedSize
		}
	}
	return row
}

// mftDefaultDataStream returns a record's unnamed $DATA attribute, or nil. It
// does not fall back to a named one, as libntfs's FindPrimaryDataAttribute does
// for reading: a file's size is its default stream's, and an alternate data
// stream's size in its place would be a different file's worth of bytes.
func mftDefaultDataStream(e *libntfs.MFTEntry) *libntfs.Attribute {
	for _, attr := range e.FindAllAttributes(libntfs.AttrTypeData) {
		switch {
		case attr.Resident != nil && attr.Resident.Name == "":
			return attr
		case attr.NonResident != nil && attr.NonResident.Name == "":
			return attr
		}
	}
	return nil
}

func (r mftRow) toHash(at mftPath) object.Object {
	m := map[string]object.Object{
		"record":          intObj(int64(r.record)),
		"parent_record":   intObj(int64(r.parent)),
		"parent_sequence": intObj(int64(r.parentSequence)),
		"parent_checked":  boolObj(at.link != mftParentUnchecked),
		"parent_reused":   boolObj(at.link == mftParentReused),
		"in_use":          boolObj(r.inUse),
		"is_directory":    boolObj(r.isDir),
		"name":            stringObj(r.name),
		"path":            stringObj(at.path),
		"size":            intObj(int64(r.size)),
		"allocated_size":  intObj(int64(r.allocated)),
		"sequence":        intObj(int64(r.sequence)),
		"hard_links":      intObj(int64(r.hardLinks)),
		"file_attributes": intObj(int64(r.fileAttrs)),
	}
	addMFTTimes(m, "si", r.si != nil, siTimes(r.si))
	addMFTTimes(m, "fn", r.fn != nil, fnTimes(r.fn))
	return makeHashObject(m)
}

// macTimes bundles the four NTFS timestamps in MACB order.
type macTimes struct{ modified, accessed, mftModified, created time.Time }

func siTimes(si *libntfs.StandardInformation) macTimes {
	if si == nil {
		return macTimes{}
	}
	return macTimes{si.ModifyTime, si.AccessTime, si.MFTChangeTime, si.CreateTime}
}

func fnTimes(fn *libntfs.FileName) macTimes {
	if fn == nil {
		return macTimes{}
	}
	return macTimes{fn.ModifyTime, fn.AccessTime, fn.MFTChangeTime, fn.CreateTime}
}

func addMFTTimes(m map[string]object.Object, prefix string, present bool, t macTimes) {
	if !present {
		return
	}
	for label, tv := range map[string]time.Time{
		"modified":     t.modified,
		"accessed":     t.accessed,
		"mft_modified": t.mftModified,
		"created":      t.created,
	} {
		unix, ns, iso := mftTimeFields(tv)
		m[prefix+"_"+label] = intObj(unix)
		// The sub-second (100 ns resolution) fraction, 0-999999999. NTFS records
		// timestamps at 100 ns; a value of exactly 0 across a file's SI times while
		// its FN times are non-zero is a strong timestomping tell (tools that set
		// whole-second times). The _iso string carries the same fraction.
		m[prefix+"_"+label+"_ns"] = intObj(ns)
		m[prefix+"_"+label+"_iso"] = stringObj(iso)
	}
}

// mftTimeFields returns a timestamp's unix seconds, its sub-second nanosecond
// fraction (0-999999999), and an RFC3339Nano string that preserves the fraction.
func mftTimeFields(t time.Time) (int64, int64, string) {
	if t.IsZero() {
		return 0, 0, ""
	}
	return t.Unix(), int64(t.Nanosecond()), t.UTC().Format(time.RFC3339Nano)
}

// mftParentLink is what comparing a record's $FILE_NAME parent reference with
// the record now at the number it names shows.
type mftParentLink int

const (
	// mftParentUnchecked: there was nothing to compare. The record has no
	// $FILE_NAME, its reference carries sequence number 0, NTFS's "no
	// sequence number", or the table holds no record at that number -- it
	// would not parse, or the table does not reach that far, as a partial
	// standalone $MFT may not.
	mftParentUnchecked mftParentLink = iota
	// mftParentSame: the record is still the directory the reference names,
	// in use or deleted.
	mftParentSame
	// mftParentReused: the record has been freed and given to another file
	// since the reference was written, so the directory it names is gone.
	mftParentReused
)

// mftPath is where reconstructMFTPaths places one record, and what that
// record's own parent reference showed.
type mftPath struct {
	path string
	link mftParentLink
}

// mftLinkToParent compares child's $FILE_NAME parent reference with the record
// at the number it names. An NTFS file reference is a record number and that
// record's sequence number, and NTFS reuses records: it adds one to a record's
// sequence number each time it frees the record, so the number tells apart the
// files that have held it. The record is still the directory the reference
// names when the two numbers agree, or when the record is free and carries the
// number freeing it made from the reference's -- the directory was deleted and
// nothing has taken its record since. Any other number means the record was
// freed and taken again after the reference was written.
func mftLinkToParent(byRecord map[uint64]mftRow, child mftRow) mftParentLink {
	if child.parentSequence == 0 {
		return mftParentUnchecked
	}
	parent, ok := byRecord[child.parent]
	if !ok {
		return mftParentUnchecked
	}
	switch {
	case parent.sequence == child.parentSequence:
		return mftParentSame
	case !parent.inUse && parent.sequence == mftSequenceAfterFree(child.parentSequence):
		return mftParentSame
	}
	return mftParentReused
}

// mftSequenceAfterFree is the sequence number NTFS gives a record when it
// frees it, if the record's sequence number was seq. The counter is circular
// and skips zero, the value a reference carries to mean "no sequence number".
func mftSequenceAfterFree(seq uint16) uint16 {
	next := seq + 1
	if next == 0 {
		return 1
	}
	return next
}

// reconstructMFTPaths walks each record's $FILE_NAME parent reference up to the
// root directory (record 5), producing a backslash-separated full path, and
// reports what each record's own reference showed (see mftLinkToParent). A
// chain that breaks is prefixed with "?" where it breaks: at a parent the table
// does not hold or that has no name (orphans, or a partial standalone $MFT), and
// at a reference whose parent's record has been given to another file since,
// rather than placing the file in whatever directory holds that record now.
// Record 5 is the root by number, so a reference to it resolves even when the
// table does not hold it. Cycles are bounded by a depth cap, which breaks a
// chain with "?" too: a walk that has followed 257 directories without
// reaching the root stops there (dirPrefix says what that leaves cached).
func reconstructMFTPaths(rows []mftRow) map[uint64]mftPath {
	byRecord := make(map[uint64]mftRow, len(rows))
	for _, r := range rows {
		byRecord[r.record] = r
	}

	// dirPrefix is the path of rec, the record a child names as its directory,
	// for a child whose own reference mftLinkToParent did not find reused. It
	// caches every prefix it builds. A prefix depends only on rec and the
	// records above it, except where the depth cap cut the walk short: a walk
	// that gives up leaves a cut-short prefix cached for each directory it
	// passed, and anything placed under one of those afterwards is cut at the
	// same place, even when its own chain to the root is shorter than the cap.
	prefixCache := make(map[uint64]string, len(rows))
	var dirPrefix func(rec uint64, depth int) string
	dirPrefix = func(rec uint64, depth int) string {
		if rec == mftRootRecord {
			return ""
		}
		if p, ok := prefixCache[rec]; ok {
			return p
		}
		if depth > 256 {
			return "?"
		}
		r, ok := byRecord[rec]
		if !ok || r.name == "" {
			return "?"
		}
		up := "?"
		if mftLinkToParent(byRecord, r) != mftParentReused {
			up = dirPrefix(r.parent, depth+1)
		}
		p := up + `\` + r.name
		prefixCache[rec] = p
		return p
	}

	paths := make(map[uint64]mftPath, len(rows))
	for _, r := range rows {
		at := mftPath{link: mftLinkToParent(byRecord, r)}
		switch {
		case r.record == mftRootRecord:
			at.path = `\`
		case r.name == "":
		case at.link == mftParentReused:
			at.path = `?\` + r.name
		default:
			at.path = dirPrefix(r.parent, 0) + `\` + r.name
		}
		paths[r.record] = at
	}
	return paths
}
