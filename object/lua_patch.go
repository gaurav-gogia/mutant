package object

import (
	"bytes"
	"crypto/sha256"
	"fmt"
)

type LuaPatch struct {
	Name              string // e.g., "mitigation_buffer_overflow"
	EncryptedPayload  []byte // Lua bytecode, encrypted
	ChecksumExpected  string // SHA-256 of decrypted bytecode (for integrity check)
	DecryptedChecksum string // Set after decryption for validation
}

func (lp *LuaPatch) Type() ObjectType {
	return LUA_PATCH_OBJ
}

func (lp *LuaPatch) Inspect() string {
	var out bytes.Buffer
	out.WriteString("LuaPatch{")
	out.WriteString("name: ")
	out.WriteString(lp.Name)
	out.WriteString(", encrypted_size: ")
	out.WriteString(fmt.Sprintf("%d", len(lp.EncryptedPayload)))
	out.WriteString("}")
	return out.String()
}

// ComputeChecksum computes SHA-256 of plaintext bytecode
func ComputeChecksum(plaintext []byte) string {
	h := sha256.Sum256(plaintext)
	return fmt.Sprintf("%x", h[:])
}
