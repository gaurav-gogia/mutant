//go:build darwin
// +build darwin

package security

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

const (
	darwinDebugMethodPTraced    = "darwin:p_traced"
	darwinDebugMethodParentProc = "darwin:parent_debugger_process"
	darwinDebugMethodEnvMarkers = "darwin:debugger_environment"

	darwinSysctlProcByPID = "kern.proc.pid"

	darwinDYLDInsertLibsEnv = "DYLD_INSERT_LIBRARIES"
	darwinPsCommand         = "ps"
	darwinPgrepCommand      = "pgrep"
	darwinOtoolCommand      = "otool"
)

var (
	// darwinDebuggerEnvVars are the variables
	// hasDebuggerEnvironmentMarkersDarwin reads. Two kinds of setting are not
	// among them, because neither says anything about this process
	// (M26-TMP-010): where a debugger keeps its history -- GDBHISTFILE, which
	// XDG base-directory guides tell users to export from a shell profile --
	// and the build settings Xcode hands to every script phase it runs,
	// XCODE_VERSION_ACTUAL among them.
	darwinDebuggerEnvVars = []string{
		"LLDB_DEBUGSERVER_PORT",
		"LLDB_MasterPort",
		"GDB_OPTS",
		"XCODE_DEBUG_PORT",
		darwinDYLDInsertLibsEnv,
		"DYLD_ROOT_PATH",
		"XPC_DEBUG",
		"LLVM_DEBUG",
		"FRIDA_DEBUG",
	}

	darwinRunningDebuggers = []string{"lldb", "gdb", "xcode", "Instruments", "Simulator", "frida-server", "idb"}
	darwinOtoolIndicators  = []string{"liblldb", "libgdb", "libdebug", "/tmp/", "/var/tmp/"}
)

// isDebuggerPresentDarwin performs multiple anti-debugging checks on macOS/Darwin
// Uses techniques employed by security vendors like Objective-See
func isDebuggerPresentDarwin() bool {
	detected, _ := detectDebuggerDetailsDarwin()
	return detected
}

func detectDebuggerDetailsDarwin() (bool, []string) {
	methods := make([]string, 0, 3)

	// Check 1: P_TRACED flag (ptrace-based debuggers like lldb, gdb). A flag
	// that could not be read was not measured, which is not the same as clear.
	// This function's answer -- a verdict and the methods that fired -- has no
	// place for "not measured", so the security dev log (--dev with
	// --security-log-level debug) says it, and the check adds no method.
	traced, err := isTracedDarwin()
	if err != nil {
		securityDevLogf("debugger check %s not measured: %v", darwinDebugMethodPTraced, err)
	}
	if traced {
		methods = append(methods, darwinDebugMethodPTraced)
	}

	// Check 2: Check parent process for known debuggers
	if isParentDebuggerDarwin() {
		methods = append(methods, darwinDebugMethodParentProc)
	}

	// Check 3: Environment variables set by debugging tools
	if hasDebuggerEnvironmentMarkersDarwin() {
		methods = append(methods, darwinDebugMethodEnvMarkers)
	}

	return len(methods) > 0, methods
}

// isTracedDarwin reports whether a debugger is tracing this process, by the
// P_TRACED bit that the kernel sets in p_flag of the process's kinfo_proc while
// lldb, or anything else attached through ptrace, traces it.
//
// An error means the bit was not read: not measured, which is not the same as
// clear. The caller says so in the security dev log and adds no method.
func isTracedDarwin() (bool, error) {
	return darwinProcessTraced(os.Getpid())
}

// darwinProcessTraced reads P_TRACED for pid, from the kinfo_proc that sysctl
// kern.proc.pid.<pid> returns.
//
// The request and the record are x/sys's, unix.SysctlKinfoProc and
// unix.KinfoProc, rather than a MIB and a layout written out here. The ones
// written out here before meant the check could never find a debugger
// (M26-TMP-011). The MIB had six elements, {CTL_KERN, KERN_PROC,
// KERN_PROC_PID, pid, size, 1}, which is the OpenBSD form; xnu's handler for
// kern.proc.pid takes exactly one element after the name, the pid, and refuses
// anything else with EINVAL, and the refusal was returned as "not traced". Had
// a request gone through, p_flag would have been read 340 bytes into a
// 968-byte record, where the kernel puts it 32 bytes into a 648-byte one on
// both amd64 and arm64. unix.SysctlKinfoProc builds the four-element MIB from
// the name, and refuses a reply that is not exactly one record.
func darwinProcessTraced(pid int) (bool, error) {
	kp, err := unix.SysctlKinfoProc(darwinSysctlProcByPID, pid)
	if err != nil {
		return false, fmt.Errorf("sysctl %s.%d: %w", darwinSysctlProcByPID, pid, err)
	}
	return darwinTracedFromRecord(pid, kp.Proc.P_pid, kp.Proc.P_flag)
}

// isParentDebuggerDarwin checks whether the parent process is one of the
// debuggers in darwinDebuggerProcessNames, by the name of its executable.
//
// macOS has no /proc, so the parent's name comes from ps, and the name is
// compared exactly rather than searched for inside what ps printed. Searching
// matched far more than it meant to even on a bare name: the list holds "trace",
// which matched ltrace, pytrace and dtrace, and "heap", which matched
// heaptrack, so an ordinary parent was reported as a debugger -- and in secure
// mode that ends the run (M26-TMP-019). `ps -o comm=` on this platform is
// documented to print the executable's path, which if it holds makes the same
// list match every tool under /Applications/Xcode.app through the entry
// "xcode"; taking the base name covers both readings and needs neither to be
// settled.
//
// A parent whose name cannot be read is not a debugger, for the same reason as
// on Linux: isTracedDarwin is the check that finds one actually attached, and
// this one should err towards silence.
func isParentDebuggerDarwin() bool {
	ppid := os.Getppid()

	cmd := exec.Command(darwinPsCommand, "-o", "comm=", "-p", strconv.Itoa(ppid))
	output, err := cmd.CombinedOutput()
	if err != nil {
		return false
	}

	return debuggerProcessNameMatches(string(output), darwinDebuggerProcessNames)
}

// hasDebuggerEnvironmentMarkersDarwin reports whether any variable in
// darwinDebuggerEnvVars holds a value: dyld's DYLD_INSERT_LIBRARIES, which
// names libraries for dyld to insert into this process as it starts, or a
// variable whose name ties it to debugging or instrumentation.
// debuggerEnvironmentMarkerSet says what holding a value means.
//
// DYLD_ROOT_PATH is on the list as well, though dyld does nothing with it
// here: on macOS 12 and later it reads it only in a simulator process (see
// loaderListSeparators). Like the debugging names, it counts when it holds a
// value.
//
// DYLD_INSERT_LIBRARIES counts whichever library it names. A filter that
// counted it only for a few libraries, libgmalloc among them, sat after a loop
// that had already returned on the variable existing, so it never ran
// (M26-TMP-010).
func hasDebuggerEnvironmentMarkersDarwin() bool {
	return debuggerEnvironmentMarkerSet(darwinDebuggerEnvVars, os.Getenv)
}

// hasDebuggerProcessRunningDarwin checks if known debuggers are running
func hasDebuggerProcessRunningDarwin() bool {
	for _, debugger := range darwinRunningDebuggers {
		cmd := exec.Command(darwinPgrepCommand, "-x", debugger)
		err := cmd.Run()
		if err == nil {
			return true
		}
	}

	return false
}

// hasInjectedCode detects code injection or unusual memory patterns
// Debuggers on macOS often inject code via DYLD or task ports
func hasInjectedCode() bool {
	// Check if we can access our own task port (sign of debugging)
	// This is a simplified check - in reality, task port access is restricted

	// Alternative: Check for unusual library loading
	// Check if standard system libraries are loaded from unusual paths
	cmd := exec.Command(darwinOtoolCommand, "-L", "/proc/self/exe")
	output, err := cmd.CombinedOutput()
	if err == nil {
		outputStr := strings.ToLower(string(output))

		// Debuggers often load debugging libraries
		for _, indicator := range darwinOtoolIndicators {
			if strings.Contains(outputStr, indicator) {
				return true
			}
		}
	}

	return false
}
