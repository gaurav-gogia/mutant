package security

import "strings"

// directDebuggerMethods are the debugger checks that read the operating
// system's own record of a debugger. debug_status's confidence follows from
// whether one of them fired; it used to be 100 for any detection at all, so a
// parent process that happened to be named gdb was reported as surely as a
// debugger the kernel says is attached (M26-TMP-014).
//
// A direct check asks the system: the PEB's BeingDebugged flag
// (IsDebuggerPresent), CheckRemoteDebuggerPresent, the process's debug port and
// debug object, whether a kernel debugger answers, TracerPid on Linux and
// P_TRACED on macOS. The first five are the Windows detector's high-confidence
// checks, any one of which is enough for it; the last two are the checks on
// Linux and macOS that read the kernel's record of an attached tracer instead
// of guessing (see debuggerProcessNameMatches).
//
// Every other check is a heuristic, a guess from something a debugger tends to
// leave behind and other things leave too: a debugger's name on the parent
// process, a debugger's environment variable, an OutputDebugString call that
// took long, a debugger's DLL in the process.
//
// The names are the methods the platform detectors report. Those detectors live
// in files that build only on their own platform, so the names are written out
// here, where one host can classify all three. A method this set does not name
// counts as a heuristic: a check added or renamed without being classified here
// is understated, never overstated.
var directDebuggerMethods = map[string]bool{
	"windows:is_debugger_present":                true,
	"windows:check_remote_debugger_present":      true,
	"windows:process_debug_port":                 true,
	"windows:process_debug_object_handle":        true,
	"windows:system_debug_control_query_version": true,
	"linux:tracer_pid":                           true,
	"darwin:p_traced":                            true,
}

const (
	// debuggerDirectConfidence is the confidence of a reading in which a direct
	// check fired: the system says a debugger is there.
	debuggerDirectConfidence = maxConfidenceScore
	// debuggerHeuristicConfidence is what each heuristic that fired adds, when
	// no direct check did.
	debuggerHeuristicConfidence = 35
	// debuggerHeuristicsCounted is how many heuristics count towards the
	// confidence: two, as many as the Windows detector wants before it reports
	// a debugger on heuristics. However many fire, heuristics alone come to
	// 70, short of the certainty only a direct check gives.
	debuggerHeuristicsCounted = 2
)

// debuggerConfidence is how much the debugger checks that fired prove, on the
// same 0-100 scale the sandbox detectors score on: 100 when a direct check
// fired; otherwise 35 for each distinct heuristic, two at most, so 35 or 70;
// and 0 when nothing fired.
func debuggerConfidence(methods []string) int {
	heuristics := map[string]bool{}
	for _, method := range methods {
		if directDebuggerMethods[method] {
			return debuggerDirectConfidence
		}
		heuristics[method] = true
	}
	counted := len(heuristics)
	if counted > debuggerHeuristicsCounted {
		counted = debuggerHeuristicsCounted
	}
	return counted * debuggerHeuristicConfidence
}

// DebuggerStatus is one reading of the debugger detector for a caller that
// reports what it found rather than acting on it, which is debug_status: the
// detector's verdict, the checks that fired, and how much those checks prove
// (debuggerConfidence), all from the same reading. It logs the reading as
// IsDebuggerPresent does.
//
// The verdict is the platform detector's, by its own rule, and the confidence
// is this file's, so the two answer different questions. The Windows detector,
// for one, wants two heuristics before it reports a debugger, so one heuristic
// there reads as not detected with a confidence of 35.
func DebuggerStatus() (detected bool, methods []string, confidence int) {
	detected, methods = DetectDebuggerDetails()
	securityDevLogf("debugger detected=%t methods=%s", detected, strings.Join(methods, ","))
	return detected, methods, debuggerConfidence(methods)
}
