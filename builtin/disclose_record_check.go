package builtin

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"reflect"

	"mutant/security"
)

// disclosureRecordCheck is what one read of a record's file establishes: the
// digest and size of the bytes read, and whether the signature holds over those
// same bytes. The two answers come from the one pass, so a ledger that records
// the digest beside the signature bit is recording a fact about one file.
type disclosureRecordCheck struct {
	sha256         string
	bytes          int64
	signed         bool
	signatureValid bool
	signatureNote  string
}

// disclosureCheckRecordFile reads the file behind an open record once, from
// the first byte to the last, hashing every byte and folding the segments as
// it goes, and verifies the signature over what it read.
//
// The check record_open made is about the file as it was when it was opened.
// A file changed on disk since then would otherwise be hashed into the ledger
// beside a signature bit that was never true of it. So the header and footer
// read here must be byte for byte (header) and field for field (footer) the
// ones the open record holds -- the header is what every segment's descriptor
// and every grant is derived from -- and a file that differs in either is
// refused as a different file, not checked as if it were the same one.
func disclosureCheckRecordFile(record *recordSession) (disclosureRecordCheck, error) {
	file, err := os.Open(record.path)
	if err != nil {
		return disclosureRecordCheck{}, err
	}
	defer func() { _ = file.Close() }()

	hasher := sha256.New()
	reader := io.TeeReader(file, hasher)
	var read int64
	next := func(n uint64, what string) ([]byte, error) {
		chunk := make([]byte, n)
		got, err := io.ReadFull(reader, chunk)
		read += int64(got)
		if err != nil {
			return nil, fmt.Errorf("%s ends inside its %s, so it is not the record that was opened", record.path, what)
		}
		return chunk, nil
	}
	changed := func(what string) error {
		return fmt.Errorf("%s has a different %s from the record that was opened, so the file on disk has "+
			"changed since record_open; close the handle and open the record again", record.path, what)
	}

	prefix, err := next(uint64(security.RecordFilePrefixSize), "file prefix")
	if err != nil {
		return disclosureRecordCheck{}, err
	}
	headerLength, err := security.ParseRecordFilePrefix(prefix)
	if err != nil {
		return disclosureRecordCheck{}, err
	}
	if headerLength != len(record.headerRaw) {
		return disclosureRecordCheck{}, changed("header length")
	}
	headerRaw, err := next(uint64(headerLength), "header")
	if err != nil {
		return disclosureRecordCheck{}, err
	}
	if !bytes.Equal(headerRaw, record.headerRaw) {
		return disclosureRecordCheck{}, changed("header")
	}

	// The header is the open record's, so its segment table is too. Segments
	// are stored in index order with nothing between them (Segments gives
	// each one the stored offset where the one before it ended), which is
	// what lets one forward read fold them.
	total := uint64(len(record.segments))
	digests := make([][sha256.Size]byte, 0, len(record.segments))
	var stored uint64
	for _, segment := range record.segments {
		if segment.StoredOffset != stored {
			return disclosureRecordCheck{}, fmt.Errorf("%s: segment %d is stored at %d and the segment before it "+
				"ends at %d, so its segments cannot be read in one pass", record.path, segment.Index,
				segment.StoredOffset, stored)
		}
		chunk, err := next(segment.StoredLength, fmt.Sprintf("segment %d", segment.Index))
		if err != nil {
			return disclosureRecordCheck{}, err
		}
		aad, err := record.header.SegmentAAD(segment, total)
		if err != nil {
			return disclosureRecordCheck{}, err
		}
		digests = append(digests, security.SegmentDigest(aad, chunk))
		stored += segment.StoredLength
	}

	footerPrefix, err := next(uint64(security.RecordFooterPrefixSize), "footer prefix")
	if err != nil {
		return disclosureRecordCheck{}, err
	}
	footerLength, err := security.ParseRecordFooterPrefix(footerPrefix)
	if err != nil {
		return disclosureRecordCheck{}, err
	}
	footerRaw, err := next(uint64(footerLength), "footer")
	if err != nil {
		return disclosureRecordCheck{}, err
	}
	footer, err := security.ParseRecordFooter(footerRaw)
	if err != nil {
		return disclosureRecordCheck{}, err
	}
	if record.footer == nil || !reflect.DeepEqual(*footer, *record.footer) {
		return disclosureRecordCheck{}, changed("footer")
	}
	// Trailing bytes are bytes somebody added after the footer, and the
	// signature does not cover them; record_open refuses them for the same
	// reason.
	var one [1]byte
	if got, _ := reader.Read(one[:]); got != 0 {
		return disclosureRecordCheck{}, changed("length")
	}

	root := security.SegmentsRoot(digests)
	signed, valid, note := security.VerifyRecordSignature(footer, headerRaw, root)
	return disclosureRecordCheck{
		sha256:         hex.EncodeToString(hasher.Sum(nil)),
		bytes:          read,
		signed:         signed,
		signatureValid: valid,
		signatureNote:  note,
	}, nil
}
