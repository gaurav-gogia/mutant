//go:build darwin
// +build darwin

package security

import (
	"os"
	"strings"
	"syscall"
	"unsafe"
)

// isDebuggerPresentDarwin checks for debugger on macOS
func isDebuggerPresentDarwin() bool {
	// Use sysctl to check P_TRACED flag
	return checkSysctlPTraced()
}

// checkSysctlPTraced uses sysctl to check if process is being traced
func checkSysctlPTraced() bool {
	// sysctl structure
	type kinfo_proc struct {
		_    [40]byte // padding to get to kp_proc
		Pid  int32
		_    [296]byte // padding to get to p_flag
		Flag uint32
		_    [624]byte // rest of structure
	}

	const P_TRACED = 0x00000800 // Process is being traced

	mib := []int32{1, 14, 1, int32(os.Getpid()), int32(unsafe.Sizeof(kinfo_proc{})), 1}

	var info kinfo_proc
	size := unsafe.Sizeof(info)

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

// detectDebuggerParentDarwin checks parent process for known debuggers
func detectDebuggerParentDarwin() bool {
	// Get parent process ID
	ppid := os.Getppid()

	// Try to read process name (simplified)
	// Full implementation would use sysctl or proc_pidpath
	_ = ppid

	// Check environment for common debugger variables
	if os.Getenv("__LLDB_DEBUGSERVER__") != "" {
		return true
	}

	// Check parent process name (would need additional syscalls)
	// For now, return false to avoid false positives
	return false
}

// CheckLLDBEnvironment checks for LLDB debugger environment
func CheckLLDBEnvironment() bool {
	lldbVars := []string{
		"__LLDB_DEBUGSERVER__",
		"LLDB_DEBUGSERVER_PATH",
	}

	for _, envVar := range lldbVars {
		if os.Getenv(envVar) != "" {
			return true
		}
	}

	return false
}

// CheckDyldInsert checks for DYLD_INSERT_LIBRARIES (hooking)
func CheckDyldInsert() bool {
	dyldInsert := os.Getenv("DYLD_INSERT_LIBRARIES")
	if dyldInsert != "" {
		return true
	}

	// Check for other suspicious DYLD variables
	suspicious := []string{
		"DYLD_FORCE_FLAT_NAMESPACE",
		"DYLD_PRINT_LIBRARIES",
	}

	for _, envVar := range suspicious {
		if os.Getenv(envVar) != "" {
			return true
		}
	}

	return false
}

// getProcessName retrieves process name for given PID
func getProcessName(pid int) string {
	// Simplified - would use proc_pidpath in full implementation
	cmdlinePath := "/proc/" + string(rune(pid)) + "/cmdline"
	data, err := os.ReadFile(cmdlinePath)
	if err != nil {
		return ""
	}

	cmdline := string(data)
	cmdlineLower := strings.ToLower(cmdline)

	// Common debugger names
	debuggers := []string{
		"lldb",
		"gdb",
		"radare2",
	}

	for _, debugger := range debuggers {
		if strings.Contains(cmdlineLower, debugger) {
			return debugger
		}
	}

	return ""
}
