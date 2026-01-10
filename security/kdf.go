package security

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/hkdf"
)

// KDFParams stores key derivation parameters
type KDFParams struct {
	Algorithm string // "argon2id" or "hkdf"
	Salt      []byte
	Time      uint32 // Argon2 only
	Memory    uint32 // Argon2 only (in KB)
	Threads   uint8  // Argon2 only
	KeyLen    uint32 // Output key length
	Info      []byte // HKDF only
}

const (
	// Argon2id recommended parameters (OWASP)
	DefaultArgon2Time    = 1
	DefaultArgon2Memory  = 64 * 1024 // 64 MB
	DefaultArgon2Threads = 4
	DefaultKeyLen        = 32 // 256 bits

	// HKDF info strings
	HKDFInfoBytecode  = "mutant-bytecode-encryption-v1"
	HKDFInfoSignature = "mutant-signature-key-v1"
)

// DeriveKeyFromPassword derives a key from password using Argon2id
// This is used when user provides a password for compilation/execution
func DeriveKeyFromPassword(password string, salt []byte) ([]byte, *KDFParams, error) {
	if len(password) == 0 {
		return nil, nil, errors.New("password cannot be empty")
	}

	if len(salt) == 0 {
		salt = make([]byte, 32)
		if _, err := io.ReadFull(rand.Reader, salt); err != nil {
			return nil, nil, err
		}
	}

	key := argon2.IDKey(
		[]byte(password),
		salt,
		DefaultArgon2Time,
		DefaultArgon2Memory,
		DefaultArgon2Threads,
		DefaultKeyLen,
	)

	params := &KDFParams{
		Algorithm: "argon2id",
		Salt:      salt,
		Time:      DefaultArgon2Time,
		Memory:    DefaultArgon2Memory,
		Threads:   DefaultArgon2Threads,
		KeyLen:    DefaultKeyLen,
	}

	return key, params, nil
}

// DeriveKeyDeterministic derives a key deterministically from source code hash
// This is used when no password is provided (automatic mode)
func DeriveKeyDeterministic(sourceHash []byte, metadata string) ([]byte, *KDFParams, error) {
	if len(sourceHash) == 0 {
		return nil, nil, errors.New("source hash cannot be empty")
	}

	// Use source hash as salt for determinism
	salt := sourceHash

	// Create HKDF with SHA-256
	info := []byte(HKDFInfoBytecode + "|" + metadata)

	hkdfReader := hkdf.New(sha256.New, sourceHash, salt, info)

	key := make([]byte, DefaultKeyLen)
	if _, err := io.ReadFull(hkdfReader, key); err != nil {
		return nil, nil, err
	}

	params := &KDFParams{
		Algorithm: "hkdf-sha256",
		Salt:      salt,
		KeyLen:    DefaultKeyLen,
		Info:      info,
	}

	return key, params, nil
}

// ReconstructKey reconstructs a key from password and stored parameters
func ReconstructKey(password string, params *KDFParams) ([]byte, error) {
	switch params.Algorithm {
	case "argon2id":
		return argon2.IDKey(
			[]byte(password),
			params.Salt,
			params.Time,
			params.Memory,
			params.Threads,
			params.KeyLen,
		), nil

	case "hkdf-sha256":
		// For HKDF, we need the original source hash
		// This should be derived from the salt which stores the source hash
		hkdfReader := hkdf.New(sha256.New, params.Salt, params.Salt, params.Info)
		key := make([]byte, params.KeyLen)
		if _, err := io.ReadFull(hkdfReader, key); err != nil {
			return nil, err
		}
		return key, nil

	default:
		return nil, fmt.Errorf("unknown KDF algorithm: %s", params.Algorithm)
	}
}

// HashSourceCode creates a SHA-256 hash of source code
func HashSourceCode(source []byte) []byte {
	hash := sha256.Sum256(source)
	return hash[:]
}

// GenerateMetadata creates metadata string for deterministic key derivation
func GenerateMetadata(filename, version string) string {
	// Use filename and version for additional entropy
	// This makes bytecode specific to project context
	return fmt.Sprintf("%s|%s", filename, version)
}

// GenerateSalt generates a cryptographically secure random salt
func GenerateSalt() ([]byte, error) {
	salt := make([]byte, 32) // 256 bits
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, err
	}
	return salt, nil
}

// EncodeParams serializes KDF parameters for storage
func (p *KDFParams) Encode() string {
	return fmt.Sprintf("%s|%s|%d|%d|%d|%d|%s",
		p.Algorithm,
		hex.EncodeToString(p.Salt),
		p.Time,
		p.Memory,
		p.Threads,
		p.KeyLen,
		hex.EncodeToString(p.Info),
	)
}

// DecodeParams deserializes KDF parameters
func DecodeParams(encoded string) (*KDFParams, error) {
	var algo, saltHex, infoHex string
	var time, memory uint32
	var threads uint8
	var keyLen uint32

	_, err := fmt.Sscanf(encoded, "%s|%s|%d|%d|%d|%d|%s",
		&algo, &saltHex, &time, &memory, &threads, &keyLen, &infoHex)
	if err != nil {
		return nil, err
	}

	salt, err := hex.DecodeString(saltHex)
	if err != nil {
		return nil, err
	}

	info, err := hex.DecodeString(infoHex)
	if err != nil {
		return nil, err
	}

	return &KDFParams{
		Algorithm: algo,
		Salt:      salt,
		Time:      time,
		Memory:    memory,
		Threads:   threads,
		KeyLen:    keyLen,
		Info:      info,
	}, nil
}
