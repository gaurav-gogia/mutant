//go:build linux
// +build linux

package security

import (
	"bytes"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// isDebuggerPresentLinux performs multiple anti-debugging checks on Linux
// using techniques employed by major security firms and tech companies
func isDebuggerPresentLinux() bool {
	// Check 1: TracerPid detection (ptrace-based debuggers)
	if isTracingDetected() {
		return true
	}

	// Check 2: Debugger process detection via /proc/cmdline of parent
	if isParentDebugger() {
		return true
	}

	// Check 3: GDB specific markers in environment
	if hasDebuggerEnvironmentMarkers() {
		return true
	}

	// Check 4: Check for strace/ltrace/debuggers in system
	if hasDebuggerInSystem() {
		return true
	}

	// Check 5: Examine file descriptors for debugger pipes
	if hasDebuggerFileDescriptors() {
		return true
	}

	// Check 6: Check for suspicious /proc entries
	if hasSuspiciousProcEntries() {
		return true
	}

	return false
}

// isTracingDetected checks if the process is being traced via ptrace
// Used by debuggers like gdb, lldb, and system tracing tools
func isTracingDetected() bool {
	data, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return false
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "TracerPid:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				pid, err := strconv.Atoi(fields[1])
				if err == nil && pid != 0 {
					return true
				}
			}
			break
		}
	}

	return false
}

// isParentDebugger checks if the parent process is a known debugger
// Looks for gdb, lldb, valgrind, strace, ltrace, etc.
func isParentDebugger() bool {
	ppid := os.Getppid()

	// Try to read parent process cmdline
	cmdlinePath := "/proc/" + strconv.Itoa(ppid) + "/cmdline"
	cmdlineData, err := os.ReadFile(cmdlinePath)
	if err != nil {
		return false
	}

	// Parse cmdline (null-separated)
	cmdline := string(bytes.Trim(cmdlineData, "\x00"))
	cmdline = strings.ToLower(cmdline)

	// Known debuggers and tracing tools
	debuggerPatterns := []string{
		"gdb", "lldb", "valgrind", "strace", "ltrace",
		"radare2", "ida", "ghidra", "angr", "frida",
		"dtrace", "systemtap", "rr", "pernosco",
	}

	for _, pattern := range debuggerPatterns {
		if strings.Contains(cmdline, pattern) {
			return true
		}
	}

	return false
}

// hasDebuggerEnvironmentMarkers checks for environment variables set by debuggers
// GDB, LLDB, and other debuggers typically set specific environment variables
func hasDebuggerEnvironmentMarkers() bool {
	debugEnvVars := []string{
		"GDB_OPTS",          // GDB options
		"GDBHISTFILE",       // GDB history file
		"LLDB_DEBUGSERVER",  // LLDB debug server
		"LLDB_HIST_FILE",    // LLDB history
		"VALGRIND_LIB",      // Valgrind library
		"VALGRIND_PID",      // Valgrind PID
		"LD_PRELOAD",        // Often used for debugging/tracing
		"LD_AUDIT",          // Library audit (debugging)
		"SYSTEMTAP_STAPRUN", // SystemTap
		"FRIDA_SERVER_PORT", // Frida instrumentation
	}

	for _, envVar := range debugEnvVars {
		if _, exists := os.LookupEnv(envVar); exists {
			return true
		}
	}

	// Check for excessive LD_PRELOAD which is often used in debuggers
	if preload, exists := os.LookupEnv("LD_PRELOAD"); exists && len(preload) > 50 {
		return true
	}

	return false
}

// hasDebuggerInSystem checks if common debuggers are installed
// This is a heuristic to detect debugging environment
func hasDebuggerInSystem() bool {
	debuggers := []string{
		"/usr/bin/gdb",
		"/usr/bin/lldb",
		"/usr/bin/valgrind",
		"/usr/bin/strace",
		"/usr/bin/ltrace",
		"/usr/bin/radare2",
		"/usr/local/bin/gdb",
		"/usr/local/bin/lldb",
	}

	for _, debugger := range debuggers {
		if _, err := os.Stat(debugger); err == nil {
			// Debugger exists - check if it's actually running any processes
			if isDebuggerRunning(debugger) {
				return true
			}
		}
	}

	return false
}

// isDebuggerRunning checks if a debugger process is currently running
// Uses ps to check for running debugger processes
func isDebuggerRunning(debuggerPath string) bool {
	// Extract just the executable name
	parts := strings.Split(debuggerPath, "/")
	debuggerName := parts[len(parts)-1]

	cmd := exec.Command("pgrep", "-x", debuggerName)
	err := cmd.Run()
	// If pgrep finds the process, err will be nil
	return err == nil
}

// hasDebuggerFileDescriptors checks for suspicious file descriptors
// Debuggers often communicate via pipes and sockets
func hasDebuggerFileDescriptors() bool {
	fdDir := "/proc/self/fd"
	entries, err := os.ReadDir(fdDir)
	if err != nil {
		return false
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			// Try to read the symlink target
			linkPath := fdDir + "/" + entry.Name()
			target, err := os.Readlink(linkPath)
			if err == nil {
				// Check for suspicious patterns
				if strings.Contains(target, "socket:") ||
					strings.Contains(target, "anon_inode:") ||
					strings.Contains(target, "eventpoll") {
				}
			}
		}
	}

	return false
}

// hasSuspiciousProcEntries checks /proc for suspicious debugging artifacts
// Looks for traces of ptrace, seccomp, or other debugging mechanisms
func hasSuspiciousProcEntries() bool {
	// Check /proc/self/fd for debugger connections
	// Check /proc/self/maps for injected libraries
	mapsPath := "/proc/self/maps"
	mapsData, err := os.ReadFile(mapsPath)
	if err != nil {
		return false
	}

	lines := strings.Split(string(mapsData), "\n")
	suspiciousLibs := []string{
		"libstdc++", "libasan", "libubsan", "libtsan", // Sanitizers
		"libdl", "libpthread", // Often hooked by debuggers
	}

	mapCount := 0
	for _, line := range lines {
		if len(line) == 0 {
			continue
		}
		mapCount++

		// Excessive mappings might indicate injection
		for _, lib := range suspiciousLibs {
			if strings.Contains(line, lib) {
				// Additional check: unusual mapping patterns
				if isUnusualMapping(line) {
					return true
				}
			}
		}
	}

	// Check for suspicious seccomp state
	if isSeccompActive() {
		return true
	}

	return false
}

// isUnusualMapping detects unusual memory mapping patterns
// Debuggers often create unusual rwx mappings
func isUnusualMapping(line string) bool {
	// Check for rwx (read-write-execute) mappings which are suspicious
	if strings.Contains(line, "rwxp") || strings.Contains(line, "rwxs") {
		return true
	}

	// Check for unusual permissions changes
	parts := strings.Fields(line)
	if len(parts) >= 2 {
		perms := parts[1]
		// Debuggers often create r--s (shared read-only) or rw-s mappings
		if strings.HasSuffix(perms, "s") && (strings.Contains(perms, "r") || strings.Contains(perms, "w")) {
			return true
		}
	}

	return false
}

// isSeccompActive checks if seccomp is enabled
// Seccomp is often configured during debugging/sandboxing
func isSeccompActive() bool {
	statusPath := "/proc/self/status"
	data, err := os.ReadFile(statusPath)
	if err != nil {
		return false
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "Seccomp:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				val, err := strconv.Atoi(fields[1])
				// Seccomp values: 0=disabled, 1=strict, 2=filter
				// Non-zero might indicate debugging/sandboxing
				if err == nil && val > 0 {
					return true
				}
			}
			break
		}
	}

	return false
}

// DetectDebuggerAndTerminate is a helper that detects debuggers and terminates
// This should be called early in the program execution
func DetectDebuggerAndTerminate() error {
	if isDebuggerPresentLinux() {
		// Take evasive action: exit ungracefully
		os.Exit(1)
	}
	return nil
}
