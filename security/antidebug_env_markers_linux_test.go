//go:build linux

package security

import (
	"os"
	"testing"
)

// TestLinuxDebuggerEnvironmentMarkersNeedAValue is M26-TMP-010 on Linux.
//
// hasDebuggerEnvironmentMarkers returned true for any listed variable that
// existed at all. An empty LD_PRELOAD, which loads nothing, therefore counted
// as a debugger attached to this process, and so did GDBHISTFILE, the path of
// gdb's history file, which XDG base-directory guides tell users to export
// from a shell profile. In secure mode either one ended the run with that
// reason.
//
// The cases that must still fire matter as much. A rule that only an
// LD_PRELOAD longer than fifty characters counted sat after a loop that had
// already returned on the variable existing, so it never ran. It is gone, and
// an LD_PRELOAD naming a library under a short path is still a marker.
//
// t.Setenv loads nothing into the test binary, which is already running: the
// loader reads LD_PRELOAD and LD_AUDIT only as a dynamically linked program
// starts. What is tested is what the detector makes of each value.
func TestLinuxDebuggerEnvironmentMarkersNeedAValue(t *testing.T) {
	// Begin with none of the variables in question set, whatever the host
	// running the test exported. t.Setenv puts each one back afterwards.
	notMarkers := []string{"GDBHISTFILE", "LLDB_HIST_FILE"}
	for _, name := range append(append([]string{}, linuxDebuggerEnvVars...), notMarkers...) {
		t.Setenv(name, "")
		os.Unsetenv(name)
	}
	if hasDebuggerEnvironmentMarkers() {
		t.Fatal("hasDebuggerEnvironmentMarkers() = true with none of its variables set")
	}

	cases := []struct {
		label, name, value string
		want               bool
		why                string
	}{
		{"empty LD_PRELOAD", "LD_PRELOAD", "", false,
			"an empty LD_PRELOAD loads nothing"},
		{"LD_PRELOAD of separators", "LD_PRELOAD", ": ", false,
			"a list with no entry loads nothing; LD_PRELOAD=$LD_PRELOAD:$EXTRA leaves one when both are empty"},
		{"empty LD_AUDIT", "LD_AUDIT", "", false,
			"an empty LD_AUDIT loads nothing either"},
		{"LD_AUDIT of colons", "LD_AUDIT", "::", false,
			"nor does an LD_AUDIT list with no entry in it"},
		{"GDBHISTFILE", "GDBHISTFILE", "/home/analyst/.local/share/gdb/history", false,
			"where gdb keeps its history, exported from a shell profile"},
		{"LLDB_HIST_FILE", "LLDB_HIST_FILE", "/home/analyst/.lldb/history", false,
			"it only looks like a history-file setting: no lldb reads it"},

		{"short LD_PRELOAD", "LD_PRELOAD", "/tmp/libhook.so", true,
			"it names a library for the loader to preload, under a path well short of fifty characters"},
		{"LD_AUDIT", "LD_AUDIT", "/tmp/libaudit.so", true,
			"it names an auditing library for the loader to load"},
	}
	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			t.Setenv(tc.name, tc.value)
			if got := hasDebuggerEnvironmentMarkers(); got != tc.want {
				t.Errorf("hasDebuggerEnvironmentMarkers() = %v with %s=%q, want %v -- %s",
					got, tc.name, tc.value, tc.want, tc.why)
			}
		})
	}

	// Every variable the check reads still counts once it holds a value.
	for _, name := range linuxDebuggerEnvVars {
		t.Run("listed "+name, func(t *testing.T) {
			t.Setenv(name, "1")
			if !hasDebuggerEnvironmentMarkers() {
				t.Errorf("hasDebuggerEnvironmentMarkers() = false with %s=1", name)
			}
		})
	}
}
