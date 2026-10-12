//go:build linux

package builtin

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
	"time"
)

// tmp014ChildLimit bounds how long the child may take before the test ends it.
const tmp014ChildLimit = 2 * time.Minute

// tmp014ChildArgs runs this test binary as the child half.
func tmp014ChildArgs() []string {
	return []string{"-test.run=^TestDebugStatusTMP014Child$", "-test.count=1", "-securitystatus.tmp014child"}
}

// M26-TMP-014 on Linux, with one heuristic: the parent process is named gdb.
// That is a guess from a name and no more, and debug_status reported it as
// confidence 100 with indicators [], while hardware_breakpoint and syscall each
// reported it as their own finding.
func TestDebugStatusTMP014ParentNamedLikeADebuggerLinux(t *testing.T) {
	if *tmp014Child {
		t.Skip("running as the child")
	}
	shell, err := os.ReadFile("/bin/sh")
	if err != nil {
		t.Skipf("no /bin/sh to run the child under: %v", err)
	}
	// A copy of the shell named gdb: the kernel names the process after the
	// file, and that name is all the parent check reads.
	gdb := filepath.Join(t.TempDir(), "gdb")
	if err := os.WriteFile(gdb, shell, 0o755); err != nil {
		t.Fatal(err)
	}
	// "; exit $?" keeps the shell from replacing itself with the child, so the
	// shell stays its parent. The two share a process group of their own, which
	// is what the time limit ends.
	args := append([]string{"-c", `"$0" "$@"; exit $?`, os.Args[0]}, tmp014ChildArgs()...)
	var output bytes.Buffer
	child := exec.Command(gdb, args...)
	child.Stdout = &output
	child.Stderr = &output
	child.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := child.Start(); err != nil {
		t.Skipf("this host will not run a program from the test's temporary directory: %v", err)
	}
	group := child.Process.Pid
	timer := time.AfterFunc(tmp014ChildLimit, func() { _ = syscall.Kill(-group, syscall.SIGKILL) })
	err = child.Wait()
	timer.Stop()
	if err != nil {
		t.Fatalf("the child failed: %v\n%s", err, output.String())
	}

	reading := tmp014ParseChild(t, output.String())
	if !tmp014Has(reading.Methods, "linux:parent_debugger_process") {
		t.Fatalf("the detector did not see the parent named gdb, so this test shows nothing: %+v", reading)
	}
	tmp014CheckIndicatorsAreTheMethods(t, reading)
	// A tracer on the child as well -- this test run under strace -f, say --
	// fires a direct check, and 100 is then right.
	if !tmp014AnyDirect(reading.Methods) && !tmp014AnyDirect(reading.Indicators) {
		tmp014CheckHeuristicConfidence(t, reading, "linux:parent_debugger_process")
	}
	tmp014CheckProbesDoNotEchoTheDetector(t, reading)
}

// M26-TMP-014 on Linux, with the child traced for real: this test is its
// tracer, so TracerPid in the child's /proc/self/status is not 0. That is the
// kernel's record of an attached tracer, and confidence 100 is right for it;
// the indicators must say so.
func TestDebugStatusTMP014TracedLinux(t *testing.T) {
	if *tmp014Child {
		t.Skip("running as the child")
	}
	// ptrace requests are answered only for the thread that started the tracee.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	out, err := os.Create(filepath.Join(t.TempDir(), "child.out"))
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	child := exec.Command(os.Args[0], tmp014ChildArgs()...)
	child.Stdout = out
	child.Stderr = out
	child.SysProcAttr = &syscall.SysProcAttr{Ptrace: true, Pdeathsig: syscall.SIGKILL}
	if err := child.Start(); err != nil {
		t.Skipf("this host does not let a test trace its child: %v", err)
	}
	pid := child.Process.Pid
	defer child.Process.Release()
	timer := time.AfterFunc(tmp014ChildLimit, func() { _ = syscall.Kill(pid, syscall.SIGKILL) })
	defer timer.Stop()
	// A test that fails part way leaves no stopped tracee behind.
	reaped := false
	defer func() {
		if !reaped {
			_ = syscall.Kill(pid, syscall.SIGKILL)
			var status syscall.WaitStatus
			_, _ = syscall.Wait4(pid, &status, 0, nil)
		}
	}()

	stoppedAtExec := false
	for {
		var status syscall.WaitStatus
		if _, err := syscall.Wait4(pid, &status, 0, nil); err != nil {
			if err == syscall.EINTR {
				continue
			}
			t.Fatalf("wait for the tracee: %v", err)
		}
		if status.Exited() || status.Signaled() {
			reaped = true
			if status.Signaled() || status.ExitStatus() != 0 {
				t.Fatalf("the tracee did not finish cleanly: %v", status)
			}
			break
		}
		if !status.Stopped() {
			continue
		}
		// The first stop is the SIGTRAP of the exec itself, which is not the
		// child's to receive; every later signal is passed on.
		signal := status.StopSignal()
		if !stoppedAtExec && signal == syscall.SIGTRAP {
			stoppedAtExec = true
			signal = 0
		}
		if err := syscall.PtraceCont(pid, int(signal)); err != nil {
			t.Fatalf("continue the tracee: %v", err)
		}
	}

	printed, err := os.ReadFile(out.Name())
	if err != nil {
		t.Fatal(err)
	}
	reading := tmp014ParseChild(t, string(printed))
	if !reading.Detected || !tmp014Has(reading.Methods, "linux:tracer_pid") {
		t.Fatalf("the detector did not see this test tracing the child, so this test shows nothing: %+v", reading)
	}
	if !tmp014Has(reading.Indicators, "linux:tracer_pid") {
		t.Errorf("debug_status indicators=%v leave out linux:tracer_pid, which the detector reports as %v", reading.Indicators, reading.Methods)
	}
	tmp014CheckIndicatorsAreTheMethods(t, reading)
	if reading.Confidence != 100 {
		t.Errorf("debug_status confidence=%d with a tracer attached, want 100", reading.Confidence)
	}
	tmp014CheckProbesDoNotEchoTheDetector(t, reading)
}
