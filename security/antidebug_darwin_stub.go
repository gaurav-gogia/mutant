//go:build !darwin
// +build !darwin

package security

// Stub implementations for non-Darwin platforms

func isDebuggerPresentDarwin() bool {
	return false
}

func detectDebuggerParentDarwin() bool {
	return false
}

func CheckLLDBEnvironment() bool {
	return false
}

func CheckDyldInsert() bool {
	return false
}
