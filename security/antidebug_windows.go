//go:build windows
// +build windows

package security

import (
	"syscall"
	"unsafe"
)

var (
	kernel32              = syscall.NewLazyDLL("kernel32.dll")
	procIsDebuggerPresent = kernel32.NewProc("IsDebuggerPresent")
)

// isDebuggerPresentWindows uses the Windows API IsDebuggerPresent function.
// This checks the BeingDebugged flag in the Process Environment Block (PEB).
// Returns true if the process is being debugged.
func isDebuggerPresentWindows() bool {
	ret, _, _ := procIsDebuggerPresent.Call()
	if ret != 0 {
		return true
	}
	return checkRemoteDebugger()
}

// CheckRemoteDebugger checks if a remote debugger is attached using CheckRemoteDebuggerPresent.
// This is more comprehensive than IsDebuggerPresent as it can detect remote debuggers.
func checkRemoteDebugger() bool {
	procCheckRemoteDebuggerPresent := kernel32.NewProc("CheckRemoteDebuggerPresent")
	procGetCurrentProcess := kernel32.NewProc("GetCurrentProcess")

	handle, _, _ := procGetCurrentProcess.Call()
	var debuggerPresent int32

	ret, _, _ := procCheckRemoteDebuggerPresent.Call(
		handle,
		uintptr(unsafe.Pointer(&debuggerPresent)),
	)

	if ret == 0 {
		return false
	}

	return debuggerPresent != 0
}
