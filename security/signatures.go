package security

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
)

// SignCode signs bytecode using Ed25519 digital signatures
// Format: HEADER|ENCODED_DATA|ED25519_SIGNATURE|PUBLIC_KEY|FOOTER
func SignCode(encodedString string, privateKey []byte) ([]byte, error) {
	// Sign the bytecode using Ed25519
	codeSignature, err := SignBytecode([]byte(encodedString), privateKey, "v2.1.0")
	if err != nil {
		return nil, fmt.Errorf("failed to sign bytecode: %w", err)
	}

	// Encode signature and public key
	signatureHex := hex.EncodeToString(codeSignature.Signature)
	publicKeyHex := hex.EncodeToString(codeSignature.PublicKey)

	// Format: HEADER|DATA|SIGNATURE|PUBKEY|FOOTER
	signedCode := fmt.Sprintf("%s%s%s%s%s%s%s%s%s",
		HEADER, OUTER_SEPERATOR,
		encodedString, OUTER_SEPERATOR,
		signatureHex, OUTER_SEPERATOR,
		publicKeyHex, OUTER_SEPERATOR,
		FOOTER)

	return []byte(signedCode), nil
} // VerifyCode verifies bytecode Ed25519 signature
func VerifyCode(signedCode []byte) error {
	signedCodeString := string(signedCode)
	parts := strings.Split(signedCodeString, OUTER_SEPERATOR)

	if len(parts) < 5 {
		return ErrWrongSignature
	}

	// Check header and footer
	if parts[0] != HEADER {
		return ErrWrongSignature
	}

	if parts[len(parts)-1] != FOOTER {
		return ErrWrongSignature
	}

	// Extract components
	encodedData := parts[1]
	signatureHex := parts[len(parts)-3]
	publicKeyHex := parts[len(parts)-2]

	// Decode signature and public key
	signatureBytes, err := hex.DecodeString(signatureHex)
	if err != nil {
		return fmt.Errorf("invalid signature encoding: %w", err)
	}

	publicKeyBytes, err := hex.DecodeString(publicKeyHex)
	if err != nil {
		return fmt.Errorf("invalid public key encoding: %w", err)
	}

	// Create CodeSignature struct for verification
	codeSignature := &CodeSignature{
		PublicKey: publicKeyBytes,
		Signature: signatureBytes,
		Algorithm: "Ed25519",
		Version:   "v2.1.0",
	}

	// Verify signature
	if err := VerifyBytecode([]byte(encodedData), codeSignature); err != nil {
		return fmt.Errorf("signature verification failed: %w", err)
	}

	return nil
}

// GetEncryptedCode extracts the encrypted bytecode from signed code
func GetEncryptedCode(signedCode []byte) string {
	signedCodeString := string(signedCode)
	parts := strings.Split(signedCodeString, OUTER_SEPERATOR)
	if len(parts) < 2 {
		return ""
	}
	return parts[1]
}

// GenerateSigningKeys generates a new Ed25519 key pair
// Returns (privateKey, publicKey, error)
func GenerateSigningKeys() ([]byte, []byte, error) {
	keyPair, err := GenerateKeyPair()
	if err != nil {
		return nil, nil, err
	}
	return keyPair.PrivateKey, keyPair.PublicKey, nil
}

// SaveSigningKey saves a signing key to disk (for development/build process)
func SaveSigningKey(privateKey []byte, path string) error {
	_ = base64.StdEncoding.EncodeToString(privateKey)
	// In production, you'd want to encrypt this key
	// For now, just encode it
	return nil // Implement file writing if needed
}

// LoadSigningKey loads a signing key from disk (for development/build process)
func LoadSigningKey(path string) ([]byte, error) {
	// Implement file reading if needed
	return nil, nil
}
