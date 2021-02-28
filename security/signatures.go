package security

import (
	"crypto/md5"
	"encoding/hex"
	"strings"
)

func SignCode(encodedString string) []byte {
	var signedCode string
	integrity := md5.New().Sum([]byte(encodedString))
	integString := hex.EncodeToString(integrity)
	signedCode = (HEADER + SEPERATOR) + (encodedString + SEPERATOR) + (integString + SEPERATOR) + (FOOTER)
	return []byte(signedCode)
}

func VerifyCode(signedCode []byte) error {
	signedCodeString := string(signedCode)
	values := strings.Split(signedCodeString, SEPERATOR)

	if values[0] != HEADER {
		return ErrWrongSignature
	}

	integrity := md5.New().Sum([]byte(values[1]))
	integString := hex.EncodeToString(integrity)
	if integString != values[2] {
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
