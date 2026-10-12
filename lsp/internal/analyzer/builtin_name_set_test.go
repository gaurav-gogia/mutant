package analyzer

import (
	"runtime"
	"testing"

	mast "mutant/ast"
	"mutant/builtin"
)

// TestTheRegistryIsReadNotRebuilt holds the rules that ask whether a callee is a
// builtin in this build to the set built once, when the package starts
// (builtinNameSet). forEachBuiltinCall and uncheckedError each built it again on
// every call -- a map of every builtin's name, about 20 KB -- and eleven rules
// ask per analysis, so every keystroke paid for eleven copies of a set that
// never changes (M26-LSP-026).
//
// Measured over one statement making one builtin call, so that a set built per
// call is caught as well as one built per walk, and which gives the walk next to
// nothing to allocate. A set of every name holds a 16-byte string header for
// each, so it cannot be built under half of that per name, which is the bound.
func TestTheRegistryIsReadNotRebuilt(t *testing.T) {
	snapshot := New().Analyze("let x = len(\"evidence\");\n")
	if snapshot == nil || snapshot.Program == nil {
		t.Fatal("the one-statement program did not analyse")
	}
	calls := 0
	forEachBuiltinCall(snapshot.Program.Statements, nil,
		func(string, mast.Node, *mast.CallExpression, map[string]mast.Expression) { calls++ })
	if calls != 1 {
		t.Fatalf("the walk visited %d builtin calls, want the one the program makes", calls)
	}
	bound := uint64(len(builtin.Builtins)) * 8
	for _, tc := range []struct {
		name string
		run  func()
	}{
		{"forEachBuiltinCall", func() {
			forEachBuiltinCall(snapshot.Program.Statements, nil,
				func(string, mast.Node, *mast.CallExpression, map[string]mast.Expression) {})
		}},
		{"uncheckedError", func() { lintUncheckedErrors(snapshot, DefaultLintConfig()) }},
	} {
		if got := leastBytesOfOneRun(tc.run); got >= bound {
			t.Errorf("%s allocates %d bytes a run over one statement, at least %d: it builds a set of every "+
				"builtin's name again instead of reading builtinNameSet", tc.name, got, bound)
		}
	}
}

// leastBytesOfOneRun is the least one run of f allocated on the heap, over a
// few runs measured one at a time, after one run that builds whatever is built
// on first use.
//
// runtime.MemStats.TotalAlloc is process-wide: everything any goroutine
// allocates between the two readings lands in the difference. The average of
// twenty runs this replaced charged all of it to f, so a correct walk read 41 KB
// a run -- seven times the bound -- the moment another goroutine allocated
// 200 KiB inside the window once in five runs (M26-LSP-032, the same defect as
// M26-TEST-021). Contamination can only ever add, so the least reading carries
// the least of it; a set rebuilt per walk or per call is in every reading, so
// the least still contains it.
//
// It is the floor and not a difference of two fixtures, which is what
// builtin/allocation_gap_test.go measures, because the regression this test
// exists for is a set built once per WALK: a one-call fixture and a many-call
// fixture would both pay for it, and their difference would cancel it.
func leastBytesOfOneRun(f func()) uint64 {
	f()
	const attempts = 10
	least := ^uint64(0)
	var before, after runtime.MemStats
	for range attempts {
		runtime.ReadMemStats(&before)
		f()
		runtime.ReadMemStats(&after)
		if got := after.TotalAlloc - before.TotalAlloc; got < least {
			least = got
		}
	}
	return least
}

var contaminationSink []byte

// TestTheFloorChargesOnlyTheRunItMeasures pins the measurement itself: another
// goroutine allocating inside the window, once in five runs, must not be
// charged to a walk that allocates almost nothing, and a walk that does build a
// set of every builtin's name must still read above the bound.
func TestTheFloorChargesOnlyTheRunItMeasures(t *testing.T) {
	snapshot := New().Analyze("let x = len(\"evidence\");\n")
	walk := func() {
		forEachBuiltinCall(snapshot.Program.Statements, nil,
			func(string, mast.Node, *mast.CallExpression, map[string]mast.Expression) {})
	}
	bound := uint64(len(builtin.Builtins)) * 8

	runs := 0
	contaminated := func() {
		walk()
		runs++
		if runs%5 == 0 {
			done := make(chan struct{})
			go func() {
				contaminationSink = make([]byte, 200<<10)
				close(done)
			}()
			<-done
		}
	}
	if got := leastBytesOfOneRun(contaminated); got >= bound {
		t.Errorf("another goroutine's allocation was charged to the walk: %d bytes a run, bound %d", got, bound)
	}

	rebuilding := func() {
		set := make(map[string]struct{}, len(builtin.Builtins))
		for _, b := range builtin.Builtins {
			set[b.Name] = struct{}{}
		}
		_ = set
		walk()
	}
	if got := leastBytesOfOneRun(rebuilding); got < bound {
		t.Errorf("a walk that rebuilds the set read %d bytes a run, under the bound %d: the measurement "+
			"no longer sees the regression it exists for", got, bound)
	}
}
