package security

import (
	"bytes"
	"crypto/sha256"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// M26-SEC-013. VerifyRecordSignature used to derive its signed bit from whether
// a signature string was present: the unsigned branch needed Signed to be false
// AND Signature to be blank, so a footer that said "signed": false and carried a
// signature anyway was reported signed -- and valid, when the signature held --
// whatever its own Signed field said. SignRecordFooter was the other half: its
// two unsigned paths set Signed = false and left the previous call's signer,
// signature and key-age bit in place, so re-sealing a signed footer built
// exactly that self-contradicting document.
//
// These tests drive only exported functions and the keystore testing override,
// so they compile against the pre-fix record_file.go as well as the fixed one.

// sec013SignedFooter seals a footer under a keystore of its own.
func sec013SignedFooter(t *testing.T) (*RecordFooter, []byte, [sha256.Size]byte) {
	t.Helper()
	useKeyStore(t, t.TempDir())
	header := []byte("a header stands in for the record this footer closes")
	root := sha256.Sum256([]byte("the segments root"))
	footer := &RecordFooter{}
	if err := SignRecordFooter(footer, header, root, true); err != nil {
		t.Fatalf("SignRecordFooter: %v", err)
	}
	if !footer.Signed || footer.Signature == "" || footer.PublicKey == "" {
		t.Fatalf("signed = %v, reason %q: these tests need a footer that is signed",
			footer.Signed, footer.SignatureReason)
	}
	if signed, valid, detail := VerifyRecordSignature(footer, header, root); !signed || !valid {
		t.Fatalf("the freshly signed footer does not verify: signed=%v valid=%v %q", signed, valid, detail)
	}
	return footer, header, root
}

func sec013WantRefused(t *testing.T, what string, footer *RecordFooter, header []byte, root [sha256.Size]byte) {
	t.Helper()
	signed, valid, detail := VerifyRecordSignature(footer, header, root)
	if signed || valid {
		t.Fatalf("%s: VerifyRecordSignature reported signed=%v valid=%v (%q) for a footer whose own signed "+
			"field is false: the bit was taken from a sibling field and not from Signed", what, signed, valid, detail)
	}
	if !strings.Contains(detail, "the footer disagrees with itself: its signed field is false") {
		t.Fatalf("%s: detail %q does not say the footer disagrees with itself, so the refusal reads as an "+
			"ordinary unsigned record", what, detail)
	}
}

// The verifier half, through the bytes a hand-edited .mrec footer would hold:
// "signed": false beside a real signature is refused, not reported signed.
func TestRecordFooterSEC013UnsignedFooterCarryingSignatureIsRefused(t *testing.T) {
	footer, header, root := sec013SignedFooter(t)
	data, err := MarshalRecordFooter(footer)
	if err != nil {
		t.Fatalf("MarshalRecordFooter: %v", err)
	}
	edited := bytes.Replace(data, []byte(`"signed":true`), []byte(`"signed":false`), 1)
	if bytes.Equal(edited, data) {
		t.Fatalf("the marshalled footer has no \"signed\":true to edit: %s", data)
	}
	parsed, err := ParseRecordFooter(edited)
	if err != nil {
		t.Fatalf("ParseRecordFooter on the edited footer: %v", err)
	}
	if parsed.Signed || parsed.Signature == "" {
		t.Fatalf("the edit did not produce the self-contradicting footer: signed=%v signature=%q",
			parsed.Signed, parsed.Signature)
	}
	sec013WantRefused(t, "hand-edited footer", parsed, header, root)
}

// Each signature-bearing field alone is enough to make an unsigned footer
// contradict itself.
func TestRecordFooterSEC013EachSignatureFieldAloneIsRefused(t *testing.T) {
	signedFooter, header, root := sec013SignedFooter(t)
	cases := []struct {
		name string
		edit func(f *RecordFooter)
	}{
		{"signature only", func(f *RecordFooter) { f.PublicKey = ""; f.KeyCreatedForThisRun = false }},
		{"public key only", func(f *RecordFooter) { f.Signature = ""; f.KeyCreatedForThisRun = false }},
		{"key created for this run only", func(f *RecordFooter) {
			f.Signature = ""
			f.PublicKey = ""
			f.KeyCreatedForThisRun = true
		}},
	}
	for _, c := range cases {
		f := *signedFooter
		f.Signed = false
		f.SignatureReason = "the record was sealed with sign:false"
		c.edit(&f)
		sec013WantRefused(t, c.name, &f, header, root)
	}

	// A footer that is unsigned and says nothing else is still the ordinary
	// unsigned record, reported with its own reason.
	plain := RecordFooter{SegmentsRoot: signedFooter.SegmentsRoot, SignatureReason: "no key store here"}
	signed, valid, detail := VerifyRecordSignature(&plain, header, root)
	if signed || valid || detail != "no key store here" {
		t.Fatalf("a plain unsigned footer reports signed=%v valid=%v detail=%q", signed, valid, detail)
	}
}

// The producer half: re-sealing a signed footer with sign:false clears every
// field a signature brings, so the footer it leaves is an ordinary unsigned one.
func TestRecordFooterSEC013ResealWithoutSigningClearsTheSignature(t *testing.T) {
	footer, header, root := sec013SignedFooter(t)
	footer.KeyCreatedForThisRun = true // as if the key had been born for the first seal
	if err := SignRecordFooter(footer, header, root, false); err != nil {
		t.Fatalf("SignRecordFooter with sign=false: %v", err)
	}
	if footer.Signed {
		t.Fatal("signed = true after sealing with sign:false")
	}
	if footer.Signature != "" || footer.PublicKey != "" || footer.KeyCreatedForThisRun {
		t.Fatalf("re-sealing with sign:false left signature %q, public key %q and key_created_for_this_run=%v "+
			"beside signed=false", footer.Signature, footer.PublicKey, footer.KeyCreatedForThisRun)
	}
	signed, valid, detail := VerifyRecordSignature(footer, header, root)
	if signed || valid || detail != footer.SignatureReason {
		t.Fatalf("the re-sealed footer reports signed=%v valid=%v detail=%q, want the unsigned reason %q",
			signed, valid, detail, footer.SignatureReason)
	}

	// And back again: signing the same footer drops the stale reason.
	if err := SignRecordFooter(footer, header, root, true); err != nil {
		t.Fatalf("SignRecordFooter with sign=true: %v", err)
	}
	if !footer.Signed || footer.SignatureReason != "" {
		t.Fatalf("re-signing left signed=%v and the unsigned reason %q on a signed footer",
			footer.Signed, footer.SignatureReason)
	}
	if signed, valid, detail := VerifyRecordSignature(footer, header, root); !signed || !valid {
		t.Fatalf("the re-signed footer does not verify: signed=%v valid=%v %q", signed, valid, detail)
	}
}

// The keystore-error path is the other unsigned exit and clears the same way.
func TestRecordFooterSEC013KeystoreErrorClearsTheSignature(t *testing.T) {
	footer, header, root := sec013SignedFooter(t)
	// A directory that is a regular file cannot hold a keystore, so the next
	// EnsureLocalSigningKeyPair fails.
	blocker := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blocker, []byte("a file where the keystore directory should be"), 0600); err != nil {
		t.Fatalf("creating the blocking file: %v", err)
	}
	useKeyStore(t, blocker)
	if err := SignRecordFooter(footer, header, root, true); err != nil {
		t.Fatalf("SignRecordFooter returned %v: a keystore error is reported on the footer", err)
	}
	if footer.Signed {
		t.Fatalf("signed = true with a keystore that cannot exist at %s", blocker)
	}
	if footer.Signature != "" || footer.PublicKey != "" || footer.KeyCreatedForThisRun {
		t.Fatalf("a keystore error left signature %q, public key %q and key_created_for_this_run=%v beside "+
			"signed=false", footer.Signature, footer.PublicKey, footer.KeyCreatedForThisRun)
	}
	if footer.SignatureReason == "" {
		t.Fatal("the keystore error left no reason on the unsigned footer")
	}
}
