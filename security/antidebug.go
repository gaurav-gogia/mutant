package security

import (
	"runtime"
	"time"
)

// IsDebuggerPresent checks if a debugger is attached
// Returns true if debugger detected, false otherwise
func IsDebuggerPresent() bool {
	// Check platform-specific debugger presence
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

// DetectDebuggerAdvanced performs multiple detection techniques
func DetectDebuggerAdvanced() bool {
	// Technique 1: Platform-specific API check
	if IsDebuggerPresent() {
		return true
	}

	// Technique 2: Timing attack detection
	if detectTimingAnomaly() {
		return true
	}

	// Technique 3: Parent process check (platform-specific)
	if detectDebuggerParent() {
		return true
	}

	return false
}

// detectTimingAnomaly uses timing to detect debuggers
// Debuggers slow down execution significantly
func detectTimingAnomaly() bool {
	iterations := 10000
	threshold := 50 * time.Millisecond // Adjust based on system

	start := time.Now()

	// Simple computation that should be fast
	sum := 0
	for i := 0; i < iterations; i++ {
		sum += i
	}

	elapsed := time.Since(start)

	// If this takes too long, likely debugger interference
	if elapsed > threshold {
		return true
	}

	// Use sum to prevent optimization
	_ = sum

	return false
}

// detectDebuggerParent checks parent process name
// Platform-specific implementations below
func detectDebuggerParent() bool {
	switch runtime.GOOS {
	case "windows":
		return detectDebuggerParentWindows()
	case "linux":
		return detectDebuggerParentLinux()
	case "darwin":
		return detectDebuggerParentDarwin()
	default:
		return false
	}
}
