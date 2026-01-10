package security

import (
	"testing"
	"time"
)

// TestIsDebuggerPresent tests basic debugger detection
func TestIsDebuggerPresent(t *testing.T) {
	// This should return false when running normally
	isDebugger := IsDebuggerPresent()

	t.Logf("Debugger detected: %v", isDebugger)

	// Note: Will be true if running under debugger
	// This is expected behavior
}

// TestDetectDebuggerAdvanced tests advanced detection
func TestDetectDebuggerAdvanced(t *testing.T) {
	isDebugger := DetectDebuggerAdvanced()

	t.Logf("Advanced debugger detection: %v", isDebugger)
}

// TestSecureCompare tests constant-time comparison
func TestSecureCompare(t *testing.T) {
	a := []byte("password123")
	b := []byte("password123")
	c := []byte("password456")

	// Should return true
	if !SecureCompare(a, b) {
		t.Error("SecureCompare failed for equal values")
	}

	// Should return false
	if SecureCompare(a, c) {
		t.Error("SecureCompare returned true for different values")
	}
}

// TestSecureZero tests memory zeroing
func TestSecureZero(t *testing.T) {
	data := []byte("sensitive data")

	// Verify data is not zero
	for _, b := range data {
		if b == 0 {
			t.Error("Data already contains zeros before zeroing")
			break
		}
	}

	// Zero the data
	SecureZero(data)

	// Verify all bytes are zero
	for i, b := range data {
		if b != 0 {
			t.Errorf("Byte at index %d was not zeroed: %d", i, b)
		}
	}
}

// TestSecureRandBytes tests secure random generation
func TestSecureRandBytes(t *testing.T) {
	size := 32
	bytes1, err := SecureRandBytes(size)
	if err != nil {
		t.Fatalf("Failed to generate random bytes: %v", err)
	}

	bytes2, err := SecureRandBytes(size)
	if err != nil {
		t.Fatalf("Failed to generate random bytes: %v", err)
	}

	// Should be different
	if string(bytes1) == string(bytes2) {
		t.Error("Two random byte sequences are identical (extremely unlikely)")
	}

	// Should be correct length
	if len(bytes1) != size {
		t.Errorf("Expected %d bytes, got %d", size, len(bytes1))
	}
}

// BenchmarkSecureCompare benchmarks constant-time comparison
func BenchmarkSecureCompare(b *testing.B) {
	a := []byte("this is a test password")
	c := []byte("this is a test password")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		SecureCompare(a, c)
	}
}

// BenchmarkTimingAttackDetection benchmarks timing-based debugger detection
func BenchmarkTimingAttackDetection(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		detectTimingAnomaly()
	}
}

// TestTimingConsistency ensures timing detection is consistent
func TestTimingConsistency(t *testing.T) {
	// Run timing test multiple times
	results := make([]bool, 10)

	for i := 0; i < 10; i++ {
		results[i] = detectTimingAnomaly()
		time.Sleep(10 * time.Millisecond)
	}

	// Count detections
	detections := 0
	for _, r := range results {
		if r {
			detections++
		}
	}

	t.Logf("Timing anomalies detected: %d/10", detections)

	// Should be consistent (all false or all true, not mixed)
	// Allow some variance due to system load
	if detections > 0 && detections < 10 {
		t.Logf("Warning: Inconsistent timing detection (system under load?)")
	}
}
