package security

import (
	"errors"
	"slices"
	"testing"
)

// M26-TMP-013. A detector can return an answer and an error together: the
// error names checks it could not make -- on Linux, a cgroup file it could not
// read -- and the answer is what the other checks found. Every reader of the
// cached detection used to take the error for the whole answer:
// DetectSandboxType reported "none" at 0, IsSandboxed false,
// GetSandboxIndicators nothing and SandboxTamperDetail no signals, for a run
// whose other checks had found WSL at 95.
//
// The detection is planted in the process cache, so these tests mean the same
// on every platform, whatever this host would detect.
func plantPartialSandboxDetection(t *testing.T, detection sandboxDetection, err error) {
	t.Helper()
	t.Cleanup(resetSandboxCache)
	resetSandboxCache()
	sandboxOnce.Do(func() {})
	sandboxCached = detection
	sandboxCachedErr = err
}

var errPartialSandboxDetection = errors.New("sandbox detection is incomplete: a cgroup file could not be read, " +
	"so its container markers went unchecked (open /proc/1/cgroup: no such file or directory)")

// At the threshold the answer stands: a check that was not made can only have
// hidden an indicator, so the full detection would score at least as high.
func TestSandboxReadersKeepADetectionReturnedBesideAnError(t *testing.T) {
	found := sandboxDetection{
		Type:       "WSL",
		Confidence: 95,
		Indicators: []string{"linux:env:wsl", "linux:kernel:microsoft"},
	}
	plantPartialSandboxDetection(t, found, errPartialSandboxDetection)

	typ, confidence, err := DetectSandboxType()
	if typ != found.Type || confidence != found.Confidence || !errors.Is(err, errPartialSandboxDetection) {
		t.Errorf("DetectSandboxType() = (%q, %d, %v); want (%q, %d, the detector's error)",
			typ, confidence, err, found.Type, found.Confidence)
	}
	if !IsSandboxed() {
		t.Errorf("IsSandboxed() = false for a detection at %d that came with an error", found.Confidence)
	}
	indicators, err := GetSandboxIndicators()
	if !slices.Equal(indicators, found.Indicators) || !errors.Is(err, errPartialSandboxDetection) {
		t.Errorf("GetSandboxIndicators() = (%v, %v); want (%v, the detector's error)", indicators, err, found.Indicators)
	}
	detail := SandboxTamperDetail()
	if detail.Kind != found.Type || detail.Confidence != found.Confidence || !slices.Equal(detail.Signals, found.Indicators) {
		t.Errorf("SandboxTamperDetail() = %+v; want the type, confidence and indicators found", detail)
	}
}

// Below the threshold the answer is open, since a check that was not made could
// have found the rest. The run is not stopped on it, and the readers still get
// what was found, beside the error that says the answer is incomplete.
func TestSandboxReadersLeaveABelowThresholdDetectionOpenBesideAnError(t *testing.T) {
	found := sandboxDetection{
		Type:       "VM",
		Confidence: 30,
		Indicators: []string{"linux:cpu:hypervisor_flag"},
	}
	plantPartialSandboxDetection(t, found, errPartialSandboxDetection)

	typ, confidence, err := DetectSandboxType()
	if typ != found.Type || confidence != found.Confidence || !errors.Is(err, errPartialSandboxDetection) {
		t.Errorf("DetectSandboxType() = (%q, %d, %v); want (%q, %d, the detector's error)",
			typ, confidence, err, found.Type, found.Confidence)
	}
	if IsSandboxed() {
		t.Errorf("IsSandboxed() = true for a detection at %d, below the threshold %d", found.Confidence, SandboxDetectedThreshold)
	}
	indicators, err := GetSandboxIndicators()
	if !slices.Equal(indicators, found.Indicators) || !errors.Is(err, errPartialSandboxDetection) {
		t.Errorf("GetSandboxIndicators() = (%v, %v); want (%v, the detector's error)", indicators, err, found.Indicators)
	}
}
