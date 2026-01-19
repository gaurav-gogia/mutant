//go:build darwin
// +build darwin

package security

import (
	"os"
	"syscall"
	"unsafe"
)

// isDebuggerPresentDarwin checks if a debugger is attached using sysctl.
// It queries the kinfo_proc structure for the current process and checks
// the P_TRACED flag which indicates if the process is being traced/debugged.
func isDebuggerPresentDarwin() bool {
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
