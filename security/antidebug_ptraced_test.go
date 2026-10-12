package security

import "testing"

// TestDarwinTracedFlagIsReadFromTheRecordAskedFor is M26-TMP-011 as far as it
// can run away from a Mac: the decision the macOS P_TRACED check makes from the
// two fields of the kernel's kinfo_proc that it reads.
//
// The check used to ask for that record with a MIB the kernel refuses, and to
// read p_flag 340 bytes into a record that holds it 32 bytes in, so it never saw
// the bit. The request and the layout are x/sys's now. The darwin-only tests in
// antidebug_ptraced_darwin_test.go pin the layout and exercise the request, and
// those run only on macOS.
//
// The flag words below are built from the bits <sys/proc.h> defines. The first
// three are words a real process can carry: P_LP64, which every arm64 and amd64
// process has, with P_EXEC, which exec sets; that pair with P_CONTROLT added;
// and that pair with P_TRACED added. The rest are edge cases of the decision,
// and no arm64 or amd64 process carries P_TRACED alone or no flags at all.
// P_TRACED is written out as its number rather than as darwinPTraced, so a
// wrong constant fails here instead of agreeing with itself.
func TestDarwinTracedFlagIsReadFromTheRecordAskedFor(t *testing.T) {
	const pid = 4242

	const (
		pControlT    = 0x00000002 // P_CONTROLT: has a controlling terminal
		pLP64        = 0x00000004 // P_LP64: every arm64 and amd64 process
		pTimeout     = 0x00000400 // P_TIMEOUT: the bit below P_TRACED
		pTraced      = 0x00000800 // P_TRACED: a debugger is tracing the process
		pDisableASLR = 0x00001000 // P_DISABLE_ASLR: the bit above P_TRACED
		pExec        = 0x00004000 // P_EXEC: the process has called exec
	)

	cases := []struct {
		recordPID int32
		flag      int32
		traced    bool
		measured  bool
		why       string
	}{
		{pid, pLP64 | pExec, false, true, "an ordinary 64-bit process"},
		{pid, pLP64 | pExec | pControlT, false, true, "the same, started from a terminal"},
		{pid, pLP64 | pExec | pTraced, true, true, "the same, with a debugger attached"},
		{pid, pTraced, true, true, "P_TRACED alone"},
		{pid, 0, false, true, "no flags at all"},
		{pid, pLP64 | pExec | pTimeout | pDisableASLR, false, true,
			"both neighbours of P_TRACED set, and P_TRACED clear"},
		{pid + 1, pLP64 | pExec | pTraced, false, false,
			"another process's record says nothing about this one"},
		{0, 0, false, false, "a zeroed record names pid 0, not the process asked about"},
	}

	for _, tc := range cases {
		traced, err := darwinTracedFromRecord(pid, tc.recordPID, tc.flag)
		if measured := err == nil; measured != tc.measured {
			t.Errorf("%s: measured = %v (error: %v), want %v", tc.why, measured, err, tc.measured)
			continue
		}
		if traced != tc.traced {
			t.Errorf("%s: traced = %v, want %v", tc.why, traced, tc.traced)
		}
	}
}
