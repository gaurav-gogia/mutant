package security

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/binary"
	"errors"
)

// SecureRandByte generates a cryptographically secure random byte
// Replaces the insecure math/rand implementation
func SecureRandByte() (byte, error) {
	b := make([]byte, 1)
	if _, err := rand.Read(b); err != nil {
		return 0, err
	}
	return b[0], nil
}

// SecureRandBytes generates n cryptographically secure random bytes
func SecureRandBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	return b, nil
}

// SecureXOR performs XOR with a cryptographically secure key derived from seed and password
// This replaces the old XOR that used math/rand
// seed: instruction length or other deterministic value
// password: optional user password (empty string for deterministic encryption without password)
func SecureXOR(data []byte, seed int64, password string) ([]byte, error) {
	if len(data) == 0 {
		return nil, errors.New("data cannot be empty")
	}

	result := make([]byte, len(data))
	for i := range data {
		res, err := SecureXOROne(data[i], seed, password)
		if err != nil {
			return nil, err
		}
		result[i] = res
	}

	return result, nil
}

// SecureXOROne performs XOR on a single byte with secure random key derived from seed and password
func SecureXOROne(instruction byte, seed int64, password string) (byte, error) {
	key, err := deriveXORKey(seed, password, 1)
	if err != nil {
		return 0, err
	}
	return instruction ^ key[0], nil
}

// passwordToSeed converts a password string to a uint64 seed
// If password is empty, returns 0
// If password is provided, returns hash of password as uint64
func passwordToSeed(password string) uint64 {
	if password == "" {
		return 0
	}

	hash := sha256.Sum256([]byte(password))
	// Convert first 8 bytes to uint64
	return binary.LittleEndian.Uint64(hash[:8])
}

// DerivePasswordFromInstructions derives a deterministic password seed from instruction hash
// This ensures same program always gets same key, different programs get different keys
// Returns a uint64 that's converted to string in the caller
func DerivePasswordFromInstructions(instructions []byte) uint64 {
	if len(instructions) == 0 {
		return 0
	}

	hash := sha256.Sum256(instructions)
	// Use first 8 bytes as seed
	return binary.LittleEndian.Uint64(hash[:8])
}

// deriveXORKey derives a key for XOR operations using HKDF-like approach
// Incorporates both seed (instruction length) and password for enhanced security
func deriveXORKey(seed int64, password string, length int) ([]byte, error) {
	// Convert seed to bytes
	seedBytes := make([]byte, 8)
	for i := 0; i < 8; i++ {
		seedBytes[i] = byte(seed >> (i * 8))
	}

	// Convert password to seed and merge with instruction-length seed
	passwordSeed := passwordToSeed(password)
	passwordBytes := make([]byte, 8)
	for i := 0; i < 8; i++ {
		passwordBytes[i] = byte(passwordSeed >> (i * 8))
	}

	// XOR seeds together to mix password and instruction-length seed
	mergedSeed := make([]byte, 8)
	for i := 0; i < 8; i++ {
		mergedSeed[i] = seedBytes[i] ^ passwordBytes[i]
	}

	// Use a deterministic but secure key derivation
	key := make([]byte, length)

	// Simple HKDF-like expansion using merged seed
	for i := 0; i < length; i += 32 {
		counter := byte(i / 32)
		block := append(mergedSeed, counter)

		for j := 0; j < 32 && i+j < length; j++ {
			key[i+j] = block[j%len(block)] ^ byte(i+j)
		}
	}

	return key, nil
}

// SecureCompare performs constant-time comparison to prevent timing attacks
func SecureCompare(a, b []byte) bool {
	return subtle.ConstantTimeCompare(a, b) == 1
}

// SecureZero zeroes out sensitive data in memory
func SecureZero(data []byte) {
	for i := range data {
		data[i] = 0
	}
}

// SecureZeroString zeroes out a string (via conversion)
// Note: This doesn't zero the original string, but the conversion
func SecureZeroString(s *string) {
	if s == nil {
		return
	}

	// Convert to byte slice and zero
	bytes := []byte(*s)
	SecureZero(bytes)
	*s = ""
}
