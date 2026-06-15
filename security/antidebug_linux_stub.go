//go:build !linux
// +build !linux

package security

// Stub for non-Linux platforms
func isDebuggerPresentLinux() bool {
	return false
}
