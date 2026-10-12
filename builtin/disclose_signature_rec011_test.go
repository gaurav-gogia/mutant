package builtin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mutant/object"
)

// rec011TamperedCopy writes a copy of the fixture's record with one byte of its
// last segment flipped, opens it, and returns its handle. The copy still
// claims its signature, and the signature no longer holds: the case the
// two-bit contract exists for.
func rec011TamperedCopy(t *testing.T, f *discloseFixture) object.Object {
	t.Helper()
	layout := mustHash(t, RecordLayout(f.record))
	segments, _ := mustHashValue(t, layout, "segments").(*object.Array)
	last, _ := segments.Elements[len(segments.Elements)-1].(*object.Hash)
	at := mustHashValue(t, last, "stored_offset").(*object.Integer).Value
	raw, err := os.ReadFile(f.recordPath)
	if err != nil {
		t.Fatal(err)
	}
	raw[at] ^= 1
	tampered := filepath.Join(t.TempDir(), "tampered.mrec")
	if err := os.WriteFile(tampered, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	verified := mustHash(t, RecordVerify(stringObj(tampered)))
	if !mustHashBoolValue(t, verified, "signed") || mustHashBoolValue(t, verified, "signature_valid") {
		t.Fatal("the tampered copy is not a signed record whose signature fails, so it tests nothing here")
	}
	handle := mustHashValue(t, mustHash(t, RecordOpen(stringObj(tampered))), "handle")
	t.Cleanup(func() { RecordClose(handle) })
	return handle
}

// rec011RecordNode returns the ledger's Record node for the fixture's record.
func rec011RecordNode(t *testing.T, f *discloseFixture) (disclosureNode, bool) {
	t.Helper()
	session, ok := ledgerGet(f.ledger)
	if !ok {
		t.Fatal("the fixture's ledger is not open")
	}
	node, found, err := disclosureFind(session.graph, disclosureNodeRecord, "record.uid",
		strings.ToLower(recordUIDOf(t, f.record)))
	if err != nil {
		t.Fatal(err)
	}
	return node, found
}

// M26-REC-011: a signed record whose signature no longer verifies was
// disclosed without a word, and the ledger's Record node said
// record.signed=true with nothing beside it saying the signature failed.
func TestRec011DiscloseRefusesARecordWhoseSignatureFails(t *testing.T) {
	f := newDiscloseFixture(t)
	if !mustHashBoolValue(t, mustHash(t, RecordVerify(stringObj(f.recordPath))), "signed") {
		t.Skip("this machine has no key store, so there is no signature to break")
	}
	tampered := rec011TamperedCopy(t, f)

	f.assign(t, "Counsel")
	purposeStub(t, map[string]string{BuiltinNameDiscloseToPassphrase: testGrantPassphrase})
	mustRefuse(t, "a disclosure of a record whose signature fails",
		DiscloseToPassphrase(intObj(f.ledger), tampered, stringObj("counsel"), stringObj("Counsel")),
		"signature")
	if _, found := rec011RecordNode(t, f); found {
		t.Fatal("a refused disclosure still wrote the record into the ledger")
	}

	// The intact record still discloses, and its node says both bits.
	f.issue(t, "counsel", "Counsel")
	node, found := rec011RecordNode(t, f)
	if !found {
		t.Fatal("the disclosure wrote no Record node")
	}
	if got := node.get("record.signed"); got != "true" {
		t.Fatalf("record.signed = %q, want true", got)
	}
	if got := node.get("record.signature_valid"); got != "true" {
		t.Fatalf("record.signature_valid = %q, want true", got)
	}
}

// rec011OpenThenChange opens an intact copy of the fixture's record and then
// changes the file on disk under the open handle: change gets the file's bytes
// and returns what to write back. record_open's check passed; the file the
// handle names no longer is the one it checked.
func rec011OpenThenChange(t *testing.T, f *discloseFixture, change func([]byte) []byte) object.Object {
	t.Helper()
	raw, err := os.ReadFile(f.recordPath)
	if err != nil {
		t.Fatal(err)
	}
	copyPath := filepath.Join(t.TempDir(), "changed-after-open.mrec")
	if err := os.WriteFile(copyPath, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	opened := mustHash(t, RecordOpen(stringObj(copyPath)))
	if !mustHashBoolValue(t, opened, "signature_valid") {
		t.Fatal("the copy's signature failed at record_open, so changing it afterwards tests nothing here")
	}
	handle := mustHashValue(t, opened, "handle")
	t.Cleanup(func() { RecordClose(handle) })
	if err := os.WriteFile(copyPath, change(append([]byte(nil), raw...)), 0o600); err != nil {
		t.Fatal(err)
	}
	return handle
}

// rec011LastSegmentAt is the file offset of the first stored byte of the
// fixture record's last segment.
func rec011LastSegmentAt(t *testing.T, f *discloseFixture) int64 {
	t.Helper()
	layout := mustHash(t, RecordLayout(f.record))
	segments, _ := mustHashValue(t, layout, "segments").(*object.Array)
	last, _ := segments.Elements[len(segments.Elements)-1].(*object.Hash)
	return mustHashValue(t, last, "stored_offset").(*object.Integer).Value
}

// record_open checks the signature once. A file changed on disk after that was
// still disclosed, and the ledger hashed the changed bytes into a Record node
// that said the signature held: the check has to be over the bytes the ledger
// records, made when it records them.
func TestRec011DiscloseRefusesARecordChangedAfterOpen(t *testing.T) {
	cases := []struct {
		name   string
		change func(t *testing.T, f *discloseFixture, raw []byte) []byte
		want   string
	}{
		{"a segment byte flipped", func(t *testing.T, f *discloseFixture, raw []byte) []byte {
			raw[rec011LastSegmentAt(t, f)] ^= 1
			return raw
		}, "signature"},
		{"a byte added after the footer", func(t *testing.T, f *discloseFixture, raw []byte) []byte {
			return append(raw, 0)
		}, "changed since record_open"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newDiscloseFixture(t)
			if !mustHashBoolValue(t, mustHash(t, RecordVerify(stringObj(f.recordPath))), "signed") {
				t.Skip("this machine has no key store, so there is no signature to break")
			}
			handle := rec011OpenThenChange(t, f, func(raw []byte) []byte { return c.change(t, f, raw) })
			f.assign(t, "Counsel")
			purposeStub(t, map[string]string{BuiltinNameDiscloseToPassphrase: testGrantPassphrase})
			mustRefuse(t, "a disclosure of a record changed on disk after record_open",
				DiscloseToPassphrase(intObj(f.ledger), handle, stringObj("counsel"), stringObj("Counsel")),
				c.want)
			if node, found := rec011RecordNode(t, f); found {
				t.Fatalf("a refused disclosure still wrote the record into the ledger: record.signature_valid = %q",
					node.get("record.signature_valid"))
			}
		})
	}
}

// The Record node is written by more than disclose_to_passphrase, and the
// other writers hash the file without verifying its signature over what they
// hashed. They write no record.signature_valid: record_open's answer is about
// the file as it was when it was opened, and copying it beside a digest of the
// file as it is now said a signature held over bytes it did not hold over.
func TestRec011RecordNodeWithoutACheckMakesNoSignatureClaim(t *testing.T) {
	f := newDiscloseFixture(t)
	if !mustHashBoolValue(t, mustHash(t, RecordVerify(stringObj(f.recordPath))), "signed") {
		t.Skip("this machine has no key store, so there is no signature to break")
	}
	handle := rec011OpenThenChange(t, f, func(raw []byte) []byte {
		raw[rec011LastSegmentAt(t, f)] ^= 1
		return raw
	})
	mustHash(t, RedactionCommit(intObj(f.ledger), handle, stringObj("counsel"),
		stringObj("a redaction of a copy changed after it was opened")))
	node, found := rec011RecordNode(t, f)
	if !found {
		t.Fatal("the redaction wrote no Record node")
	}
	if got := node.get("record.signed"); got != "true" {
		t.Fatalf("record.signed = %q, want true: the copy does carry a signature", got)
	}
	if got, present := node.props["record.signature_valid"]; present {
		t.Fatalf("record.signature_valid = %q from a writer that never checked the signature over the bytes "+
			"it hashed; it must be absent", got)
	}
}
