//go:build linux
// +build linux

package security

import (
	"os"
	"strconv"
	"strings"
)

const (
	linuxProcSelfStatusPath   = "/proc/self/status"
	linuxTracerPidPrefix      = "TracerPid:"
	linuxParentCommPathPrefix = "/proc/"
	linuxParentCommPathSuffix = "/comm"
	linuxLDPreloadVar         = "LD_PRELOAD"
	linuxForcedExitCode       = 1
)

var (
	// linuxDebuggerEnvVars are the variables hasDebuggerEnvironmentMarkers
	// reads (M26-TMP-010). GDBHISTFILE is not among them: it is where gdb
	// keeps its history, which XDG base-directory guides tell users to export
	// from a shell profile, and the location of a history file says nothing
	// about this process. Nor is LLDB_HIST_FILE, which only looks like such a
	// setting: no lldb reads it, since lldb keeps its history under ~/.lldb
	// and has no variable of its own to move it.
	linuxDebuggerEnvVars = []string{
		"GDB_OPTS",
		"LLDB_DEBUGSERVER",
		"VALGRIND_LIB",
		"VALGRIND_PID",
		linuxLDPreloadVar,
		"LD_AUDIT",
		"SYSTEMTAP_STAPRUN",
		"FRIDA_SERVER_PORT",
	}
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

	// Check 2: Debugger process detection via /proc/<ppid>/comm of the parent
	if isParentDebugger() {
		methods = append(methods, "linux:parent_debugger_process")
	}

	// Check 3: debugger and library-loading markers in the environment
	if hasDebuggerEnvironmentMarkers() {
		methods = append(methods, "linux:debugger_environment")
	}

	return len(methods) > 0, methods
}

// isTracingDetected checks if the process is being traced via ptrace
// Used by debuggers like gdb, lldb, and system tracing tools
func isTracingDetected() bool {
	data, err := os.ReadFile(linuxProcSelfStatusPath)
	if err != nil {
		return false
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, linuxTracerPidPrefix) {
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

// isParentDebugger checks whether the parent process is one of the debuggers
// in linuxDebuggerProcessNames, by the name of its executable and nothing else.
//
// /proc/<ppid>/comm is the kernel's own name for the parent: no path, no
// arguments, and at most fifteen characters, which every name in the list fits
// inside. Reading /proc/<ppid>/cmdline instead and searching the whole of it is
// what made an ordinary argument look like a debugger, and in secure mode that
// ends the run (M26-TMP-002).
//
// A parent whose comm cannot be read is not a debugger. The file is gone for a
// reaped parent and unreadable under some container policies, and of the three
// checks the Linux detector makes, isTracingDetected is the one that finds a
// debugger actually attached, so this one fails towards silence rather than
// terminating a run over a file it could not open.
func isParentDebugger() bool {
	ppid := os.Getppid()

	commPath := linuxParentCommPathPrefix + strconv.Itoa(ppid) + linuxParentCommPathSuffix
	commData, err := os.ReadFile(commPath)
	if err != nil {
		return false
	}

	return debuggerProcessNameMatches(string(commData), linuxDebuggerProcessNames)
}

// hasDebuggerEnvironmentMarkers reports whether any variable in
// linuxDebuggerEnvVars holds a value: the dynamic loader's LD_PRELOAD and
// LD_AUDIT, which name libraries for the loader to load into a dynamically
// linked program as it starts, or a variable named for a debugging or
// instrumentation tool. debuggerEnvironmentMarkerSet says what holding a
// value means.
//
// Neither loader variable can load anything into this program as released: a
// Linux release is built with CGO_ENABLED=0 and statically linked, so the
// dynamic loader never runs for it. They are reported all the same, as
// docs/CONFIGURATION_POLICY.md has it for LD_PRELOAD: an indicator of
// compromise, read and reported.
//
// LD_PRELOAD counts whatever the length of its value. A rule that counted it
// only past fifty characters sat after a loop that had already returned on
// the variable existing, so it never ran (M26-TMP-010).
func hasDebuggerEnvironmentMarkers() bool {
	return debuggerEnvironmentMarkerSet(linuxDebuggerEnvVars, os.Getenv)
}

// DetectDebuggerAndTerminate is a helper that detects debuggers and terminates
// This should be called early in the program execution
func DetectDebuggerAndTerminate() error {
	if isDebuggerPresentLinux() {
		// Take evasive action: exit ungracefully
		os.Exit(linuxForcedExitCode)
	}
	return nil
}
