//go:build !linux
// +build !linux

package security

// Stub implementations for non-Linux platforms

func isDebuggerPresentLinux() bool {
	return false
}

func detectDebuggerParentLinux() bool {
	return false
}

func CheckLDPreload() bool {
	return false
}
