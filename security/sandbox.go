package security

import (
	"runtime"
	"strings"
	"sync"
)

type sandboxDetection struct {
	Type       string
	Confidence int
	Indicators []string
}

// SandboxDetectedThreshold is the confidence from which a run counts as
// sandboxed, by the run's own checks and by the status builtins alike. A detector
// adds up its indicators' confidences per sandbox type; on Linux, for one, the
// specific indicators -- a Docker or WSL marker, a VM vendor in the CPU or the
// firmware -- reach it alone, and the generic ones, such as the CPU's
// hypervisor flag, do not.
//
// It is exported because builtin/security_status.go answers the same question
// for the status builtin's "detected" field, and answered it from a literal 70
// until 2.6.0. Two copies of one rule disagree the moment either moves, and
// this one moving is how the builtin would have started publishing a different
// answer from the detector's own.
//
//mutant:limit score
const SandboxDetectedThreshold = 70

const (
	// SandboxTypeNone is the type DetectSandboxType reports when it found
	// nothing, and is exported for the same reason as the threshold above:
	// builtin/security_status.go compared against the bare string.
	SandboxTypeNone = "none"
	// minConfidenceScore and maxConfidenceScore are the ends of the scale
	// every detector scores on, and a score is clamped into them.
	//
	//mutant:format the 0-100 confidence scale of TamperDetail.Confidence
	minConfidenceScore = 0
	// maxConfidenceScore is the top of the same scale.
	//
	//mutant:format the 0-100 confidence scale of TamperDetail.Confidence
	maxConfidenceScore = 100
)

// IsSandboxed returns true when environment signals indicate a high-confidence
// container, virtualization, or sandboxed runtime.
func IsSandboxed() bool {
	// An error from DetectSandboxType names checks it could not make, and the
	// confidence beside it is what the other checks found. A check not made
	// can only have hidden an indicator, so that confidence is a floor: at the
	// threshold it is a detection whether or not every check ran. Answering
	// false on any error reported a container it had found as no sandbox at
	// all. (M26-TMP-013)
	_, confidence, _ := DetectSandboxType()
	return confidence >= SandboxDetectedThreshold
}

// DetectSandboxType returns the most likely sandbox type and confidence (0-100).
//
// A non-nil error names checks the detector could not make, such as a Linux
// cgroup file it could not read. It comes beside the type and confidence the
// other checks found, never in place of them: a check that was not made can
// only have hidden an indicator, so what the others found still stands.
func DetectSandboxType() (string, int, error) {
	result, err := detectSandbox()
	if err != nil {
		securityDevLogf("sandbox detect error=%v", err)
	}
	if result.Confidence < minConfidenceScore {
		result.Confidence = minConfidenceScore
	}
	if result.Confidence > maxConfidenceScore {
		result.Confidence = maxConfidenceScore
	}
	if result.Type == "" || result.Confidence == minConfidenceScore {
		securityDevLogf("sandbox detected=false type=%s confidence=0 indicators=", SandboxTypeNone)
		return SandboxTypeNone, minConfidenceScore, err
	}
	securityDevLogf(
		"sandbox detected=%t type=%s confidence=%d indicators=%s",
		result.Confidence >= SandboxDetectedThreshold,
		result.Type,
		result.Confidence,
		strings.Join(result.Indicators, ","),
	)
	return result.Type, result.Confidence, err
}

// GetSandboxIndicators returns normalized detection indicators for audit/logging,
// with the error DetectSandboxType returns: beside the indicators the other
// checks found, not in place of them.
func GetSandboxIndicators() ([]string, error) {
	result, err := detectSandbox()
	if len(result.Indicators) == 0 {
		return nil, err
	}

	indicators := make([]string, len(result.Indicators))
	copy(indicators, result.Indicators)
	return indicators, err
}

// SandboxTamperDetail reports what the sandbox detector saw, for a run that is
// stopping because of it. Detection is cached for the process (see
// detectSandbox), so this re-reads the answer the gate already got rather than
// probing again. (M-7)
func SandboxTamperDetail() TamperDetail {
	// An error beside the detection names checks that were not made. It does
	// not unmake the signals that fired, and they are what this reports.
	// (M26-TMP-013)
	result, _ := detectSandbox()

	return TamperDetail{
		Detector:   "sandbox",
		Kind:       result.Type,
		Confidence: result.Confidence,
		Signals:    append([]string(nil), result.Indicators...),
	}
}

// Whether this process is running inside a container, VM, or analysis sandbox is
// fixed for its lifetime -- a program does not move between a hypervisor and
// bare metal while it runs -- but establishing it is expensive. On Windows the
// probe shells out to tasklist, three wmic queries, powershell, and four reg
// queries, which together cost about a second.
//
// That cost used to be paid per call, and the VM calls this from its instruction
// stream: OpChkSnd runs security.IsSandboxed() wherever the compiler injected a
// check, so a program's runtime grew by roughly a second for every injected
// check it reached. Detecting once and remembering the answer is both faster and
// more honest about what is being measured.
var (
	sandboxOnce      sync.Once
	sandboxCached    sandboxDetection
	sandboxCachedErr error
)

func detectSandbox() (sandboxDetection, error) {
	sandboxOnce.Do(func() {
		sandboxCached, sandboxCachedErr = detectSandboxUncached()
	})
	// The struct is returned by value; callers that adjust Confidence or read
	// Indicators (which GetSandboxIndicators copies) cannot disturb the cache.
	return sandboxCached, sandboxCachedErr
}

// resetSandboxCache forces the next detection to run for real. Tests that drive
// the platform detectors through different environments need it; nothing in the
// running language does.
func resetSandboxCache() {
	sandboxOnce = sync.Once{}
	sandboxCached = sandboxDetection{}
	sandboxCachedErr = nil
}

func detectSandboxUncached() (sandboxDetection, error) {
	switch runtime.GOOS {
	case "windows":
		return detectSandboxWindows()
	case "linux":
		return detectSandboxLinux()
	case "darwin":
		return detectSandboxDarwin()
	default:
		return sandboxDetection{}, nil
	}
}
