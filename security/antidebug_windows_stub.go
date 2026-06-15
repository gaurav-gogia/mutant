//go:build !windows
// +build !windows

package security

// Stubs for non-Windows platforms

func isDebuggerPresentWindows() bool {
	return false
}

func CheckRemoteDebugger() bool {
	return false
}
