//go:build linux
// +build linux

package security

import (
	"os"
	"os/exec"
	"strings"
)

// isSandboxedLinux performs multiple sandbox/VM detection techniques on Linux
func isSandboxedLinux() bool {
	// Check for VM/Sandbox indicators
	if checkVirtualMachineHardwareLinux() {
		return true
	}
	if checkDockerContainer() {
		return true
	}
	if checkKubernetesContainer() {
		return true
	}
	if checkLXCContainer() {
		return true
	}
	if checkQemuKVMLinux() {
		return true
	}
	if checkCuckooLinux() {
		return true
	}
	if checkCgroupSandbox() {
		return true
	}
	if checkSystemdNspawnContainer() {
		return true
	}

	return false
}

// detectSandboxTypeLinux identifies the specific sandbox/VM type on Linux
func detectSandboxTypeLinux() (string, int) {
	// Docker detection
	if checkDockerContainer() {
		return "Docker", 95
	}
	// Kubernetes detection
	if checkKubernetesContainer() {
		return "Kubernetes", 90
	}
	// LXC detection
	if checkLXCContainer() {
		return "LXC", 85
	}
	// KVM/QEMU detection
	if checkQemuKVMLinux() {
		return "KVM/QEMU", 80
	}
	// Cuckoo Sandbox detection
	if checkCuckooLinux() {
		return "Cuckoo", 90
	}
	// systemd-nspawn detection
	if checkSystemdNspawnContainer() {
		return "systemd-nspawn", 80
	}
	// VMware detection
	if checkVMwareLinux() {
		return "VMware", 80
	}
	// VirtualBox detection
	if checkVirtualBoxLinux() {
		return "VirtualBox", 80
	}
	// Xen detection
	if checkXenLinux() {
		return "Xen", 75
	}

	return "", 0
}

// getSandboxIndicatorsLinux returns all detected indicators
func getSandboxIndicatorsLinux() []string {
	var indicators []string

	if checkDockerContainer() {
		indicators = append(indicators, "Docker container detected")
	}
	if checkKubernetesContainer() {
		indicators = append(indicators, "Kubernetes container detected")
	}
	if checkLXCContainer() {
		indicators = append(indicators, "LXC container detected")
	}
	if checkQemuKVMLinux() {
		indicators = append(indicators, "KVM/QEMU detected")
	}
	if checkCuckooLinux() {
		indicators = append(indicators, "Cuckoo Sandbox detected")
	}
	if checkSystemdNspawnContainer() {
		indicators = append(indicators, "systemd-nspawn container detected")
	}
	if checkVMwareLinux() {
		indicators = append(indicators, "VMware detected")
	}
	if checkVirtualBoxLinux() {
		indicators = append(indicators, "VirtualBox detected")
	}
	if checkXenLinux() {
		indicators = append(indicators, "Xen detected")
	}
	if checkCgroupSandbox() {
		indicators = append(indicators, "Cgroup-based sandbox detected")
	}

	return indicators
}

// checkVirtualMachineHardwareLinux checks for VM indicators on Linux
func checkVirtualMachineHardwareLinux() bool {
	return checkVMwareLinux() || checkVirtualBoxLinux() || checkXenLinux() || checkQemuKVMLinux()
}

// checkDockerContainer detects Docker container environment
func checkDockerContainer() bool {
	// Check for /.dockerenv file
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true
	}

	// Check for /docker in cgroup
	if checkCgroupContains("docker") {
		return true
	}

	// Check for Docker-specific environment variables
	if os.Getenv("DOCKER_HOST") != "" {
		return true
	}

	// Check /proc/1/cgroup for Docker references
	if fileContains("/proc/1/cgroup", "docker") {
		return true
	}

	// Check for Docker hostname patterns
	if checkHostnamePattern("^[a-f0-9]{12}$") {
		return true
	}

	return false
}

// checkKubernetesContainer detects Kubernetes container environment
func checkKubernetesContainer() bool {
	// Check for Kubernetes service environment variables
	if os.Getenv("KUBERNETES_SERVICE_HOST") != "" {
		return true
	}
	if os.Getenv("KUBERNETES_SERVICE_PORT") != "" {
		return true
	}

	// Check for Kubernetes token
	if _, err := os.Stat("/var/run/secrets/kubernetes.io/serviceaccount/token"); err == nil {
		return true
	}

	// Check for Kubernetes namespace file
	if _, err := os.Stat("/var/run/secrets/kubernetes.io/serviceaccount/namespace"); err == nil {
		return true
	}

	return false
}

// checkLXCContainer detects LXC container environment
func checkLXCContainer() bool {
	// Check for lxc-init process
	if fileContains("/proc/1/cmdline", "lxc") {
		return true
	}

	// Check for LXC-specific cgroup
	if checkCgroupContains("lxc") {
		return true
	}

	// Check for LXC apparmor profile
	if fileContains("/proc/self/attr/current", "lxc") {
		return true
	}

	// Check for container type in cgroup
	if fileContains("/proc/1/cgroup", "lxc") {
		return true
	}

	return false
}

// checkQemuKVMLinux detects KVM/QEMU environment on Linux
func checkQemuKVMLinux() bool {
	// Check for KVM in CPUID
	if checkCPUIDKVM() {
		return true
	}

	// Check /proc/cpuinfo for KVM indicators
	if fileContains("/proc/cpuinfo", "hypervisor") {
		return true
	}

	// Check for QEMU process
	if processExists("qemu") || processExists("qemu-system") {
		return true
	}

	// Check DMI product name for QEMU
	if fileContains("/sys/class/dmi/id/product_name", "QEMU") {
		return true
	}

	return false
}

// checkCuckooLinux detects Cuckoo Sandbox on Linux
func checkCuckooLinux() bool {
	// Check for Cuckoo agent process
	if processExists("agent.py") || processExists("cuckoo") {
		return true
	}

	// Check for Cuckoo paths
	if _, err := os.Stat("/cuckoo/"); err == nil {
		return true
	}
	if _, err := os.Stat("/opt/cuckoo/"); err == nil {
		return true
	}

	// Check for Python analysis framework
	if fileContains("/proc/1/cgroup", "cuckoo") {
		return true
	}

	return false
}

// checkCgroupSandbox detects sandbox through cgroup analysis
func checkCgroupSandbox() bool {
	// Check if running in restricted cgroup
	content, err := os.ReadFile("/proc/self/cgroup")
	if err == nil {
		cgroupStr := string(content)
		// If cgroup indicates container/sandbox
		if strings.Contains(cgroupStr, "docker") ||
			strings.Contains(cgroupStr, "lxc") ||
			strings.Contains(cgroupStr, "systemd") ||
			strings.Contains(cgroupStr, "sandbox") {
			return true
		}
	}
	return false
}

// checkSystemdNspawnContainer detects systemd-nspawn container
func checkSystemdNspawnContainer() bool {
	// Check for container-specific environment
	if os.Getenv("container") == "systemd-nspawn" {
		return true
	}

	// Check /proc/self/cgroup for nspawn
	if fileContains("/proc/self/cgroup", "systemd") && !isSystemdHost() {
		return true
	}

	return false
}

// checkVMwareLinux detects VMware on Linux
func checkVMwareLinux() bool {
	// Check DMI product name for VMware
	if fileContains("/sys/class/dmi/id/product_name", "VMware") {
		return true
	}

	// Check system serial number
	if fileContains("/sys/class/dmi/id/system_serial", "VMware") {
		return true
	}

	// Check /proc/scsi/scsi for VMware SCSI adapter
	if fileContains("/proc/scsi/scsi", "VMware") {
		return true
	}

	return false
}

// checkVirtualBoxLinux detects VirtualBox on Linux
func checkVirtualBoxLinux() bool {
	// Check DMI product name for VirtualBox
	if fileContains("/sys/class/dmi/id/product_name", "VirtualBox") {
		return true
	}

	// Check for VirtualBox Guest Additions
	if _, err := os.Stat("/opt/VBoxGuestAdditions"); err == nil {
		return true
	}

	// Check for vboxguest module
	if fileContains("/proc/modules", "vboxguest") {
		return true
	}

	return false
}

// checkXenLinux detects Xen hypervisor on Linux
func checkXenLinux() bool {
	// Check for Xen dom0/domU
	if _, err := os.Stat("/proc/xen"); err == nil {
		return true
	}

	// Check for Xen in DMI
	if fileContains("/sys/class/dmi/id/system_product", "Xen") {
		return true
	}

	// Check for xenbus module
	if fileContains("/proc/modules", "xenbus") {
		return true
	}

	return false
}

// Helper functions

func fileContains(filePath, search string) bool {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(string(content)), strings.ToLower(search))
}

func checkCgroupContains(search string) bool {
	content, err := os.ReadFile("/proc/self/cgroup")
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(string(content)), strings.ToLower(search))
}

func checkHostnamePattern(pattern string) bool {
	// Simplified hostname pattern check
	hostname, err := os.Hostname()
	if err != nil {
		return false
	}
	// Docker typically uses 12-char hex strings
	if len(hostname) == 12 && strings.ContainsAny(hostname, "abcdef0123456789") {
		return true
	}
	return false
}

func processExists(processName string) bool {
	cmd := exec.Command("pgrep", processName)
	err := cmd.Run()
	return err == nil
}

func checkCPUIDKVM() bool {
	// Would use CPUID instruction to check for KVM hypervisor
	// Stub implementation
	return false
}

func isSystemdHost() bool {
	// Check if this is the host systemd
	init, err := os.ReadFile("/proc/1/comm")
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(init)) == "systemd"
}

// isSandboxedWindows is a stub for Linux builds
func isSandboxedWindows() bool {
	return false
}

// isSandboxedDarwin is a stub for Linux builds
func isSandboxedDarwin() bool {
	return false
}

// detectSandboxTypeWindows is a stub for Linux builds
func detectSandboxTypeWindows() (string, int) {
	return "", 0
}

// detectSandboxTypeDarwin is a stub for Linux builds
func detectSandboxTypeDarwin() (string, int) {
	return "", 0
}

// getSandboxIndicatorsWindows is a stub for Linux builds
func getSandboxIndicatorsWindows() []string {
	return nil
}

// getSandboxIndicatorsDarwin is a stub for Linux builds
func getSandboxIndicatorsDarwin() []string {
	return nil
}
