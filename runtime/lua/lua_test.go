package lua

import (
	"testing"

	"mutant/mutil"
	"mutant/object"
	"mutant/security"
)

func TestPatchLoaderDecryptsPatchCorrectly(t *testing.T) {
	// Create plaintext Lua bytecode
	plaintext := []byte{0x1b, 0x4c, 0x75, 0x61, 0x53, 0x00, 0x19}
	expectedChecksum := object.ComputeChecksum(plaintext)

	// Encrypt using SecureXOR
	password := "test-password"
	inslen := 256
	encrypted, err := security.SecureXOR(plaintext, int64(inslen), password)
	if err != nil {
		t.Fatalf("failed to encrypt: %v", err)
	}

	// Create a patch with encrypted payload
	patch := &object.LuaPatch{
		Name:             "test_patch",
		EncryptedPayload: encrypted,
		ChecksumExpected: expectedChecksum,
	}

	// Load patch using PatchLoader
	loader := NewPatchLoader(password, int64(inslen))
	buffer, err := loader.LoadPatch(patch)
	if err != nil {
		t.Fatalf("failed to load patch: %v", err)
	}
	defer buffer.Close()

	// Verify decrypted content matches plaintext
	decrypted := buffer.Bytes()
	if len(decrypted) != len(plaintext) {
		t.Errorf("decrypted length mismatch: got %d, want %d", len(decrypted), len(plaintext))
	}
	for i, b := range decrypted {
		if b != plaintext[i] {
			t.Errorf("decrypted byte mismatch at index %d: got %02x, want %02x", i, b, plaintext[i])
		}
	}

	// Verify buffer size
	if buffer.Size() != int64(len(plaintext)) {
		t.Errorf("buffer size mismatch: got %d, want %d", buffer.Size(), len(plaintext))
	}
}

func TestPatchLoaderDetectsChecksumMismatch(t *testing.T) {
	plaintext := []byte{0x1b, 0x4c, 0x75, 0x61}
	wrongChecksum := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

	password := "test-password"
	inslen := 256
	encrypted, _ := security.SecureXOR(plaintext, int64(inslen), password)

	patch := &object.LuaPatch{
		Name:             "test_patch",
		EncryptedPayload: encrypted,
		ChecksumExpected: wrongChecksum,
	}

	loader := NewPatchLoader(password, int64(inslen))
	buffer, err := loader.LoadPatch(patch)

	if err == nil {
		t.Fatal("expected checksum mismatch error, got nil")
	}
	if buffer != nil {
		buffer.Close()
	}
}

func TestValidatePatchMetadataRejectsNil(t *testing.T) {
	err := ValidatePatchMetadata(nil)
	if err == nil {
		t.Fatal("expected error for nil patch, got nil")
	}
}

func TestValidatePatchMetadataRejectsEmptyName(t *testing.T) {
	patch := &object.LuaPatch{
		Name:             "",
		ChecksumExpected: "a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0",
	}
	err := ValidatePatchMetadata(patch)
	if err == nil {
		t.Fatal("expected error for empty name, got nil")
	}
}

func TestSecureBufferZerosDataOnClose(t *testing.T) {
	data := []byte{0x01, 0x02, 0x03, 0x04}
	buffer := &SecureBuffer{data: data, size: 4}

	// Get reference to original slice
	originalData := buffer.Bytes()
	if len(originalData) != 4 {
		t.Fatalf("initial data length incorrect: %d", len(originalData))
	}

	// Close buffer (should zero data)
	buffer.Close()

	// Verify Bytes() returns nil after close
	if buffer.Bytes() != nil {
		t.Error("expected nil bytes after close, got non-nil")
	}

	// Verify Size is zero
	if buffer.Size() != 0 {
		t.Errorf("expected size 0 after close, got %d", buffer.Size())
	}
}

func TestPatchEncryptionRoundTrip(t *testing.T) {
	// Create a patch
	plainPayload := []byte{0x1b, 0x4c, 0x75, 0x61, 0x53, 0x00}
	checksum := object.ComputeChecksum(plainPayload)

	password := "round-trip-test"
	inslen := 512

	encrypted, _ := security.SecureXOR(plainPayload, int64(inslen), password)

	patch := &object.LuaPatch{
		Name:             "round_trip_patch",
		EncryptedPayload: encrypted,
		ChecksumExpected: checksum,
	}

	// Encrypt the patch using mutil.EncryptObject (simulating at-rest encryption)
	encryptedPatch, err := mutil.EncryptObject(patch, inslen, password)
	if err != nil {
		t.Fatalf("failed to encrypt patch: %v", err)
	}

	if encryptedPatch.Type() != object.LUA_PATCH_OBJ {
		t.Fatalf("expected LUA_PATCH_OBJ type, got %s", encryptedPatch.Type())
	}

	// Decrypt using mutil.DecryptObject
	decryptedPatch, err := mutil.DecryptObject(encryptedPatch, inslen, password)
	if err != nil {
		t.Fatalf("failed to decrypt patch: %v", err)
	}

	recovered := decryptedPatch.(*object.LuaPatch)
	if recovered.Name != patch.Name {
		t.Errorf("name mismatch: got %s, want %s", recovered.Name, patch.Name)
	}
	if recovered.ChecksumExpected != patch.ChecksumExpected {
		t.Errorf("checksum mismatch: got %s, want %s", recovered.ChecksumExpected, patch.ChecksumExpected)
	}
}
