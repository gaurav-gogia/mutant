package builtin

import (
	"encoding/json"
	"flag"
	"fmt"
	"sort"
	"strings"
	"testing"

	"mutant/object"
	"mutant/security"
)

// tmp014Child is set only when one of the M26-TMP-014 tests runs this test
// binary again -- under a debugger, or under a parent named like one -- so that
// debug_status is read from inside that process. A flag and not an environment
// variable, for the reason recordCrashChild gives.
var tmp014Child = flag.Bool("securitystatus.tmp014child", false, "internal: print what debug_status reports, as one JSON line")

// tmp014Marker starts the one line the child prints.
const tmp014Marker = "TMP014-DEBUG-STATUS "

// tmp014Reading is what one process saw: debug_status's verdict, confidence
// and indicators; the checks the debugger detector reports as fired, read
// separately; and every probe debug_status ran, by name, with its detail.
type tmp014Reading struct {
	Detected   bool              `json:"detected"`
	Confidence int64             `json:"confidence"`
	Indicators []string          `json:"indicators"`
	Methods    []string          `json:"methods"`
	Probes     map[string]string `json:"probes"`
}

// TestDebugStatusTMP014Child is the child half of the M26-TMP-014 tests that
// need a process of their own: it prints what debug_status reports there.
func TestDebugStatusTMP014Child(t *testing.T) {
	if !*tmp014Child {
		t.Skip("the child half of the M26-TMP-014 debug_status tests")
	}
	line, err := json.Marshal(tmp014Read(t))
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(tmp014Marker + string(line))
}

// tmp014Read calls debug_status in this process and reads the detector's own
// list of the checks that fired beside it.
func tmp014Read(t *testing.T) tmp014Reading {
	t.Helper()
	payload, errObj := unwrapPair(t, DebugStatus())
	if errObj != nil {
		t.Fatalf("debug_status: %s", errObj.Inspect())
	}
	status, ok := payload.(*object.Hash)
	if !ok {
		t.Fatalf("debug_status returned %T, not a hash", payload)
	}
	_, methods := security.DetectDebuggerDetails()
	reading := tmp014Reading{
		Detected:   hashValueByKey(status, "detected").(*object.Boolean).Value,
		Confidence: hashValueByKey(status, "confidence").(*object.Integer).Value,
		Indicators: tmp014Strings(t, hashValueByKey(status, "indicators")),
		Methods:    append([]string{}, methods...),
		Probes:     tmp014ProbeDetails(t, status),
	}
	sort.Strings(reading.Methods)
	return reading
}

// tmp014ParseChild finds the child's reading in what it printed.
func tmp014ParseChild(t *testing.T, output string) tmp014Reading {
	t.Helper()
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, tmp014Marker) {
			continue
		}
		var reading tmp014Reading
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, tmp014Marker)), &reading); err != nil {
			t.Fatalf("the child's reading does not parse: %v\n%s", err, line)
		}
		return reading
	}
	t.Fatalf("the child printed no reading:\n%s", output)
	return tmp014Reading{}
}

func tmp014Strings(t *testing.T, value object.Object) []string {
	t.Helper()
	array, ok := value.(*object.Array)
	if !ok {
		t.Fatalf("want an array, got %T", value)
	}
	out := []string{}
	for _, element := range array.Elements {
		text, ok := element.(*object.String)
		if !ok {
			t.Fatalf("want an array of strings, got an element of %T", element)
		}
		out = append(out, text.Value)
	}
	return out
}

// tmp014ProbeDetails maps the name of every probe a status builtin ran to the
// detail it reported.
func tmp014ProbeDetails(t *testing.T, status *object.Hash) map[string]string {
	t.Helper()
	signals, ok := hashValueByKey(status, "probe_signals").(*object.Array)
	if !ok {
		t.Fatal("probe_signals is not an array")
	}
	details := map[string]string{}
	for _, element := range signals.Elements {
		row := element.(*object.Hash)
		details[hashValueByKey(row, "name").(*object.String).Value] = hashValueByKey(row, "detail").(*object.String).Value
	}
	return details
}

func tmp014Has(list []string, want string) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}

// tmp014DirectMethods are the debugger checks that read the operating system's
// own record of a debugger, as docs/ANTITAMPER_PROBE.md lists them. Every other
// check the detectors make is a heuristic.
var tmp014DirectMethods = map[string]bool{
	"windows:is_debugger_present":                true,
	"windows:check_remote_debugger_present":      true,
	"windows:process_debug_port":                 true,
	"windows:process_debug_object_handle":        true,
	"windows:system_debug_control_query_version": true,
	"linux:tracer_pid":                           true,
	"darwin:p_traced":                            true,
}

// tmp014WantConfidence is the confidence the documentation gives a set of
// indicators: 100 when a direct check fired, otherwise 35 for each heuristic
// and 70 at most.
func tmp014WantConfidence(indicators []string) int64 {
	heuristics := int64(0)
	for _, method := range indicators {
		if tmp014DirectMethods[method] {
			return 100
		}
		heuristics++
	}
	if heuristics > 2 {
		heuristics = 2
	}
	return 35 * heuristics
}

func tmp014AnyDirect(indicators []string) bool {
	for _, method := range indicators {
		if tmp014DirectMethods[method] {
			return true
		}
	}
	return false
}

// tmp014Reporter is what tmp014CheckHeuristicConfidence needs of a
// *testing.T, so that the check can also be run on a made-up reading and what
// it reports collected (TestDebugStatusTMP014HeuristicCheckUsesOneReading).
type tmp014Reporter interface {
	Helper()
	Errorf(format string, args ...any)
}

// tmp014CheckHeuristicConfidence checks debug_status in a process where the
// test set up one heuristic -- a debugger's name on the parent process, a
// debugger's DLL loaded -- and no direct check fired. It holds debug_status to
// its own reading and to nothing else. Methods, the detector's list read again
// a moment later, can name other checks: on Windows the OutputDebugString check
// times one call with GetTickCount, so whether it fires is a matter of timing,
// and a reading of 70 from two heuristics can sit beside a later list that
// names one. Both are right.
func tmp014CheckHeuristicConfidence(r tmp014Reporter, reading tmp014Reading, heuristic string) {
	r.Helper()
	if !tmp014Has(reading.Indicators, heuristic) {
		r.Errorf("debug_status indicators=%v leave out %s, which the detector reports as %v", reading.Indicators, heuristic, reading.Methods)
	}
	if want := tmp014WantConfidence(reading.Indicators); reading.Confidence != want {
		r.Errorf("debug_status confidence=%d for indicators %v, want %d", reading.Confidence, reading.Indicators, want)
	}
	// The heuristic was in place for the whole of debug_status's reading and
	// no direct check fired, so the reading comes to 35 or 70: not 0, and not
	// the 100 that only a direct check gives.
	if reading.Confidence < 35 || reading.Confidence >= 100 {
		r.Errorf("debug_status confidence=%d from heuristics alone (%s fired), want 35 or 70", reading.Confidence, heuristic)
	}
}

// tmp014Recorder collects what a reading check reports, instead of failing.
type tmp014Recorder struct{ reports []string }

func (r *tmp014Recorder) Helper() {}

func (r *tmp014Recorder) Errorf(format string, args ...any) {
	r.reports = append(r.reports, fmt.Sprintf(format, args...))
}

// The heuristic check holds debug_status to its own reading. The first case is
// the pair that failed TestDebugStatusTMP014LoneDebuggerDLLWindows while that
// test held the confidence to Methods: debug_status's reading named the DLL and
// the OutputDebugString timing and came to 70, and the detector, read again,
// named the DLL alone. Both readings were right. The second is the same pair
// the other way round. The rest are wrong readings, which the check must still
// report -- the last of them one that a check against Methods would pass.
func TestDebugStatusTMP014HeuristicCheckUsesOneReading(t *testing.T) {
	const dll, timing = "windows:debugger_dll_loaded", "windows:output_debug_string_timing"
	cases := []struct {
		name    string
		reading tmp014Reading
		wrong   bool
	}{
		{"two heuristics, one in the later list", tmp014Reading{Detected: true, Confidence: 70, Indicators: []string{dll, timing}, Methods: []string{dll}}, false},
		{"one heuristic, two in the later list", tmp014Reading{Confidence: 35, Indicators: []string{dll}, Methods: []string{dll, timing}}, false},
		{"one heuristic at 100", tmp014Reading{Detected: true, Confidence: 100, Indicators: []string{dll}, Methods: []string{dll}}, true},
		{"the heuristic left out", tmp014Reading{Confidence: 0, Indicators: []string{}, Methods: []string{dll}}, true},
		{"70 from one heuristic, two in the later list", tmp014Reading{Confidence: 70, Indicators: []string{dll}, Methods: []string{dll, timing}}, true},
	}
	for _, c := range cases {
		var recorder tmp014Recorder
		tmp014CheckHeuristicConfidence(&recorder, c.reading, dll)
		switch reported := len(recorder.reports) > 0; {
		case reported && !c.wrong:
			t.Errorf("%s: the check fails a right reading %+v: %q", c.name, c.reading, recorder.reports)
		case !reported && c.wrong:
			t.Errorf("%s: the check passes a wrong reading %+v", c.name, c.reading)
		}
	}
}

// tmp014CheckProbesDoNotEchoTheDetector fails for any probe whose detail
// carries one of the detector's own method names: a probe that re-reports the
// debugger detector under a name of its own is what M26-TMP-014 found
// hardware_breakpoint and syscall doing.
func tmp014CheckProbesDoNotEchoTheDetector(t *testing.T, reading tmp014Reading) {
	t.Helper()
	for name, detail := range reading.Probes {
		for _, method := range reading.Methods {
			if strings.Contains(detail, method) {
				t.Errorf("probe %s reports the debugger detector's %s as its own finding: %q", name, method, detail)
			}
		}
	}
}

// tmp014CheckIndicatorsAreTheMethods fails when debug_status's indicators are
// not the checks the detector says fired.
func tmp014CheckIndicatorsAreTheMethods(t *testing.T, reading tmp014Reading) {
	t.Helper()
	if strings.Join(reading.Indicators, ",") != strings.Join(reading.Methods, ",") {
		t.Errorf("debug_status indicators=%v, but the detector reports the checks %v as fired", reading.Indicators, reading.Methods)
	}
}

// M26-TMP-014. Four probes carried the names of measurements they never made:
// hardware_breakpoint and syscall re-ran the debugger detector and reported its
// verdict under their own names, cpuid_hypervisor reported the sandbox
// detector's, and rdtsc_drift timed three sleeps on the OS clock and made no
// RDTSC check. The first three are gone -- debug_status and sandbox_status
// report those detectors in their own fields -- and the fourth is called
// sleep_drift.
func TestStatusProbesTMP014RetiredAndRenamed(t *testing.T) {
	if *tmp014Child {
		t.Skip("running as the child")
	}
	debugPayload, errObj := unwrapPair(t, DebugStatus())
	if errObj != nil {
		t.Fatalf("debug_status: %s", errObj.Inspect())
	}
	debugProbes := tmp014ProbeDetails(t, debugPayload.(*object.Hash))
	for _, retired := range []string{"hardware_breakpoint", "syscall"} {
		if detail, ok := debugProbes[retired]; ok {
			t.Errorf("debug_status still runs a probe named %s, which measures no such thing (detail %q)", retired, detail)
		}
	}

	sandboxPayload, errObj := unwrapPair(t, SandboxStatus())
	if errObj != nil {
		t.Fatalf("sandbox_status: %s", errObj.Inspect())
	}
	sandboxProbes := tmp014ProbeDetails(t, sandboxPayload.(*object.Hash))
	for _, retired := range []string{"cpuid_hypervisor", "rdtsc_drift"} {
		if detail, ok := sandboxProbes[retired]; ok {
			t.Errorf("sandbox_status still runs a probe named %s, which measures no such thing (detail %q)", retired, detail)
		}
	}
	detail, ok := sandboxProbes["sleep_drift"]
	switch {
	case !ok:
		t.Errorf("sandbox_status runs no sleep_drift probe; it ran %v", sandboxProbes)
	case !strings.HasPrefix(detail, "sleep_ms="):
		t.Errorf("sleep_drift does not report the sleeps it timed: %q", detail)
	}
}

// Whatever the host, debug_status's confidence follows from its indicators as
// documented, and a detection names at least one of them.
func TestDebugStatusTMP014ConfidenceFollowsIndicators(t *testing.T) {
	if *tmp014Child {
		t.Skip("running as the child")
	}
	reading := tmp014Read(t)
	if want := tmp014WantConfidence(reading.Indicators); reading.Confidence != want {
		t.Errorf("debug_status confidence=%d for indicators %v, want %d", reading.Confidence, reading.Indicators, want)
	}
	if reading.Detected && len(reading.Indicators) == 0 {
		t.Errorf("debug_status detected a debugger and names no check that found it")
	}
}
