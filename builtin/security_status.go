package builtin

import (
	"sort"

	"mutant/object"
	"mutant/security"
)

func DebugStatus(args ...object.Object) object.Object {
	if len(args) != 0 {
		return newError("wrong number of arguments. got=%d, want=0", len(args))
	}

	detected := security.IsDebuggerPresent()
	telemetry := security.SecurityTelemetrySnapshot()

	result := map[string]object.Object{
		"detected":       boolObj(detected),
		"type":           stringObj(detectionType(detected, "debugger")),
		"confidence":     intObj(detectionConfidence(detected)),
		"indicators":     stringArrayObj(nil),
		"rust_signals":   rustSignalArrayObj(nil),
		"rust_enabled":   boolObj(false),
		"rust_error":     stringObj(""),
		"source":         stringObj("go-security"),
		"advisory":       boolObj(true),
		"event_count":    intObj(int64(telemetry["debugger_detected"])),
		"error":          stringObj(""),
		"schema_version": intObj(1),
	}

	rustSignals, rustEnabled, rustErr := security.RunRustProbe(
		[]string{"hardware_breakpoint", "timing", "syscall", "frida_ptrace", "trampoline", "iat_got"},
		"builtin:debug_status",
	)
	result["rust_enabled"] = boolObj(rustEnabled)
	result["rust_signals"] = rustSignalArrayObj(rustSignals)
	if rustErr != nil {
		result["rust_error"] = stringObj(rustErr.Error())
	}

	return makeHashObject(result)
}

func SandboxStatus(args ...object.Object) object.Object {
	if len(args) != 0 {
		return newError("wrong number of arguments. got=%d, want=0", len(args))
	}

	statusType, confidence, err := security.DetectSandboxType()
	indicators, indicatorsErr := security.GetSandboxIndicators()
	if indicatorsErr != nil {
		indicators = nil
	}

	detected := confidence >= 70
	if statusType == "none" {
		detected = false
	}

	errorMessage := ""
	if err != nil {
		errorMessage = err.Error()
	}
	if indicatorsErr != nil {
		if errorMessage != "" {
			errorMessage += "; "
		}
		errorMessage += indicatorsErr.Error()
	}

	telemetry := security.SecurityTelemetrySnapshot()
	result := map[string]object.Object{
		"detected":       boolObj(detected),
		"type":           stringObj(statusType),
		"confidence":     intObj(int64(confidence)),
		"indicators":     stringArrayObj(indicators),
		"rust_signals":   rustSignalArrayObj(nil),
		"rust_enabled":   boolObj(false),
		"rust_error":     stringObj(""),
		"source":         stringObj("go-security"),
		"advisory":       boolObj(true),
		"event_count":    intObj(int64(telemetry["sandbox_detected"])),
		"error":          stringObj(errorMessage),
		"schema_version": intObj(1),
	}

	rustSignals, rustEnabled, rustErr := security.RunRustProbe(
		[]string{"cpuid_hypervisor", "rdtsc_drift", "acpi_pci", "gpu_feature", "ld_preload", "syscall_table"},
		"builtin:sandbox_status",
	)
	result["rust_enabled"] = boolObj(rustEnabled)
	result["rust_signals"] = rustSignalArrayObj(rustSignals)
	if rustErr != nil {
		result["rust_error"] = stringObj(rustErr.Error())
	}

	return makeHashObject(result)
}

func SecurityDiagnostics(args ...object.Object) object.Object {
	if len(args) != 0 {
		return newError("wrong number of arguments. got=%d, want=0", len(args))
	}

	debugDetected, debugMethods := security.DetectDebuggerDetails()
	sandboxType, sandboxConfidence, sandboxErr := security.DetectSandboxType()
	sandboxIndicators, indicatorsErr := security.GetSandboxIndicators()

	sandboxDetected := sandboxConfidence >= 70
	if sandboxType == "none" {
		sandboxDetected = false
	}

	sandboxError := ""
	if sandboxErr != nil {
		sandboxError = sandboxErr.Error()
	}
	if indicatorsErr != nil {
		if sandboxError != "" {
			sandboxError += "; "
		}
		sandboxError += indicatorsErr.Error()
	}

	telemetry := security.SecurityTelemetrySnapshot()

	return makeHashObject(map[string]object.Object{
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
	})
}

func detectionType(detected bool, name string) string {
	if detected {
		return name
	}
	return "none"
}

func detectionConfidence(detected bool) int64 {
	if detected {
		return 100
	}
	return 0
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

func rustSignalArrayObj(signals []security.RustSignal) *object.Array {
	if len(signals) == 0 {
		return &object.Array{Elements: []object.Object{}}
	}

	elements := make([]object.Object, len(signals))
	for i, signal := range signals {
		elements[i] = makeHashObject(map[string]object.Object{
			"name":       stringObj(signal.Name),
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
