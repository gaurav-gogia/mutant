package builtin

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mutant/object"
)

// discloseSEC013RecordSignatureCheck runs disclose_verify's record check over a
// directory holding only record.mrec and returns the record_signature check.
func discloseSEC013RecordSignatureCheck(t *testing.T, dir string) (passed bool, detail string) {
	t.Helper()
	v := &discloseVerification{op: "disclose_verify", dir: dir, manifest: map[string]any{}}
	v.checkRecord()
	if v.record != nil {
		v.record.file.Close()
	}
	for _, c := range v.checks {
		if c.name == "record_signature" {
			return c.passed, c.detail
		}
	}
	t.Fatalf("checkRecord added no record_signature check: %+v", v.checks)
	return false, ""
}

// discloseSEC013WriteRecord writes raw as dir/record.mrec, the name
// disclose_verify reads.
func discloseSEC013WriteRecord(t *testing.T, raw []byte) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, discloseRecordName), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

// A record whose footer says "signed":false but keeps its real signer and
// signature is refused by security.VerifyRecordSignature as a footer that
// disagrees with itself (M26-SEC-013). disclose_verify must fail the
// record_signature check, and must not tell the reader the record "carries no
// signature": it carries one, which is the whole of the contradiction.
func TestDiscloseVerifySEC013SelfContradictingFooterIsNotCalledUnsigned(t *testing.T) {
	useTestKeyStore(t)
	recordTestCase(t)
	source, dest, _ := recordFixture(t)
	sealed := mustHash(t, RecordSeal(stringObj(source), stringObj(dest), recordArray(),
		recordSealOpts(map[string]object.Object{"sign": boolObj(true)})))
	if !mustHashBoolValue(t, sealed, "signed") {
		t.Fatal("the fixture record was not signed, so there is no signature to keep beside signed:false")
	}
	raw, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	// The footer is the trailing JSON document, behind a 4-byte big-endian
	// length prefix.
	at := bytes.LastIndex(raw, []byte(`{"sealed_meta"`))
	if at < 4 {
		t.Fatalf("no footer found in the sealed record")
	}
	footer := raw[at:]
	if int(binary.BigEndian.Uint32(raw[at-4:at])) != len(footer) {
		t.Fatalf("the footer's length prefix does not match the trailing JSON, so the edit below would be malformed")
	}
	edited := bytes.Replace(footer, []byte(`"signed":true,`), []byte(`"signed":false,`), 1)
	if bytes.Equal(edited, footer) || !bytes.Contains(edited, []byte(`"signature":"`)) {
		t.Fatalf("could not flip signed to false while keeping the signature: %s", footer)
	}
	out := append([]byte{}, raw[:at-4]...)
	out = binary.BigEndian.AppendUint32(out, uint32(len(edited)))
	out = append(out, edited...)

	passed, detail := discloseSEC013RecordSignatureCheck(t, discloseSEC013WriteRecord(t, out))
	if passed {
		t.Fatalf("record_signature passed (%q) for a footer whose signed field is false while it keeps a "+
			"signature: the contradiction was resolved instead of refused", detail)
	}
	if strings.Contains(detail, "carries no signature") {
		t.Fatalf("record_signature says %q for a footer that carries a signature: the refusal is worded as "+
			"an unsigned record, and the first half of it is false", detail)
	}
	if !strings.Contains(detail, "the footer disagrees with itself: its signed field is false") {
		t.Fatalf("record_signature detail %q does not say the footer disagrees with itself", detail)
	}
}

// The ordinary unsigned record keeps its wording: it claims nothing, and
// disclose_verify says it carries no signature, followed by the footer's reason.
func TestDiscloseVerifySEC013PlainUnsignedRecordStillSaysNoSignature(t *testing.T) {
	useTestKeyStore(t)
	recordTestCase(t)
	source, dest, _ := recordFixture(t)
	sealed := mustHash(t, RecordSeal(stringObj(source), stringObj(dest), recordArray(),
		recordSealOpts(map[string]object.Object{"sign": boolObj(false)})))
	if mustHashBoolValue(t, sealed, "signed") {
		t.Fatal("a record sealed with sign:false reported signed")
	}
	raw, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	passed, detail := discloseSEC013RecordSignatureCheck(t, discloseSEC013WriteRecord(t, raw))
	if passed {
		t.Fatalf("record_signature passed (%q) for an unsigned record", detail)
	}
	if !strings.HasPrefix(detail, "the record carries no signature: ") {
		t.Fatalf("record_signature detail %q for an unsigned record lost its \"carries no signature\" prefix", detail)
	}
	if strings.Contains(detail, "the footer disagrees with itself: its signed field is false") {
		t.Fatalf("an ordinary unsigned record was refused as self-contradicting: %q", detail)
	}
}
