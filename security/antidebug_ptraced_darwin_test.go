//go:build darwin
// +build darwin

package security

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"unsafe"

	"golang.org/x/sys/unix"
)

// tmp011TraceeArg, after "--" on the test binary's command line, makes
// TestDarwinTracedCheckTraceeHelper the process that
// TestDarwinTracedCheckSeesATracer attaches to.
const tmp011TraceeArg = "tmp011-tracee"

// TestDarwinTracedCheckMeasuresThisProcess is M26-TMP-011 on a Mac. The check
// asked the kernel with a six-element MIB, which xnu refuses with EINVAL, and
// returned the refusal as "not traced": it could not find a debugger, and
// nothing said why. It asks with the four-element kern.proc.pid.<pid> now, and
// a refusal comes back as an error.
func TestDarwinTracedCheckMeasuresThisProcess(t *testing.T) {
	traced, err := isTracedDarwin()
	if err != nil {
		t.Fatalf("the P_TRACED check did not measure this process: %v", err)
	}
	// Whether a debugger is attached depends on how the test was started, so
	// the answer is reported rather than asserted.
	t.Logf("P_TRACED for this test process: %v", traced)
}

// TestDarwinTracedCheckSeesATracer is the case the check exists for, and the
// one it could never report: a process with a tracer attached. It starts a
// child, reads the child's P_TRACED, attaches to the child with ptrace, and
// reads it again.
//
// macOS may refuse the attach -- its security policy has a say in every ptrace
// attach -- and the test then skips, naming the refusal, rather than failing: a
// host that will not let a test trace its own child has said nothing about the
// check.
func TestDarwinTracedCheckSeesATracer(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("finding the test binary: %v", err)
	}
	child := exec.Command(exe, "-test.run=^TestDarwinTracedCheckTraceeHelper$", "--", tmp011TraceeArg)
	stdin, err := child.StdinPipe()
	if err != nil {
		t.Fatalf("a stdin pipe for the child: %v", err)
	}
	stdout, err := child.StdoutPipe()
	if err != nil {
		t.Fatalf("a stdout pipe for the child: %v", err)
	}
	if err := child.Start(); err != nil {
		t.Fatalf("starting the child: %v", err)
	}
	t.Cleanup(func() {
		_ = child.Process.Kill()
		_ = stdin.Close()
		_ = child.Wait()
	})

	// The child says it is running before anything reads its record, so a
	// child that never reached the helper fails here instead of passing for
	// one that cannot be traced.
	ready := bufio.NewScanner(stdout)
	if !ready.Scan() || ready.Text() != tmp011TraceeArg+" ready" {
		t.Fatalf("the child did not report ready: read %q, error %v", ready.Text(), ready.Err())
	}
	pid := child.Process.Pid

	traced, err := darwinProcessTraced(pid)
	if err != nil {
		t.Fatalf("the check did not measure the child before the attach: %v", err)
	}
	if traced {
		t.Fatal("the check says the child is traced before anything has attached to it")
	}

	if err := unix.PtraceAttach(pid); err != nil {
		t.Skipf("this host refused to let the test trace its own child (%v), so the attached case was not measured", err)
	}
	// PT_ATTACH marks the child traced and stops it with SIGSTOP. Waiting for
	// that stop leaves the child where an attached debugger holds it before
	// the flag is read again.
	var status syscall.WaitStatus
	if _, err := syscall.Wait4(pid, &status, 0, nil); err != nil {
		t.Fatalf("waiting for the traced child to stop: %v", err)
	}
	if !status.Stopped() {
		t.Fatalf("the child did not stop after PT_ATTACH: wait status %#x", uint32(status))
	}

	traced, err = darwinProcessTraced(pid)
	if err != nil {
		t.Fatalf("the check did not measure the traced child: %v", err)
	}
	if !traced {
		t.Fatal("the check says a child with a tracer attached is not traced")
	}
}

// TestDarwinTracedCheckTraceeHelper is the child that
// TestDarwinTracedCheckSeesATracer traces. Run in any other way, it skips.
func TestDarwinTracedCheckTraceeHelper(t *testing.T) {
	if args := flag.Args(); len(args) != 1 || args[0] != tmp011TraceeArg {
		t.Skip("the child of TestDarwinTracedCheckSeesATracer; it does nothing on its own")
	}
	fmt.Println(tmp011TraceeArg, "ready")
	// Wait for the parent to finish with this process. It kills it. If the
	// parent exits first, stdin closes and this returns, unless the parent had
	// attached to this process: then the kernel kills it.
	_, _ = io.Copy(io.Discard, os.Stdin)
}

// TestDarwinKinfoProcIsTheKernelsLayout pins the record the check reads to the
// layout <sys/sysctl.h> and <sys/proc.h> give a 64-bit process: kinfo_proc is
// 648 bytes and starts with extern_proc, whose p_flag is 32 bytes in and p_pid
// 40. The struct the check used to declare for itself put p_flag 340 bytes into
// 968.
func TestDarwinKinfoProcIsTheKernelsLayout(t *testing.T) {
	for _, c := range []struct {
		field     string
		got, want uintptr
	}{
		{"sizeof(struct kinfo_proc)", unsafe.Sizeof(unix.KinfoProc{}), 648},
		{"offsetof(struct kinfo_proc, kp_proc)", unsafe.Offsetof(unix.KinfoProc{}.Proc), 0},
		{"offsetof(struct extern_proc, p_flag)", unsafe.Offsetof(unix.KinfoProc{}.Proc.P_flag), 32},
		{"offsetof(struct extern_proc, p_pid)", unsafe.Offsetof(unix.KinfoProc{}.Proc.P_pid), 40},
	} {
		if c.got != c.want {
			t.Errorf("%s = %d, want %d", c.field, c.got, c.want)
		}
	}
}
