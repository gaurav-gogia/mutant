package security

import (
	"runtime"
	"strings"
)

// debuggerProcessNameMatches reports whether a parent process is one of the
// named debuggers, comparing the base name of its executable exactly.
//
// It used to be a substring search. On Linux it ran over the parent's whole
// command line, so the two-letter name "rr" matched any parent whose arguments
// held an ordinary word containing it -- "current", "terraform", "error" -- and
// in secure mode a match terminates the run, which meant
// `mutant cases/current/triage.mu` stopped itself (M26-TMP-002). The macOS
// reader had the same shape over the output of `ps -o comm=`, which on that
// platform is a path and not a bare name, so every tool under
// /Applications/Xcode.app matched the entry "xcode" (M26-TMP-019).
//
// Exactness costs little that the list meant to catch, because the entries are
// the binaries themselves: gdb, lldb, valgrind, dtrace, sample, leaks, vmmap.
// What it does give up is a variant spelling -- gdbserver, or a versioned
// gdb-12 -- and that is the right trade here, because this is the weaker of
// the two signals each platform collects. A debugger that is actually attached
// is found by the TracerPid line on Linux and the P_TRACED flag on macOS,
// neither of which depends on a name at all. This check only guesses from a
// parent, so it is the one that should err towards silence.
//
// One consequence is worth stating: an entry that could only ever match as a
// substring now matches nothing. "trace" no longer stands in for strace and
// ltrace, both of which set TracerPid when they are tracing and are caught
// there instead.
func debuggerProcessNameMatches(raw string, names []string) bool {
	name := processBaseName(raw)
	if name == "" {
		return false
	}
	for _, candidate := range names {
		if name == candidate {
			return true
		}
	}
	return false
}

// processBaseName reduces whatever a platform reports for a process -- a bare
// name from /proc/<pid>/comm, which is NUL-padded and newline-terminated, a
// full path from macOS's ps, a Windows image name -- to the lower-case base
// name of the executable with any .exe suffix off it.
func processBaseName(raw string) string {
	name := strings.Trim(strings.TrimSpace(raw), "\x00")
	if i := strings.IndexAny(name, "\r\n"); i >= 0 {
		name = name[:i]
	}
	name = strings.ToLower(strings.TrimSpace(name))
	if i := strings.LastIndexAny(name, "/\\"); i >= 0 {
		name = name[i+1:]
	}
	return strings.TrimSuffix(name, ".exe")
}

// The two parent-process lists live here, beside the matcher that reads them,
// and not in the files that fetch the parent's name. They are data rather than
// platform code, and keeping them here is what lets both platforms' rules be
// tested from one host -- which is the thing that had never happened, and the
// reason both bugs lasted as long as they did.
//
// Each platform keeps its own list because the tools are different: there is no
// vmmap on Linux and no rr on macOS. Every entry is the base name of an
// executable and is compared exactly, so an entry with a path separator or a
// space in it would match nothing; on Linux it must also fit inside the fifteen
// characters the kernel allows /proc/<pid>/comm.
var (
	linuxDebuggerProcessNames = []string{
		"gdb", "lldb", "valgrind",
		"radare2", "ida", "ghidra", "angr", "frida",
		"rr", "pernosco",
	}

	darwinDebuggerProcessNames = []string{
		"lldb", "gdb", "xcode", "simulator", "instruments",
		"dtrace", "fs_usage", "sample", "trace", "sc_usage",
		"leaks", "malloc_history", "heap", "vmmap",
		"frida-server", "idb", "appium",
	}
)

// IsDebuggerPresent checks if a debugger is currently attached to the process.
// It uses multiple platform-specific detection techniques:
//
// Windows:
//   - IsDebuggerPresent API (checks PEB BeingDebugged flag)
//   - CheckRemoteDebuggerPresent API
//   - NtQueryInformationProcess with ProcessDebugPort
//   - NtQueryInformationProcess with ProcessDebugObjectHandle
//   - OutputDebugString timing test
//   - Parent process name analysis
//   - Common debugger DLL detection
//
// Linux:
//   - TracerPid from /proc/self/status
//   - ptrace self-attachment test
//   - Parent process name analysis
//   - LD_PRELOAD detection
//
// macOS:
//   - sysctl P_TRACED flag
//   - LLDB environment variables
//   - DYLD_INSERT_LIBRARIES detection
//
// Returns true if any debugger detection method triggers.
func IsDebuggerPresent() bool {
	detected, methods := DetectDebuggerDetails()
	securityDevLogf("debugger detected=%t methods=%s", detected, strings.Join(methods, ","))
	return detected
}

// DetectDebuggerDetails returns debugger detection status plus triggered methods.
func DetectDebuggerDetails() (bool, []string) {
	switch runtime.GOOS {
	case "windows":
		return detectDebuggerDetailsWindows()
	case "linux":
		return detectDebuggerDetailsLinux()
	case "darwin":
		return detectDebuggerDetailsDarwin()
	default:
		return false, nil
	}
}

// DebuggerTamperDetail reports which debugger checks fired, for a run that is
// stopping because of one.
//
// It probes again rather than reusing the gate's answer. Unlike the sandbox
// detector, debugger detection is deliberately not cached: a process that
// started clean can have a debugger attached to it later, so a cached "no"
// would be wrong for the rest of the run. The gate's answer is what terminates;
// this is a second reading taken to describe it, and it reports nothing if the
// debugger detached in between. (M-7)
func DebuggerTamperDetail() TamperDetail {
	detected, methods := DetectDebuggerDetails()
	if !detected {
		return TamperDetail{Detector: "debugger"}
	}

	return TamperDetail{Detector: "debugger", Signals: methods}
}

// debuggerEnvironmentMarkerSet reports whether any of names holds a value in
// the environment getenv reads. It is the rule the Linux and macOS
// environment checks share; each keeps its own list of names, and the rule
// lives here so that it can be run from any host.
//
// Being present used to be enough, so an empty LD_PRELOAD counted as a
// debugger attached to this process, and in secure mode that ends the run
// (M26-TMP-010). An empty variable holds no value, and nor does one of the
// loader's lists (loaderListSeparators) made only of separators: it gives the
// loader nothing to use, and it is what LD_PRELOAD=$LD_PRELOAD:$EXTRA leaves
// when both are empty.
func debuggerEnvironmentMarkerSet(names []string, getenv func(string) string) bool {
	for _, name := range names {
		if strings.Trim(getenv(name), loaderListSeparators[name]) != "" {
			return true
		}
	}
	return false
}

// loaderListSeparators holds, for each dynamic-loader variable on
// linuxDebuggerEnvVars or darwinDebuggerEnvVars whose value is a list of
// paths, the characters the loader splits that value at. glibc's ld.so splits
// LD_PRELOAD at spaces and colons and LD_AUDIT at colons; dyld splits
// DYLD_INSERT_LIBRARIES, the libraries it inserts, at colons. Any other
// character can be part of a path, so a value with one in it names something
// for the loader to use. A variable with no entry here is not a list, and any
// value it holds counts.
//
// DYLD_ROOT_PATH has no entry. dyld has read it only in a simulator process
// since macOS 12 (dyld-940), and there as one path rather than a list; Go has
// required macOS 12 or later since Go 1.25.
var loaderListSeparators = map[string]string{
	"LD_PRELOAD":            " :",
	"LD_AUDIT":              ":",
	"DYLD_INSERT_LIBRARIES": ":",
}
