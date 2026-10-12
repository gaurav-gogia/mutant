//go:build darwin

package security

import (
	"os"
	"testing"
)

// TestDarwinDebuggerEnvironmentMarkersNeedAValue is M26-TMP-010 on macOS.
//
// hasDebuggerEnvironmentMarkersDarwin returned true for any listed variable
// that existed at all. An empty DYLD_INSERT_LIBRARIES therefore counted as a
// debugger attached to this process, and so did GDBHISTFILE, exported from a
// shell profile, and XCODE_VERSION_ACTUAL, which Xcode sets for every
// build-phase script. In secure mode each one ended the run with that reason.
//
// A filter that counted DYLD_INSERT_LIBRARIES only for a few libraries sat
// after that loop and never ran. It is gone, and any inserted library counts.
func TestDarwinDebuggerEnvironmentMarkersNeedAValue(t *testing.T) {
	// Begin with none of the variables in question set, whatever the host
	// running the test exported. t.Setenv puts each one back afterwards.
	notMarkers := []string{"GDBHISTFILE", "XCODE_VERSION_ACTUAL"}
	for _, name := range append(append([]string{}, darwinDebuggerEnvVars...), notMarkers...) {
		t.Setenv(name, "")
		os.Unsetenv(name)
	}
	if hasDebuggerEnvironmentMarkersDarwin() {
		t.Fatal("hasDebuggerEnvironmentMarkersDarwin() = true with none of its variables set")
	}

	cases := []struct {
		label, name, value string
		want               bool
		why                string
	}{
		{"empty DYLD_INSERT_LIBRARIES", "DYLD_INSERT_LIBRARIES", "", false,
			"an empty DYLD_INSERT_LIBRARIES inserts nothing"},
		{"DYLD_INSERT_LIBRARIES of colons", "DYLD_INSERT_LIBRARIES", "::", false,
			"nor does a list with no entry in it"},
		{"GDBHISTFILE", "GDBHISTFILE", "/Users/analyst/.local/share/gdb/history", false,
			"where gdb keeps its history, exported from a shell profile"},
		{"XCODE_VERSION_ACTUAL", "XCODE_VERSION_ACTUAL", "1600", false,
			"Xcode sets it for every build-phase script"},

		{"an ordinary inserted library", "DYLD_INSERT_LIBRARIES", "/tmp/libhook.dylib", true,
			"a library the old filter did not name"},
		{"Guard Malloc", "DYLD_INSERT_LIBRARIES", "/usr/lib/libgmalloc.dylib", true,
			"a library the old filter did name"},
	}
	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			t.Setenv(tc.name, tc.value)
			if got := hasDebuggerEnvironmentMarkersDarwin(); got != tc.want {
				t.Errorf("hasDebuggerEnvironmentMarkersDarwin() = %v with %s=%q, want %v -- %s",
					got, tc.name, tc.value, tc.want, tc.why)
			}
		})
	}

	// Every variable the check reads still counts once it holds a value.
	for _, name := range darwinDebuggerEnvVars {
		t.Run("listed "+name, func(t *testing.T) {
			t.Setenv(name, "1")
			if !hasDebuggerEnvironmentMarkersDarwin() {
				t.Errorf("hasDebuggerEnvironmentMarkersDarwin() = false with %s=1", name)
			}
		})
	}
}
