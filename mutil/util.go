package mutil

import (
	"crypto/sha512"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"mutant/compiler"
	"mutant/global"
	"mutant/object"
	"mutant/security"
	"strconv"
	"strings"

	"golang.org/x/crypto/hkdf"
)

func EncryptByteCode(byteCode *compiler.ByteCode, password string) *compiler.ByteCode {
	insLen := len(byteCode.Instructions)

	// If no password provided, derive one from instruction hash for deterministic encryption
	if password == "" {
		derivingPassword := security.DerivePasswordFromInstructions(byteCode.Instructions)
		password = string(rune(derivingPassword)) // Convert to string for consistent handling
	}

	xored, err := security.SecureXOR(byteCode.Instructions, int64(insLen), password)
	if err == nil {
		byteCode.Instructions = xored
	}

	for i := range byteCode.Constants {
		if byteCode.Constants[i].Type() == object.COMPILED_FN_OBJ {
			ins := byteCode.Constants[i].(*object.CompiledFunction).Instructions
			xored, err := security.SecureXOR(ins, int64(insLen), password)
			if err == nil {
				byteCode.Constants[i].(*object.CompiledFunction).Instructions = xored
			}
			continue
		}

		if encConst, err := EncryptObject(byteCode.Constants[i], insLen, password); err == nil {
			byteCode.Constants[i] = encConst
		}
	}

	return byteCode
}

func EncryptObject(obj object.Object, length int, password string) (object.Object, error) {
	var encObj object.Object
	var err error

	switch obj.Type() {
	case object.INTEGER_OBJ:
		val := obj.(*object.Integer).Value
		bite := make([]byte, 8)
		binary.LittleEndian.PutUint64(bite, uint64(val))
		xored, err := security.SecureXOR(bite, int64(length), password)
		if err != nil {
			return nil, err
		}

		encObj = &object.Encrypted{
			EncType: object.INTEGER_OBJ,
			Value:   xored,
		}

	case object.STRING_OBJ:
		val := obj.(*object.String).Value
		xored, err := security.SecureXOR([]byte(val), int64(length), password)
		if err != nil {
			return nil, err
		}

		encObj = &object.Encrypted{
			EncType: object.STRING_OBJ,
			Value:   xored,
		}

	case object.BOOLEAN_OBJ:
		val := obj.(*object.Boolean).Value
		str := strconv.FormatBool(val)
		xored, err := security.SecureXOR([]byte(str), int64(length), password)
		if err != nil {
			return nil, err
		}

		encObj = &object.Encrypted{
			EncType: object.BOOLEAN_OBJ,
			Value:   xored,
		}

	default:
		err = errors.New("wrong obj type")
	}

	return encObj, err
}

func DecryptObject(obj object.Object, length int, password string) (object.Object, error) {
	decObj := obj
	var err error

	if decObj.Type() == object.ENCRYPTED_OBJ {
		biteVal := decObj.(*object.Encrypted).Value
		bite := make([]byte, len(biteVal))
		copy(bite, biteVal)
		xored, err := security.SecureXOR(bite, int64(length), password)
		if err != nil {
			return nil, err
		}

		switch decObj.(*object.Encrypted).EncType {
		case object.INTEGER_OBJ:
			val := binary.LittleEndian.Uint64(xored)
			decObj = &object.Integer{Value: int64(val)}

		case object.STRING_OBJ:
			decObj = &object.String{Value: string(xored)}

		case object.BOOLEAN_OBJ:
			str := strings.ToLower(string(xored))
			if str == "true" {
				decObj = global.True
			} else {
				decObj = global.False
			}
		}

		return decObj, nil
	}

	err = errors.New("wrong obj type")
	return obj, err
}

func GetPwd() string {
	// Use HKDF (HMAC-based Key Derivation Function) - RFC 5869
	// This generates a deterministic password from fixed context

	// Master secret - fixed entropy source
	masterSecret := []byte("mutant-lang-security-kdf-v1-deterministic-key")

	// Context info - includes program name and version for uniqueness
	contextInfo := []byte("mutant-instruction-encryption-key")

	// Salt - deterministic and fixed (could include compile-time info for variation)
	salt := []byte("mutant-hkdf-salt-v1")

	// Create HKDF reader with SHA-256
	// SHA-256 produces 32-byte hash, we'll use first 24 bytes as password (192 bits of entropy)
	hkdfReader := hkdf.New(sha512.New, masterSecret, salt, contextInfo)

	// Generate 24 bytes for a strong password (192 bits)
	derivedKey := make([]byte, 64)
	_, err := hkdfReader.Read(derivedKey)
	if err != nil {
		// Fallback to hardcoded password if HKDF fails (should never happen in practice)
		return "mutant-default-security-key-v1"
	}

	// Convert to hex string for consistent string representation
	// 24 bytes = 48 hex characters
	return hex.EncodeToString(derivedKey)
}
