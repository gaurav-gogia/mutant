//go:build linux
// +build linux

package security

import (
	"bytes"
	"os"
	"strconv"
	"strings"
)

// isDebuggerPresentLinux performs multiple anti-debugging checks on Linux
// using techniques employed by major security firms and tech companies
func isDebuggerPresentLinux() bool {
	detected, _ := detectDebuggerDetailsLinux()
	return detected
}

func detectDebuggerDetailsLinux() (bool, []string) {
	methods := make([]string, 0, 3)

	// Check 1: TracerPid detection (ptrace-based debuggers)
	if isTracingDetected() {
		methods = append(methods, "linux:tracer_pid")
	}

	// Check 2: Debugger process detection via /proc/cmdline of parent
	if isParentDebugger() {
		methods = append(methods, "linux:parent_debugger_process")
	}

	// Check 3: GDB specific markers in environment
	if hasDebuggerEnvironmentMarkers() {
		methods = append(methods, "linux:debugger_environment")
	}

	return len(methods) > 0, methods
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

	// Known debuggers and reverse-engineering tools
	debuggerPatterns := []string{
		"gdb", "lldb", "valgrind",
		"radare2", "ida", "ghidra", "angr", "frida",
		"rr", "pernosco",
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

// DetectDebuggerAndTerminate is a helper that detects debuggers and terminates
// This should be called early in the program execution
func DetectDebuggerAndTerminate() error {
	if isDebuggerPresentLinux() {
		// Take evasive action: exit ungracefully
		os.Exit(1)
	}
	return nil
}
