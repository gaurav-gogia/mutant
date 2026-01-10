//go:build !windows
// +build !windows

package security

// Stub implementations for non-Windows platforms

func isDebuggerPresentWindows() bool {
	return false
}

func detectDebuggerParentWindows() bool {
	return false
}

func CheckRemoteDebugger() bool {
	return false
}

func IsBeingDebugged() bool {
	return false
}
