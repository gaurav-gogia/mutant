//go:build darwin
// +build darwin

package security

import "os/exec"

// detectSandboxDarwin reads what the macOS rules score -- the environment, the
// process list and the CPU's hypervisor vendor -- and hands it to
// scoreDarwinSandbox. The rules live in sandbox_darwin_signals.go, which builds
// on every platform, so that they can be run from any host and not only from a
// Mac.
func detectSandboxDarwin() (sandboxDetection, error) {
	processes := ""
	if out, err := exec.Command("ps", "-axo", "comm").CombinedOutput(); err == nil {
		processes = string(out)
	}
	return scoreDarwinSandbox(envSet, processes, getCPUIDHypervisorVendor()), nil
}
