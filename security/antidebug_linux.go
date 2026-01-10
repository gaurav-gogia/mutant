//go:build linux
// +build linux

package security

import (
	"os"
	"strings"
	"syscall"
)

// isDebuggerPresentLinux checks for debugger using ptrace
func isDebuggerPresentLinux() bool {
	// Check /proc/self/status for TracerPid
	if checkTracerPid() {
		return true
	}

	// Try to attach ptrace to ourselves
	// If we can't, something else has (like a debugger)
	if checkPtraceSelf() {
		return true
	}

	return false
}

// checkTracerPid reads /proc/self/status to check for tracer
func checkTracerPid() bool {
	data, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return false
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "TracerPid:") {
			// TracerPid:0 means no tracer
			// TracerPid:>0 means something is tracing us
			parts := strings.Fields(line)
			if len(parts) >= 2 && parts[1] != "0" {
				return true
			}
		}
	}

	return false
}

// checkPtraceSelf tries to ptrace ourselves
func checkPtraceSelf() bool {
	// Try PTRACE_TRACEME - if it fails, we're already being traced
	err := syscall.PtraceAttach(os.Getpid())
	if err != nil {
		// EPERM means already being traced
		if err == syscall.EPERM {
			return true
		}
	} else {
		// Detach if we successfully attached
		syscall.PtraceDetach(os.Getpid())
	}

	return false
}

// detectDebuggerParentLinux checks parent process for known debuggers
func detectDebuggerParentLinux() bool {
	// Read /proc/self/cmdline for parent process
	ppid := os.Getppid()

	cmdlinePath := "/proc/" + string(rune(ppid)) + "/cmdline"
	data, err := os.ReadFile(cmdlinePath)
	if err != nil {
		return false
	}

	cmdline := string(data)
	cmdlineLower := strings.ToLower(cmdline)

	// Common debugger names
	debuggers := []string{
		"gdb",
		"lldb",
		"strace",
		"ltrace",
		"radare2",
		"r2",
		"edb",
	}

	for _, debugger := range debuggers {
		if strings.Contains(cmdlineLower, debugger) {
			return true
		}
	}

	return false
}

// CheckLDPreload detects LD_PRELOAD (common hooking technique)
func CheckLDPreload() bool {
	ldPreload := os.Getenv("LD_PRELOAD")
	if ldPreload != "" {
		return true
	}

	ldLibraryPath := os.Getenv("LD_LIBRARY_PATH")
	if ldLibraryPath != "" {
		// Check for suspicious paths
		suspicious := []string{
			"/tmp",
			"tmp",
		}
		for _, path := range suspicious {
			if strings.Contains(ldLibraryPath, path) {
				return true
			}
		}
	}

	return false
}
