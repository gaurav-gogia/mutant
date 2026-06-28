package security

import (
	"runtime"
	"strings"
)

// IsDebuggerPresent checks if a debugger is currently attached to the process.
// It uses multiple platform-specific detection techniques:
//
// Windows:
//   - IsDebuggerPresent API (checks PEB BeingDebugged flag)
//   - CheckRemoteDebuggerPresent API
//   - NtQueryInformationProcess with ProcessDebugPort
//   - NtQueryInformationProcess with ProcessDebugObjectHandle
//   - OutputDebugString timing test
//   - Parent process name analysis
//   - Common debugger DLL detection
//
// Linux:
//   - TracerPid from /proc/self/status
//   - ptrace self-attachment test
//   - Parent process name analysis
//   - LD_PRELOAD detection
//
// macOS:
//   - sysctl P_TRACED flag
//   - LLDB environment variables
//   - DYLD_INSERT_LIBRARIES detection
//
// Returns true if any debugger detection method triggers.
func IsDebuggerPresent() bool {
	detected, methods := DetectDebuggerDetails()
	securityDevLogf("debugger detected=%t methods=%s", detected, strings.Join(methods, ","))
	return detected
}

// DetectDebuggerDetails returns debugger detection status plus triggered methods.
func DetectDebuggerDetails() (bool, []string) {
	switch runtime.GOOS {
	case "windows":
		return detectDebuggerDetailsWindows()
	case "linux":
		return detectDebuggerDetailsLinux()
	case "darwin":
		return detectDebuggerDetailsDarwin()
	default:
		return false, nil
	}
}
