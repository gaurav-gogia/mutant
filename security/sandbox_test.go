package security

import (
	"testing"
)

// TestSandboxDetection tests basic sandbox detection
func TestSandboxDetection(t *testing.T) {
	// Run detection
	sandboxed := IsSandboxed()

	// This test will pass regardless - just verifies no panic
	t.Logf("Sandbox detection result: %v", sandboxed)
}

// TestDetectSandboxType tests sandbox type detection
func TestDetectSandboxType(t *testing.T) {
	sandboxType, confidence := DetectSandboxType()

	t.Logf("Detected sandbox type: %s (confidence: %d%%)", sandboxType, confidence)

	// Validate confidence range
	if confidence < 0 || confidence > 100 {
		t.Errorf("Invalid confidence level: %d", confidence)
	}
}

// TestGetSandboxIndicators tests indicator retrieval
func TestGetSandboxIndicators(t *testing.T) {
	indicators := GetSandboxIndicators()

	t.Logf("Detected %d sandbox indicators", len(indicators))
	for i, indicator := range indicators {
		t.Logf("  [%d] %s", i+1, indicator)
	}
}

// BenchmarkSandboxDetection benchmarks the detection function
func BenchmarkSandboxDetection(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = IsSandboxed()
	}
}

// BenchmarkDetectSandboxType benchmarks type detection
func BenchmarkDetectSandboxType(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, _ = DetectSandboxType()
	}
}

// BenchmarkGetSandboxIndicators benchmarks indicator retrieval
func BenchmarkGetSandboxIndicators(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = GetSandboxIndicators()
	}
}
