package lua

import (
	"errors"
	"fmt"
	"mutant/mutil"
	"mutant/object"
	"mutant/security"
)

// PatchLoader handles loading and decrypting Lua patches from encrypted bytecode.
type PatchLoader struct {
	password string
	inslen   int64
}

// NewPatchLoader creates a new patch loader with the specified password and instruction length.
func NewPatchLoader(password string, inslen int64) *PatchLoader {
	return &PatchLoader{
		password: password,
		inslen:   inslen,
	}
}

// LoadPatch decrypts and validates a Lua patch from encrypted payload.
// The plaintext bytecode is loaded into an ephemeral SecureBuffer.
// Caller must invoke buffer.Close() to zero sensitive data.
func (pl *PatchLoader) LoadPatch(patch *object.LuaPatch) (*SecureBuffer, error) {
	if patch == nil {
		return nil, errors.New("patch is nil")
	}

	if len(patch.EncryptedPayload) == 0 {
		return nil, fmt.Errorf("patch %q has empty encrypted payload", patch.Name)
	}

	// Decrypt using Mutant's existing pipeline
	decrypted, err := mutil.DecryptLuaPatch(patch, int(pl.inslen), pl.password)
	if err != nil {
		security.RecordIntegrityFailure("lua-patch-decrypt")
		return nil, fmt.Errorf("failed to decrypt patch %q: %w", patch.Name, err)
	}

	// Verify integrity checksum
	computedChecksum := object.ComputeChecksum(decrypted)
	if computedChecksum != patch.ChecksumExpected {
		security.RecordIntegrityFailure("lua-patch-checksum")
		// Wipe decrypted data before returning error
		security.SecureZero(decrypted)
		return nil, fmt.Errorf("patch %q checksum mismatch: expected %s, got %s", patch.Name, patch.ChecksumExpected, computedChecksum)
	}

	// Store decrypted bytecode in ephemeral secure buffer
	buffer := &SecureBuffer{
		data: decrypted,
		size: int64(len(decrypted)),
	}

	return buffer, nil
}

// ValidatePatchMetadata checks that patch metadata is well-formed before decryption.
func ValidatePatchMetadata(patch *object.LuaPatch) error {
	if patch == nil {
		return errors.New("patch is nil")
	}

	if patch.Name == "" {
		return errors.New("patch name cannot be empty")
	}

	if patch.ChecksumExpected == "" {
		return errors.New("patch checksum cannot be empty")
	}

	if len(patch.ChecksumExpected) != 64 { // SHA-256 hex = 64 chars
		return fmt.Errorf("patch checksum invalid length: expected 64, got %d", len(patch.ChecksumExpected))
	}

	return nil
}
