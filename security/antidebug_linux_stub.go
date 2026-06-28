//go:build !linux
// +build !linux

package security

// Stub for non-Linux platforms
func isDebuggerPresentLinux() bool {
	return false
}

func detectDebuggerDetailsLinux() (bool, []string) {
	return false, nil
}
