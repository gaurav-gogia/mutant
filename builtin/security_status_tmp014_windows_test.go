//go:build windows

package builtin

import (
	"bytes"
	"encoding/binary"
	"os"
	"os/exec"
	"runtime"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

const (
	// tmp014ChildLimit bounds how long the debuggee may take before the test
	// ends it.
	tmp014ChildLimit = 2 * time.Minute

	tmp014DebugOnlyThisProcess   = 0x00000002
	tmp014ExceptionDebugEvent    = 1
	tmp014CreateProcessEvent     = 3
	tmp014ExitProcessEvent       = 5
	tmp014LoadDLLEvent           = 6
	tmp014DBGContinue            = 0x00010002
	tmp014DBGExceptionNotHandled = 0x80010001
	tmp014ExceptionBreakpoint    = 0x80000003
)

// tmp014DebugEvent is DEBUG_EVENT on 64-bit Windows: the event code, the
// process and thread, and the union of event records, which is aligned to
// eight bytes and 160 bytes long.
type tmp014DebugEvent struct {
	Code      uint32
	ProcessID uint32
	ThreadID  uint32
	_         uint32
	Union     [160]byte
}

// tmp014RunUnderDebugger runs the child half as a debuggee of this test: the
// process is created with DEBUG_ONLY_THIS_PROCESS and its debug events are
// pumped until it exits. No breakpoint of any kind is set, and every exception
// but the loader's first breakpoint goes back to the process.
func tmp014RunUnderDebugger(t *testing.T) tmp014Reading {
	t.Helper()
	if runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
		t.Skipf("tmp014DebugEvent is laid out for 64-bit Windows, not %s", runtime.GOARCH)
	}
	// WaitForDebugEvent answers only on the thread that created the debuggee.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	kernel := syscall.NewLazyDLL("kernel32.dll")
	waitForDebugEvent := kernel.NewProc("WaitForDebugEvent")
	continueDebugEvent := kernel.NewProc("ContinueDebugEvent")

	var output bytes.Buffer
	child := exec.Command(os.Args[0], "-test.run=^TestDebugStatusTMP014Child$", "-test.count=1",
		"-securitystatus.tmp014child")
	child.Stdout = &output
	child.Stderr = &output
	child.SysProcAttr = &syscall.SysProcAttr{CreationFlags: tmp014DebugOnlyThisProcess}
	if err := child.Start(); err != nil {
		t.Fatalf("start the debuggee: %v", err)
	}

	deadline := time.Now().Add(tmp014ChildLimit)
	sawLoaderBreakpoint := false
	for {
		if time.Now().After(deadline) {
			_ = child.Process.Kill()
			t.Fatalf("the debuggee did not finish within %v", tmp014ChildLimit)
		}
		var event tmp014DebugEvent
		if got, _, _ := waitForDebugEvent.Call(uintptr(unsafe.Pointer(&event)), 1000); got == 0 {
			continue
		}
		status := uintptr(tmp014DBGContinue)
		switch event.Code {
		case tmp014ExceptionDebugEvent:
			if binary.LittleEndian.Uint32(event.Union[0:4]) == tmp014ExceptionBreakpoint && !sawLoaderBreakpoint {
				sawLoaderBreakpoint = true
			} else {
				status = tmp014DBGExceptionNotHandled
			}
		case tmp014CreateProcessEvent, tmp014LoadDLLEvent:
			// Both records start with an open handle to the image file.
			if handle := binary.LittleEndian.Uint64(event.Union[0:8]); handle != 0 {
				_ = syscall.CloseHandle(syscall.Handle(handle))
			}
		}
		continueDebugEvent.Call(uintptr(event.ProcessID), uintptr(event.ThreadID), status)
		if event.Code == tmp014ExitProcessEvent && int(event.ProcessID) == child.Process.Pid {
			break
		}
	}
	if err := child.Wait(); err != nil {
		t.Fatalf("the debuggee failed: %v\n%s", err, output.String())
	}
	return tmp014ParseChild(t, output.String())
}

// M26-TMP-014 on Windows, with a debugger attached for real. debug_status said
// confidence 100 and indicators [] while security_diagnostics listed the four
// checks that fired, and hardware_breakpoint and syscall each reported that
// list as their own finding though this debugger sets no breakpoint at all.
func TestDebugStatusTMP014AttachedDebuggerWindows(t *testing.T) {
	if *tmp014Child {
		t.Skip("running as the child")
	}
	reading := tmp014RunUnderDebugger(t)
	if !reading.Detected {
		t.Errorf("debug_status did not detect the debugger this test attached: %+v", reading)
	}
	if !tmp014Has(reading.Indicators, "windows:is_debugger_present") {
		t.Errorf("debug_status indicators=%v leave out windows:is_debugger_present, which the detector reports as %v", reading.Indicators, reading.Methods)
	}
	tmp014CheckIndicatorsAreTheMethods(t, reading)
	if reading.Confidence != 100 {
		t.Errorf("debug_status confidence=%d with a debugger attached, want 100", reading.Confidence)
	}
	tmp014CheckProbesDoNotEchoTheDetector(t, reading)
}

// M26-TMP-014 on Windows, with one heuristic: a debugger's DLL loaded in the
// process. The Windows detector does not report a debugger on that alone, and
// debug_status said indicators [] although the detector names the check.
func TestDebugStatusTMP014LoneDebuggerDLLWindows(t *testing.T) {
	if *tmp014Child {
		t.Skip("running as the child")
	}
	dll, err := syscall.LoadLibrary("dbghelp.dll")
	if err != nil {
		t.Skipf("dbghelp.dll cannot be loaded here: %v", err)
	}
	t.Cleanup(func() { _ = syscall.FreeLibrary(dll) })

	reading := tmp014Read(t)
	// With a debugger attached, the detector stops at the direct checks and
	// never looks for the DLL.
	if tmp014AnyDirect(reading.Methods) || tmp014AnyDirect(reading.Indicators) {
		t.Skipf("a debugger is attached to this test run, so the lone heuristic cannot be seen: %v", reading.Methods)
	}
	// Held to debug_status's own reading, not to Methods: the OutputDebugString
	// timing check can fire in one of the two readings and not the other.
	tmp014CheckHeuristicConfidence(t, reading, "windows:debugger_dll_loaded")
}
