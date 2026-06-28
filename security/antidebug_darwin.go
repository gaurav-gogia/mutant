//go:build darwin
// +build darwin

package security

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"unsafe"
)

// isDebuggerPresentDarwin performs multiple anti-debugging checks on macOS/Darwin
// Uses techniques employed by security vendors like Objective-See
func isDebuggerPresentDarwin() bool {
	detected, _ := detectDebuggerDetailsDarwin()
	return detected
}

func detectDebuggerDetailsDarwin() (bool, []string) {
	methods := make([]string, 0, 3)

	// Check 1: P_TRACED flag (ptrace-based debuggers like lldb, gdb)
	if isProcessBeingTraced() {
		methods = append(methods, "darwin:p_traced")
	}

	// Check 2: Check parent process for known debuggers
	if isParentDebuggerDarwin() {
		methods = append(methods, "darwin:parent_debugger_process")
	}

	// Check 3: Environment variables set by debugging tools
	if hasDebuggerEnvironmentMarkersDarwin() {
		methods = append(methods, "darwin:debugger_environment")
	}

	return len(methods) > 0, methods
}

// isProcessBeingTraced checks if the process is being traced via P_TRACED flag
func isProcessBeingTraced() bool {
	// Define kinfo_proc structure (simplified, only the parts we need)
	// The P_TRACED flag is at a specific offset in the kp_proc.p_flag field
	type kinfoProc struct {
		_    [40]byte  // padding to kp_proc
		_    [4]byte   // kp_proc.p_pid
		_    [296]byte // padding to kp_proc.p_flag
		Flag uint32    // kp_proc.p_flag
		_    [624]byte // rest of structure
	}

	const P_TRACED = 0x00000800 // Process is being traced

	// sysctl MIB for querying process info
	// CTL_KERN.KERN_PROC.KERN_PROC_PID.<pid>
	mib := []int32{1, 14, 1, int32(os.Getpid()), int32(unsafe.Sizeof(kinfoProc{})), 1}

	var info kinfoProc
	size := uintptr(unsafe.Sizeof(info))

	_, _, err := syscall.Syscall6(
		syscall.SYS___SYSCTL,
		uintptr(unsafe.Pointer(&mib[0])),
		uintptr(len(mib)),
		uintptr(unsafe.Pointer(&info)),
		uintptr(unsafe.Pointer(&size)),
		0,
		0,
	)

	if err != 0 {
		return false
	}

	// Check if P_TRACED flag is set
	return (info.Flag & P_TRACED) != 0
}

// isParentDebuggerDarwin checks if the parent process is a known debugger
func isParentDebuggerDarwin() bool {
	ppid := os.Getppid()

	// Try to get parent process name via /proc
	// Note: macOS /proc is limited, so we use ps command
	cmd := exec.Command("ps", "-o", "comm=", "-p", strconv.Itoa(ppid))
	output, err := cmd.CombinedOutput()
	if err != nil {
		return false
	}

	parentName := strings.ToLower(strings.TrimSpace(string(output)))

	// Known macOS debuggers and development tools
	debuggerPatterns := []string{
		"lldb", "gdb", "xcode", "simulator", "instruments",
		"dtrace", "fs_usage", "sample", "trace", "sc_usage",
		"leaks", "malloc_history", "heap", "vmmap",
		"frida-server", "idb", "appium",
	}

	for _, pattern := range debuggerPatterns {
		if strings.Contains(parentName, pattern) {
			return true
		}
	}

	return false
}

// hasDebuggerEnvironmentMarkersDarwin checks for debugger environment markers on macOS
func hasDebuggerEnvironmentMarkersDarwin() bool {
	debugEnvVars := []string{
		"LLDB_DEBUGSERVER_PORT",
		"LLDB_MasterPort",
		"GDB_OPTS",
		"GDBHISTFILE",
		"XCODE_DEBUG_PORT",
		"DYLD_INSERT_LIBRARIES", // Code injection
		"DYLD_ROOT_PATH",        // Simulator indicator
		"XCODE_VERSION_ACTUAL",  // Xcode debugging
		"XPC_DEBUG",             // XPC debugging
		"LLVM_DEBUG",
		"FRIDA_DEBUG",
	}

	for _, envVar := range debugEnvVars {
		if _, exists := os.LookupEnv(envVar); exists {
			return true
		}
	}

	// Check for unusual DYLD settings which indicate debugging
	if dyldLibs, exists := os.LookupEnv("DYLD_INSERT_LIBRARIES"); exists {
		// Check if suspicious libraries are being injected
		suspiciousLibs := []string{
			"libgmalloc", "libc++abi", "libsystem_trace", "libsystem_sandbox",
		}
		for _, lib := range suspiciousLibs {
			if strings.Contains(dyldLibs, lib) {
				return true
			}
		}
	}

	return false
}

// hasDebuggerProcessRunningDarwin checks if known debuggers are running
func hasDebuggerProcessRunningDarwin() bool {
	debuggers := []string{
		"lldb", "gdb", "xcode", "Instruments", "Simulator",
		"frida-server", "idb",
	}

	for _, debugger := range debuggers {
		cmd := exec.Command("pgrep", "-x", debugger)
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
	cmd := exec.Command("otool", "-L", "/proc/self/exe")
	output, err := cmd.CombinedOutput()
	if err == nil {
		outputStr := strings.ToLower(string(output))

		// Debuggers often load debugging libraries
		suspiciousIndicators := []string{
			"liblldb", "libgdb", "libdebug",
			"/tmp/", "/var/tmp/", // Unusual library locations
		}

		for _, indicator := range suspiciousIndicators {
			if strings.Contains(outputStr, indicator) {
				return true
			}
		}
	}

	return false
}
