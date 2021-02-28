package security

import (
	"crypto/sha256"
	"strings"
)

func SignCode(encodedString string) []byte {
	var signedCode string
	integrity := sha256.New().Sum([]byte(encodedString))
	signedCode = (HEADER + SEPERATOR) + (encodedString + SEPERATOR) + (string(integrity) + SEPERATOR) + (FOOTER)
	return []byte(signedCode)
}

func VerifyCode(signedCode []byte) error {
	signedCodeString := string(signedCode)
	values := strings.Split(signedCodeString, SEPERATOR)

	if values[0] != HEADER {
		return ErrWrongSignature
	}

	integrity := sha256.New().Sum([]byte(values[1]))
	if string(integrity) != values[2] {
		return ErrWrongSignature
	}

	if values[3] != FOOTER {
		return ErrWrongSignature
	}

	return nil
}

func GetEncryptedCode(signedCode []byte) string {
	signedCodeString := string(signedCode)
	return strings.Split(signedCodeString, SEPERATOR)[1]
}
