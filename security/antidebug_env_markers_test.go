package security

import "testing"

// TestADebuggerEnvironmentMarkerMustHoldSomething runs the rule the Linux and
// macOS environment checks share since M26-TMP-010 on whatever host runs the
// tests, through the getenv each check passes os.Getenv for.
//
// A variable used to count by existing, so an empty LD_PRELOAD counted as a
// debugger attached to this process, and in secure mode that ends the run. An
// empty value says nothing, and neither does a loader list made only of the
// separators the loader splits it at: there is nothing in it for the loader
// to use. DYLD_ROOT_PATH is not such a list, so any value it holds counts.
func TestADebuggerEnvironmentMarkerMustHoldSomething(t *testing.T) {
	names := []string{"LD_PRELOAD", "LD_AUDIT", "DYLD_INSERT_LIBRARIES", "DYLD_ROOT_PATH", "GDB_OPTS"}

	cases := []struct {
		env  map[string]string
		want bool
		why  string
	}{
		{nil, false, "nothing is set"},
		{map[string]string{"LD_PRELOAD": ""}, false, "an empty LD_PRELOAD, the row's own case"},
		{map[string]string{"LD_PRELOAD": ":"}, false, "what LD_PRELOAD=$LD_PRELOAD:$EXTRA leaves when both are empty"},
		{map[string]string{"LD_PRELOAD": " : "}, false, "ld.so splits LD_PRELOAD at spaces as well as colons"},
		{map[string]string{"LD_AUDIT": "::"}, false, "ld.so splits LD_AUDIT at colons"},
		{map[string]string{"DYLD_INSERT_LIBRARIES": ":"}, false, "dyld splits DYLD_INSERT_LIBRARIES at colons"},
		{map[string]string{"DYLD_ROOT_PATH": ""}, false, "an empty DYLD_ROOT_PATH"},
		{map[string]string{"GDB_OPTS": ""}, false, "an empty marker that is not a list"},
		{map[string]string{"GDBHISTFILE": "/home/analyst/.gdb_history"}, false, "a name not on the list is never read"},

		{map[string]string{"LD_PRELOAD": "/tmp/libhook.so"}, true, "a library named for the loader to preload, whatever the length of its path"},
		{map[string]string{"LD_PRELOAD": " :/tmp/libhook.so"}, true, "a library after empty entries"},
		{map[string]string{"LD_AUDIT": " "}, true, "a space is not a separator in LD_AUDIT, so it names a file"},
		{map[string]string{"DYLD_INSERT_LIBRARIES": "/tmp/libhook.dylib"}, true, "any inserted library, not only a few named ones"},
		{map[string]string{"DYLD_ROOT_PATH": "/tmp/root"}, true, "a name on the list that holds a value"},
		{map[string]string{"DYLD_ROOT_PATH": ":"}, true, "since macOS 12 dyld reads DYLD_ROOT_PATH only in a simulator process, and as one path, not a list"},
		{map[string]string{"GDB_OPTS": ":"}, true, "a marker that is not a list counts with any value"},
		{map[string]string{"LD_PRELOAD": "", "GDB_OPTS": "-q"}, true, "one marker with a value is enough"},
	}
	for _, tc := range cases {
		getenv := func(name string) string { return tc.env[name] }
		if got := debuggerEnvironmentMarkerSet(names, getenv); got != tc.want {
			t.Errorf("debuggerEnvironmentMarkerSet with %q = %v, want %v -- %s", tc.env, got, tc.want, tc.why)
		}
	}
}
