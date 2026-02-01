package security

import "runtime"

// SandboxDetector provides comprehensive sandbox detection across multiple platforms.
// It detects common virtualization and sandbox environments used for analysis and testing.
//
// Detection methods include:
// - Virtual machine detection (VMware, VirtualBox, Hyper-V, KVM, QEMU, Parallels, Xen)
// - Container detection (Docker, Kubernetes, LXC)
// - Sandbox environments (cuckoo, WINE, Sandboxie, QEMU)
// - Cloud detection (AWS, Azure, GCP, DigitalOcean)
// - Behavioral analysis environments (various AV/EDR vendors)
type SandboxDetector struct {
	detectedType string
	confidence   int // 0-100
}

// IsSandboxed checks if the process is running in a sandbox or virtualized environment.
// Returns true if any sandbox/VM environment is detected.
func IsSandboxed() bool {
	switch runtime.GOOS {
	case "windows":
		return isSandboxedWindows()
	case "linux":
		return isSandboxedLinux()
	case "darwin":
		return isSandboxedDarwin()
	default:
		return false
	}
}

// DetectSandboxType returns the type of sandbox/VM detected and confidence level (0-100).
// If no sandbox is detected, returns ("", 0).
func DetectSandboxType() (string, int) {
	switch runtime.GOOS {
	case "windows":
		return detectSandboxTypeWindows()
	case "linux":
		return detectSandboxTypeLinux()
	case "darwin":
		return detectSandboxTypeDarwin()
	default:
		return "", 0
	}
}

// GetSandboxIndicators returns a list of all detected sandbox/VM indicators.
// Useful for detailed analysis and logging.
func GetSandboxIndicators() []string {
	switch runtime.GOOS {
	case "windows":
		return getSandboxIndicatorsWindows()
	case "linux":
		return getSandboxIndicatorsLinux()
	case "darwin":
		return getSandboxIndicatorsDarwin()
	default:
		return nil
	}
}
