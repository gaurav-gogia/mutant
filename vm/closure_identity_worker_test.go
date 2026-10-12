package vm

import "testing"

// A worker detaches the closures it is handed, so that their cells are its
// own (M26-VM-004). The detached closure is still the function value the
// caller handed over, so `==` inside the worker must still call it equal to
// itself however it reached the worker: as an element and as a capture of the
// callback (M26-EVL-005). Before closures carried an identity, the VM's copy on
// every load made this 0 even without a worker.
func TestWorkerKeepsClosureIdentityEVL005(t *testing.T) {
	input := `let probe = fn() {
		let n = 0;
		let bump = fn() { n = n + 1; return n; };
		let r = pmap([bump, bump], fn(f) { return f == bump; }, 2);
		if (r[0] && r[1]) { return 1; }
		return 0;
	}; probe()`

	machine, err := runEncryptedVM(input)
	if err != nil {
		t.Fatalf("vm error: %s", err)
	}
	if err := testIntegerObject(1, machine.LastPoppedStackElement()); err != nil {
		t.Fatalf("a closure handed to a worker is not equal to itself there: %s", err)
	}
}
