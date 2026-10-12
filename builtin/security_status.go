package builtin

import (
	"sort"

	"mutant/object"
	"mutant/security"
)

func DebugStatus(args ...object.Object) object.Object {
	if len(args) != 0 {
		return resultAndError(nil, newError("wrong number of arguments. got=%d, want=0", len(args)))
	}

	// One reading gives the verdict, the checks that fired and what they
	// prove. The confidence used to be 100 for any detection and the checks
	// were dropped, so a parent process named gdb read as certainly as an
	// attached debugger, with nothing to say which it was (M26-TMP-014).
	detected, methods, confidence := security.DebuggerStatus()
	telemetry := security.SecurityTelemetrySnapshot()

	result := map[string]object.Object{
		"detected":       boolObj(detected),
		"type":           stringObj(detectionType(detected, "debugger")),
		"confidence":     intObj(int64(confidence)),
		"indicators":     stringArrayObj(methods),
		"probe_signals":  antiTamperSignalArrayObj(nil),
		"probe_enabled":  boolObj(false),
		"probe_error":    stringObj(""),
		"source":         stringObj("go-security"),
		"advisory":       boolObj(true),
		"event_count":    intObj(int64(telemetry["debugger_detected"])),
		"error":          stringObj(""),
		"schema_version": intObj(1),
	}

	// hardware_breakpoint and syscall were in this list and are not probes now:
	// each re-reported the debugger detector, whose checks are this builtin's
	// own indicators, under the name of a measurement it never made
	// (M26-TMP-014).
	probeSignals, probeEnabled, probeErr := security.RunAntiTamperProbe(
		[]string{"timing", "frida_ptrace", "trampoline", "iat_got", "process_injection", "module_integrity", "memory_page_anomaly"},
		"builtin:debug_status",
	)
	result["probe_enabled"] = boolObj(probeEnabled)
	result["probe_signals"] = antiTamperSignalArrayObj(probeSignals)
	if probeErr != nil {
		result["probe_error"] = stringObj(probeErr.Error())
	}

	return resultAndError(makeHashObject(result), nil)
}

func SandboxStatus(args ...object.Object) object.Object {
	if len(args) != 0 {
		return resultAndError(nil, newError("wrong number of arguments. got=%d, want=0", len(args)))
	}

	// An error no longer voids the detection: it names checks the detector
	// could not make, and the type, confidence and indicators are what the
	// others found. Both calls read one cached detection and return the same
	// error, so it is reported once. (M26-TMP-013)
	statusType, confidence, err := security.DetectSandboxType()
	indicators, _ := security.GetSandboxIndicators()

	detected := confidence >= security.SandboxDetectedThreshold
	if statusType == security.SandboxTypeNone {
		detected = false
	}

	errorMessage := ""
	if err != nil {
		errorMessage = err.Error()
	}

	telemetry := security.SecurityTelemetrySnapshot()
	result := map[string]object.Object{
		"detected":       boolObj(detected),
		"type":           stringObj(statusType),
		"confidence":     intObj(int64(confidence)),
		"indicators":     stringArrayObj(indicators),
		"probe_signals":  antiTamperSignalArrayObj(nil),
		"probe_enabled":  boolObj(false),
		"probe_error":    stringObj(""),
		"source":         stringObj("go-security"),
		"advisory":       boolObj(true),
		"event_count":    intObj(int64(telemetry["sandbox_detected"])),
		"error":          stringObj(errorMessage),
		"schema_version": intObj(1),
	}

	// cpuid_hypervisor was in this list and is not a probe now: it re-reported
	// the sandbox detector, which this builtin reports in its own fields, and
	// read no CPUID of its own. sleep_drift was called rdtsc_drift; it times
	// short sleeps on the OS clock and makes no RDTSC check (M26-TMP-014).
	probeSignals, probeEnabled, probeErr := security.RunAntiTamperProbe(
		[]string{"sleep_drift", "syscall_table", "module_integrity", "memory_page_anomaly"},
		"builtin:sandbox_status",
	)
	result["probe_enabled"] = boolObj(probeEnabled)
	result["probe_signals"] = antiTamperSignalArrayObj(probeSignals)
	if probeErr != nil {
		result["probe_error"] = stringObj(probeErr.Error())
	}

	return resultAndError(makeHashObject(result), nil)
}

func SecurityDiagnostics(args ...object.Object) object.Object {
	if len(args) != 0 {
		return resultAndError(nil, newError("wrong number of arguments. got=%d, want=0", len(args)))
	}

	debugDetected, debugMethods := security.DetectDebuggerDetails()
	sandboxType, sandboxConfidence, sandboxErr := security.DetectSandboxType()
	// As in SandboxStatus: the indicators come beside sandboxErr, which is
	// reported once. (M26-TMP-013)
	sandboxIndicators, _ := security.GetSandboxIndicators()

	sandboxDetected := sandboxConfidence >= security.SandboxDetectedThreshold
	if sandboxType == security.SandboxTypeNone {
		sandboxDetected = false
	}

	sandboxError := ""
	if sandboxErr != nil {
		sandboxError = sandboxErr.Error()
	}

	telemetry := security.SecurityTelemetrySnapshot()

	return resultAndError(makeHashObject(map[string]object.Object{
		"debugger": makeHashObject(map[string]object.Object{
			"detected":       boolObj(debugDetected),
			"methods":        stringArrayObj(debugMethods),
			"event_count":    intObj(int64(telemetry["debugger_detected"])),
			"schema_version": intObj(1),
		}),
		"sandbox": makeHashObject(map[string]object.Object{
			"detected":       boolObj(sandboxDetected),
			"type":           stringObj(sandboxType),
			"confidence":     intObj(int64(sandboxConfidence)),
			"indicators":     stringArrayObj(sandboxIndicators),
			"event_count":    intObj(int64(telemetry["sandbox_detected"])),
			"error":          stringObj(sandboxError),
			"schema_version": intObj(1),
		}),
		"source":         stringObj("go-security"),
		"schema_version": intObj(1),
	}), nil)
}

func detectionType(detected bool, name string) string {
	if detected {
		return name
	}
	return security.SandboxTypeNone
}

func makeHashObject(values map[string]object.Object) *object.Hash {
	pairs := make(map[object.HashKey]object.HashPair, len(values))
	for key, value := range values {
		keyObj := &object.String{Value: key}
		pairs[keyObj.HashKey()] = object.HashPair{Key: keyObj, Value: value}
	}
	return &object.Hash{Pairs: pairs}
}

func stringArrayObj(values []string) *object.Array {
	if len(values) == 0 {
		return &object.Array{Elements: []object.Object{}}
	}

	sorted := make([]string, len(values))
	copy(sorted, values)
	sort.Strings(sorted)

	elements := make([]object.Object, len(sorted))
	for i, value := range sorted {
		elements[i] = &object.String{Value: value}
	}
	return &object.Array{Elements: elements}
}

func antiTamperSignalArrayObj(signals []security.AntiTamperSignal) *object.Array {
	if len(signals) == 0 {
		return &object.Array{Elements: []object.Object{}}
	}

	elements := make([]object.Object, len(signals))
	for i, signal := range signals {
		elements[i] = makeHashObject(map[string]object.Object{
			"name": stringObj(signal.Name),
			// measured false means the probe did not look -- it cannot run on
			// this host, or its own check failed -- and detected false beside
			// it is then not a finding. detail says which.
			"measured":   boolObj(signal.Measured),
			"detected":   boolObj(signal.Detected),
			"confidence": intObj(int64(signal.Confidence)),
			"detail":     stringObj(signal.Detail),
		})
	}

	return &object.Array{Elements: elements}
}

func boolObj(v bool) *object.Boolean {
	return &object.Boolean{Value: v}
}

func intObj(v int64) *object.Integer {
	return &object.Integer{Value: v}
}

func stringObj(v string) *object.String {
	return &object.String{Value: v}
}
