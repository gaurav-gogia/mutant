package lua

import (
	"mutant/security"
)

// SecureBuffer wraps plaintext Lua bytecode in an ephemeral in-memory buffer.
// The buffer is only accessible during patch execution and must be explicitly closed.
// Close() securely zeros the plaintext, making it unrecoverable.
type SecureBuffer struct {
	data []byte
	size int64
}

// Bytes returns the plaintext bytecode. Valid only until Close() is called.
func (sb *SecureBuffer) Bytes() []byte {
	if sb == nil || sb.data == nil {
		return nil
	}
	return sb.data
}

// Size returns the size of the plaintext bytecode.
func (sb *SecureBuffer) Size() int64 {
	if sb == nil {
		return 0
	}
	return sb.size
}

// Close securely zeros the plaintext bytecode and marks the buffer as invalid.
// After Close(), Bytes() returns nil. Close() is idempotent.
func (sb *SecureBuffer) Close() {
	if sb == nil || sb.data == nil {
		return
	}
	security.SecureZero(sb.data)
	sb.data = nil
	sb.size = 0
}
