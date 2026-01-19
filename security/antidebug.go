package security

import "runtime"

// IsDebuggerPresent checks if a debugger is currently attached to the process.
// Returns true if a debugger is detected, false otherwise.
// This uses platform-specific detection methods.
func IsDebuggerPresent() bool {
	switch runtime.GOOS {
	case "windows":
		return isDebuggerPresentWindows()
	case "linux":
		return isDebuggerPresentLinux()
	case "darwin":
		return isDebuggerPresentDarwin()
	default:
		return false
	}
}
