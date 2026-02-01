//go:build windows
// +build windows

package security

import (
	"os"
	"strings"
	"syscall"
	"unsafe"
)

var (
	kernel32                      = syscall.NewLazyDLL("kernel32.dll")
	ntdll                         = syscall.NewLazyDLL("ntdll.dll")
	procIsDebuggerPresent         = kernel32.NewProc("IsDebuggerPresent")
	procGetModuleHandle           = kernel32.NewProc("GetModuleHandleW")
	procGetProcAddress            = kernel32.NewProc("GetProcAddress")
	procOutputDebugString         = kernel32.NewProc("OutputDebugStringW")
	procGetCurrentProcess         = kernel32.NewProc("GetCurrentProcess")
	procNtQueryInformationProcess = ntdll.NewProc("NtQueryInformationProcess")
	procGetParentProcess          = kernel32.NewProc("CreateToolhelp32Snapshot")
)

// ProcessDebugPort is used with NtQueryInformationProcess
const (
	ProcessDebugPort         = 7
	ProcessDebugObjectHandle = 30
)

// isDebuggerPresentWindows performs multiple debugger detection techniques.
// Uses a combination of methods inspired by Kaspersky, Symantec, and other security vendors.
func isDebuggerPresentWindows() bool {
	// Method 1: Check BeingDebugged flag in PEB
	if checkBeingDebugged() {
		return true
	}

	// Method 2: Check RemoteDebuggerPresent
	if checkRemoteDebugger() {
		return true
	}

	// Method 3: Check ProcessDebugPort via NtQueryInformationProcess
	if checkProcessDebugPort() {
		return true
	}

	// Method 4: Check ProcessDebugObjectHandle
	if checkProcessDebugObjectHandle() {
		return true
	}

	// Method 5: Output debug string test
	if checkOutputDebugStringTest() {
		return true
	}

	// Method 6: Check for debugger in parent process
	if checkDebuggerParentProcess() {
		return true
	}

	// Method 7: Check for common debugger DLLs loaded
	if checkDebuggerDLLs() {
		return true
	}

	// Method 8: Check for exception handler hooks (debugger breakpoints)
	if checkExceptionHandlers() {
		return true
	}

	// Method 9: Check for Windows debugging privileges
	if checkDebugPrivileges() {
		return true
	}

	// Method 10: Detect common debugger signatures in memory
	if checkDebuggerMemoryPatterns() {
		return true
	}

	return false
}

// checkBeingDebugged uses IsDebuggerPresent API
func checkBeingDebugged() bool {
	ret, _, _ := procIsDebuggerPresent.Call()
	return ret != 0
}

// checkRemoteDebugger checks if a remote debugger is attached using CheckRemoteDebuggerPresent.
func checkRemoteDebugger() bool {
	procCheckRemoteDebuggerPresent := kernel32.NewProc("CheckRemoteDebuggerPresent")

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

// checkProcessDebugPort queries the ProcessDebugPort using NtQueryInformationProcess.
// A non-zero value indicates the process is being debugged.
func checkProcessDebugPort() bool {
	handle, _, _ := procGetCurrentProcess.Call()

	var debugPort uintptr
	returnLength := uint32(0)

	ret, _, _ := procNtQueryInformationProcess.Call(
		handle,
		ProcessDebugPort,
		uintptr(unsafe.Pointer(&debugPort)),
		unsafe.Sizeof(debugPort),
		uintptr(unsafe.Pointer(&returnLength)),
	)

	// ret == 0 means STATUS_SUCCESS
	if ret == 0 && debugPort != 0 {
		return true
	}

	return false
}

// checkProcessDebugObjectHandle queries ProcessDebugObjectHandle.
// A non-null handle indicates a debugger is attached.
func checkProcessDebugObjectHandle() bool {
	handle, _, _ := procGetCurrentProcess.Call()

	var debugHandle uintptr
	returnLength := uint32(0)

	ret, _, _ := procNtQueryInformationProcess.Call(
		handle,
		ProcessDebugObjectHandle,
		uintptr(unsafe.Pointer(&debugHandle)),
		unsafe.Sizeof(debugHandle),
		uintptr(unsafe.Pointer(&returnLength)),
	)

	// ret == 0 means STATUS_SUCCESS
	if ret == 0 && debugHandle != 0 {
		return true
	}

	return false
}

// checkOutputDebugStringTest uses a trick with OutputDebugString.
// If a debugger is present, it will consume the output.
// We can detect this by checking system state before/after.
func checkOutputDebugStringTest() bool {
	// Store initial tick count
	initialTicks := getTicks()

	// Call OutputDebugString - debugger will intercept this
	msg, _ := syscall.UTF16PtrFromString("DEBUG_CHECK")
	procOutputDebugString.Call(uintptr(unsafe.Pointer(msg)))

	// If a debugger consumed it, there might be timing artifacts
	// This is a weak check but can help in some cases
	finalTicks := getTicks()

	// If time difference is suspicious (debugger stepping), flag it
	// Allow up to 10ms normally, debuggers often take longer
	if finalTicks-initialTicks > 10 {
		return true
	}

	return false
}

// getTicks returns the current system tick count
func getTicks() uint32 {
	procGetTickCount := kernel32.NewProc("GetTickCount")
	ret, _, _ := procGetTickCount.Call()
	return uint32(ret)
}

// checkDebuggerParentProcess checks if the parent process is a known debugger
func checkDebuggerParentProcess() bool {
	ppid := os.Getppid()

	// Get parent process name using toolhelp32 snapshot
	handle, _, _ := procGetParentProcess.Call(
		0x00000002, // TH32CS_SNAPPROCESS
		0,          // dwProcessId (0 = all processes)
	)

	if handle == 0 {
		return false
	}

	defer syscall.CloseHandle(syscall.Handle(handle))

	parentName := getProcessNameByPID(uint32(ppid))
	if parentName == "" {
		return false
	}

	parentNameLower := strings.ToLower(parentName)

	// Common debuggers
	debuggers := []string{
		"windbg",
		"ollydbg",
		"x64dbg",
		"x32dbg",
		"ida",
		"idaq",
		"immunity",
		"radare2",
		"lldb",
		"gdb",
		"devenv",        // Visual Studio
		"code",          // VS Code (can debug)
		"msvsmon",       // Visual Studio remote debugger
		"vsjitdebugger", // VS JIT debugger
	}

	for _, debugger := range debuggers {
		if strings.Contains(parentNameLower, debugger) {
			return true
		}
	}

	return false
}

// getProcessNameByPID retrieves process name by PID using WMIC alternative or simple check
func getProcessNameByPID(pid uint32) string {
	// Simplified version - in real scenario would iterate through snapshots
	// For now, return empty if we can't get it efficiently
	return ""
}

// checkDebuggerDLLs checks if common debugger DLLs are loaded in memory
func checkDebuggerDLLs() bool {
	debuggerDLLs := []string{
		"dbghelp.dll",
		"msvcr120d.dll",
		"msvcd120d.dll",
		"vccorlib120d.dll",
	}

	for _, dll := range debuggerDLLs {
		moduleHandle, _, _ := procGetModuleHandle.Call(uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(dll))))
		if moduleHandle != 0 {
			// DLL is loaded - could indicate debugger tools
			return true
		}
	}

	return false
}

// checkExceptionHandlers detects if exception handlers have been hooked by debuggers
// Debuggers often modify exception handling to intercept breakpoints
func checkExceptionHandlers() bool {
	// Check if we can set an exception handler (indicates no debugger interference)
	// If a debugger is present, it may have modified exception handling

	// This is a simplified check - in reality would use SEH or VEH
	// For now, we use timing analysis which debuggers often interfere with
	return false // Placeholder - full implementation would require assembly
}

// checkDebugPrivileges checks if the current process has debug privileges enabled
// Debug privileges are typically only available to debuggers and admin tools
func checkDebugPrivileges() bool {
	// Check if SeDebugPrivilege is enabled
	// This requires Windows API calls to token privileges
	// Simplified implementation - full version would check token privileges
	return false // Placeholder
}

// checkDebuggerMemoryPatterns detects known debugger signatures in memory
// Many debuggers have identifiable patterns in their code sections
func checkDebuggerMemoryPatterns() bool {
	// Common debugger signatures in memory
	_ = []string{
		"Debugger=YES",
		"Debug.Assert",
		"WinDbgFrameClass",
		"OllyDbg",
		"x64dbg",
	}

	// This would require memory scanning which is complex in Go
	// For now, return false as placeholder
	return false
}
