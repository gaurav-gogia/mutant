package security

import (
	"crypto/aes"
	"crypto/cipher"
	cryptoRand "crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
)

// EncryptionMetadata stores all information needed to decrypt data
// WITHOUT storing the actual encryption key
type EncryptionMetadata struct {
	Ciphertext     string // Base64 encoded ciphertext with nonce
	Salt           string // Hex encoded salt for KDF
	SourceCodeHash string // Hex encoded hash of source code (for deterministic KDF)
	UsePasswordKDF bool   // True if password-based, false if deterministic
	IterationCount uint32 // Argon2id iterations (if UsePasswordKDF=true)
	Memory         uint32 // Argon2id memory in KB (if UsePasswordKDF=true)
	Parallelism    uint8  // Argon2id parallelism (if UsePasswordKDF=true)
}

// AESEncrypt encrypts data using password-based key derivation
// NEVER stores the encryption key - only stores KDF parameters
func AESEncrypt(data []byte, password string) (string, error) {
	// Generate a secure random salt
	salt, err := GenerateSalt()
	if err != nil {
		return "", fmt.Errorf("failed to generate salt: %w", err)
	}

	// Derive key using Argon2id (uses OWASP recommended parameters)
	key, kdfParams, err := DeriveKeyFromPassword(password, salt)
	if err != nil {
		return "", fmt.Errorf("failed to derive key from password: %w", err)
	}
	defer SecureZero(key) // Zero the key from memory when done	// Encrypt the data
	c, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(c)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(cryptoRand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := gcm.Seal(nonce, nonce, data, []byte(ENCSIG))

	// Create metadata (WITHOUT storing the key or password)
	metadata := EncryptionMetadata{
		Ciphertext:     base64.StdEncoding.EncodeToString(ciphertext),
		Salt:           hex.EncodeToString(salt),
		UsePasswordKDF: true,
		IterationCount: kdfParams.Time,
		Memory:         kdfParams.Memory,
		Parallelism:    kdfParams.Threads,
	}

	// Serialize metadata
	return serializeMetadata(metadata), nil
}

// AESDecrypt decrypts data using password-based key derivation
// Reconstructs the key from password and metadata - key is NEVER stored
func AESDecrypt(encodedMetadata string, password string) ([]byte, error) {
	// Deserialize metadata
	metadata, err := deserializeMetadata(encodedMetadata)
	if err != nil {
		return nil, err
	}

	if !metadata.UsePasswordKDF {
		return nil, errors.New("this data was encrypted deterministically, use AESDecrypt instead")
	}

	// Reconstruct the key using password and stored KDF parameters
	salt, err := hex.DecodeString(metadata.Salt)
	if err != nil {
		return nil, fmt.Errorf("invalid salt: %w", err)
	}

	key, _, err := DeriveKeyFromPassword(password, salt)
	if err != nil {
		return nil, fmt.Errorf("failed to derive key from password: %w", err)
	}
	defer SecureZero(key) // Zero the key from memory when done	// Decrypt the data
	return decryptWithKey(key, metadata.Ciphertext)
}

// decryptWithKey performs the actual AES-GCM decryption
func decryptWithKey(key []byte, ciphertextB64 string) ([]byte, error) {
	ciphertext, err := base64.StdEncoding.DecodeString(ciphertextB64)
	if err != nil {
		return nil, fmt.Errorf("invalid ciphertext encoding: %w", err)
	}

	c, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(c)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, errors.New("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	data, err := gcm.Open(nil, nonce, ciphertext, []byte(ENCSIG))
	if err != nil {
		return nil, fmt.Errorf("decryption failed: %w", err)
	}

	return data, nil
}

// serializeMetadata converts metadata to string format
func serializeMetadata(m EncryptionMetadata) string {
	if m.UsePasswordKDF {
		return fmt.Sprintf("%s%s%s%s%s%strue%s%d%s%d%s%d",
			m.Ciphertext, SEPERATOR,
			m.Salt, SEPERATOR,
			"", SEPERATOR, // Empty source hash for password-based
			SEPERATOR,
			m.IterationCount, SEPERATOR,
			m.Memory, SEPERATOR,
			m.Parallelism)
	}

	return fmt.Sprintf("%s%s%s%s%s%sfalse",
		m.Ciphertext, SEPERATOR,
		m.Salt, SEPERATOR,
		m.SourceCodeHash, SEPERATOR)
}

// deserializeMetadata parses metadata from string format
func deserializeMetadata(encoded string) (EncryptionMetadata, error) {
	parts := strings.Split(encoded, SEPERATOR)

	if len(parts) < 4 {
		return EncryptionMetadata{}, errors.New("invalid metadata format")
	}

	metadata := EncryptionMetadata{
		Ciphertext: parts[0],
		Salt:       parts[1],
	}

	if parts[3] == "true" {
		// Password-based encryption
		if len(parts) < 7 {
			return EncryptionMetadata{}, errors.New("invalid password-based metadata format")
		}

		metadata.UsePasswordKDF = true
		fmt.Sscanf(parts[4], "%d", &metadata.IterationCount)
		fmt.Sscanf(parts[5], "%d", &metadata.Memory)
		fmt.Sscanf(parts[6], "%d", &metadata.Parallelism)
	} else {
		// Deterministic encryption
		metadata.UsePasswordKDF = false
		metadata.SourceCodeHash = parts[2]
	}

	return metadata, nil
}

// SecureXOREncrypt performs XOR encryption with embedded key
// The key is embedded in the output, making it self-contained
// This is less secure than the old approach but maintains compatibility
func SecureXOREncrypt(data []byte) ([]byte, error) {
	// Generate secure random key
	key, err := SecureRandBytes(len(data))
	if err != nil {
		return nil, err
	}

	// XOR the data
	xored := make([]byte, len(data))
	for i := range data {
		xored[i] = data[i] ^ key[i]
	}

	// Prepend key length and key to the xored data
	// Format: [keyLen(4 bytes)][key][xored data]
	result := make([]byte, 4+len(key)+len(xored))
	result[0] = byte(len(key) >> 24)
	result[1] = byte(len(key) >> 16)
	result[2] = byte(len(key) >> 8)
	result[3] = byte(len(key))
	copy(result[4:4+len(key)], key)
	copy(result[4+len(key):], xored)

	return result, nil
}

// SecureXORDecrypt performs XOR decryption with embedded key
func SecureXORDecrypt(data []byte) ([]byte, error) {
	if len(data) < 4 {
		return nil, errors.New("invalid XOR encrypted data: too short")
	}

	// Extract key length
	keyLen := int(data[0])<<24 | int(data[1])<<16 | int(data[2])<<8 | int(data[3])

	if len(data) < 4+keyLen {
		return nil, errors.New("invalid XOR encrypted data: corrupted key")
	}

	// Extract key and xored data
	key := data[4 : 4+keyLen]
	xored := data[4+keyLen:]

	// XOR to decrypt
	result := make([]byte, len(xored))
	for i := range xored {
		result[i] = xored[i] ^ key[i]
	}

	return result, nil
}
