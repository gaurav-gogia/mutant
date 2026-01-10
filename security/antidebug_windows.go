//go:build windows
// +build windows

package security

import (
	"strings"
	"syscall"
	"unsafe"
)

var (
	kernel32                       = syscall.NewLazyDLL("kernel32.dll")
	procIsDebuggerPresent          = kernel32.NewProc("IsDebuggerPresent")
	procCheckRemoteDebuggerPresent = kernel32.NewProc("CheckRemoteDebuggerPresent")
	procGetCurrentProcess          = kernel32.NewProc("GetCurrentProcess")
)

// isDebuggerPresentWindows checks for debugger using Windows API
func isDebuggerPresentWindows() bool {
	ret, _, _ := procIsDebuggerPresent.Call()
	return ret != 0
}

// detectDebuggerParentWindows checks parent process for known debuggers
func detectDebuggerParentWindows() bool {
	// Get parent process info
	parentName := getParentProcessName()

	// List of common debugger process names
	debuggers := []string{
		"ollydbg.exe",
		"x64dbg.exe",
		"x32dbg.exe",
		"windbg.exe",
		"ida.exe",
		"ida64.exe",
		"idaq.exe",
		"idaq64.exe",
		"radare2.exe",
		"gdb.exe",
		"immunity debugger.exe",
	}

	parentLower := strings.ToLower(parentName)
	for _, debugger := range debuggers {
		if strings.Contains(parentLower, debugger) {
			return true
		}
	}

	return false
}

// getParentProcessName retrieves the parent process name (simplified)
func getParentProcessName() string {
	// This is a simplified version
	// Full implementation would use CreateToolhelp32Snapshot
	// For now, return empty to avoid false positives
	return ""
}

// CheckRemoteDebugger checks if a remote debugger is attached
func CheckRemoteDebugger() bool {
	handle, _, _ := procGetCurrentProcess.Call()
	var isDebuggerPresent int32

	ret, _, _ := procCheckRemoteDebuggerPresent.Call(
		handle,
		uintptr(unsafe.Pointer(&isDebuggerPresent)),
	)

	if ret == 0 {
		return false
	}

	return isDebuggerPresent != 0
}

// IsBeingDebugged checks PEB (Process Environment Block) BeingDebugged flag
func IsBeingDebugged() bool {
	// Access PEB directly (advanced technique)
	// This is harder for debuggers to fake

	// In 64-bit: PEB at gs:[0x60], BeingDebugged at offset 0x02
	// In 32-bit: PEB at fs:[0x30], BeingDebugged at offset 0x02

	// Note: This requires assembly or unsafe operations
	// Simplified version - use IsDebuggerPresent for now
	return isDebuggerPresentWindows()
}
