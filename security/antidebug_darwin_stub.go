//go:build !darwin
// +build !darwin

package security

// Stub for non-Darwin platforms
func isDebuggerPresentDarwin() bool {
	return false
}

func detectDebuggerDetailsDarwin() (bool, []string) {
	return false, nil
}
