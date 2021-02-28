package security

import (
	"crypto/aes"
	"crypto/cipher"
	cryptoRand "crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	mathRand "math/rand"
	"strings"
)

func AESEncrypt(data []byte) (string, error) {
	key := sha256.New().Sum(data)[:32]
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

	cypher := gcm.Seal(nonce, nonce, data, []byte(ENCSIG))

	cipherString := base64.StdEncoding.EncodeToString(cypher)
	keyString := hex.EncodeToString(key)

	interim := cipherString + SEPERATOR + keyString
	finalCipher := base64.StdEncoding.EncodeToString([]byte(interim))

	return finalCipher, nil
}

func AESDecrypt(encodedCipherData string) ([]byte, error) {
	cipData, err := base64.StdEncoding.DecodeString(encodedCipherData)
	if err != nil {
		return nil, err
	}

	values := strings.Split(string(cipData), SEPERATOR)
	cipherString := values[0]
	keyString := values[1]

	cypher, err := base64.StdEncoding.DecodeString(cipherString)
	if err != nil {
		return nil, err
	}
	key, err := hex.DecodeString(keyString)
	if err != nil {
		return nil, err
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
	if len(cypher) < nonceSize {
		return nil, errors.New("wrong nonce")
	}

	nonce, cipherText := cypher[:nonceSize], cypher[nonceSize:]
	data, err := gcm.Open(nil, nonce, cipherText, []byte(ENCSIG))

	return data, nil
}

// XOR function performs xor on a byte array
func XOR(data []byte, length int) []byte {
	key := randByte(int64(length))
	for i := range data {
		data[i] ^= key
	}
	return data
}

// XOROne funciton performs xor on a single byte(instruction)
func XOROne(instruction byte, length int) byte {
	key := randByte(int64(length))
	return instruction ^ key
}

func randByte(seed int64) byte {
	src := mathRand.NewSource(seed)
	newrand := mathRand.New(src)
	number := newrand.Int()
	return byte(number)
}
