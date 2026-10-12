package security

import "fmt"

// darwinPTraced is P_TRACED from macOS's <sys/proc.h>: the bit of p_flag, in
// the kinfo_proc that sysctl kern.proc.pid.<pid> returns, that the kernel sets
// while a debugger traces the process.
const darwinPTraced = 0x00000800

// darwinTracedFromRecord is the decision the macOS P_TRACED check makes from
// the two fields of a process's kinfo_proc that it reads: p_pid and p_flag.
//
// It is kept apart from the sysctl that fetches the record, and builds on every
// platform, so that what the check concludes can be tested on a host that is
// not a Mac. The request and the record exist only on macOS, where
// darwinProcessTraced (antidebug_darwin.go) makes the one and reads the other.
//
// A record for any process but the one asked about is an error, not an answer.
// The kernel returns the record of the process a request names, so another pid
// means the request was not the one intended, and its p_flag describes some
// other process.
func darwinTracedFromRecord(askedPID int, recordPID, flag int32) (bool, error) {
	if int(recordPID) != askedPID {
		return false, fmt.Errorf("the kernel's process record is for pid %d, not %d", recordPID, askedPID)
	}
	return flag&darwinPTraced != 0, nil
}
