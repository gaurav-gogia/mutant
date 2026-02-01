//go:build darwin
// +build darwin

package security

import (
	"os"
	"os/exec"
	"strings"
)

// isSandboxedDarwin performs multiple sandbox/VM detection techniques on macOS
func isSandboxedDarwin() bool {
	// Check for VM/Sandbox indicators
	if checkVirtualMachineHardwareDarwin() {
		return true
	}
	if checkMacOSAppSandbox() {
		return true
	}
	if checkParallelsDarwin() {
		return true
	}
	if checkVMwareDarwin() {
		return true
	}
	if checkVirtualBoxDarwin() {
		return true
	}
	if checkUTMVirtualMachine() {
		return true
	}
	if checkColima() {
		return true
	}
	if checkQemuDarwin() {
		return true
	}

	return false
}

// detectSandboxTypeDarwin identifies the specific sandbox/VM type on macOS
func detectSandboxTypeDarwin() (string, int) {
	// macOS App Sandbox detection
	if checkMacOSAppSandbox() {
		return "macOS App Sandbox", 95
	}
	// Parallels detection
	if checkParallelsDarwin() {
		return "Parallels", 90
	}
	// VMware detection
	if checkVMwareDarwin() {
		return "VMware Fusion", 85
	}
	// VirtualBox detection
	if checkVirtualBoxDarwin() {
		return "VirtualBox", 80
	}
	// UTM detection
	if checkUTMVirtualMachine() {
		return "UTM", 85
	}
	// Colima detection
	if checkColima() {
		return "Colima", 80
	}
	// QEMU detection
	if checkQemuDarwin() {
		return "QEMU", 75
	}

	return "", 0
}

// getSandboxIndicatorsDarwin returns all detected indicators
func getSandboxIndicatorsDarwin() []string {
	var indicators []string

	if checkMacOSAppSandbox() {
		indicators = append(indicators, "macOS App Sandbox detected")
	}
	if checkParallelsDarwin() {
		indicators = append(indicators, "Parallels Desktop detected")
	}
	if checkVMwareDarwin() {
		indicators = append(indicators, "VMware Fusion detected")
	}
	if checkVirtualBoxDarwin() {
		indicators = append(indicators, "VirtualBox detected")
	}
	if checkUTMVirtualMachine() {
		indicators = append(indicators, "UTM Virtual Machine detected")
	}
	if checkColima() {
		indicators = append(indicators, "Colima container detected")
	}
	if checkQemuDarwin() {
		indicators = append(indicators, "QEMU detected")
	}

	return indicators
}

// checkVirtualMachineHardwareDarwin checks for VM indicators on macOS
func checkVirtualMachineHardwareDarwin() bool {
	return checkParallelsDarwin() || checkVMwareDarwin() || checkVirtualBoxDarwin() || checkQemuDarwin()
}

// checkMacOSAppSandbox detects macOS App Sandbox environment
func checkMacOSAppSandbox() bool {
	// Check for sandbox environment variable
	if os.Getenv("APP_SANDBOX_READ_ONLY_HOME") != "" {
		return true
	}
	if os.Getenv("APP_SANDBOX_READ_ONLY_PREFERENCES_DOMAIN") != "" {
		return true
	}

	// Check HOME environment for sandbox container path
	home := os.Getenv("HOME")
	if strings.Contains(home, "Containers") && strings.Contains(home, "Data") {
		return true
	}

	// Check for sandbox libraries
	libsandbox := "/usr/lib/system/libsystem_sandbox.dylib"
	if _, err := os.Stat(libsandbox); err == nil {
		// File exists, but also check if we have sandbox entitlements
		if hasSandboxEntitlements() {
			return true
		}
	}

	return false
}

// checkParallelsDarwin detects Parallels Desktop on macOS
func checkParallelsDarwin() bool {
	// Check for Parallels Tools
	parallelsTools := []string{
		"/Applications/Parallels Desktop.app",
		"/Library/Parallels/Tools",
		"/System/Library/Extensions/prl_eth.kext",
		"/Library/LaunchDaemons/com.parallels.vm.prl_tools.plist",
	}

	for _, path := range parallelsTools {
		if _, err := os.Stat(path); err == nil {
			return true
		}
	}

	// Check system info for Parallels
	if sysctlContains("hw.product_name", "Parallels") {
		return true
	}

	// Check for Parallels processes
	if processRunningDarwin("prl_tools_service") || processRunningDarwin("parallels") {
		return true
	}

	return false
}

// checkVMwareDarwin detects VMware Fusion on macOS
func checkVMwareDarwin() bool {
	// Check for VMware Fusion application
	vmwarePaths := []string{
		"/Applications/VMware Fusion.app",
		"/Library/Preferences/VMware",
		"/usr/local/bin/vmware-tray",
	}

	for _, path := range vmwarePaths {
		if _, err := os.Stat(path); err == nil {
			return true
		}
	}

	// Check system info for VMware
	if sysctlContains("hw.product_name", "VMware") || sysctlContains("hw.product_name", "VMW") {
		return true
	}

	// Check for VMware-specific processes
	if processRunningDarwin("vmware") || processRunningDarwin("vmtoolsd") {
		return true
	}

	return false
}

// checkVirtualBoxDarwin detects VirtualBox on macOS
func checkVirtualBoxDarwin() bool {
	// Check for VirtualBox application and files
	vboxPaths := []string{
		"/Applications/VirtualBox.app",
		"/usr/local/bin/VBoxManage",
		"/Library/Extensions/VirtualBoxKEXT.kext",
	}

	for _, path := range vboxPaths {
		if _, err := os.Stat(path); err == nil {
			return true
		}
	}

	// Check for VirtualBox Guest Additions
	if _, err := os.Stat("/opt/VBoxGuestAdditions-7.0.0/bin"); err == nil {
		return true
	}

	// Check processes
	if processRunningDarwin("VBoxService") || processRunningDarwin("VBoxClient") {
		return true
	}

	return false
}

// checkUTMVirtualMachine detects UTM Virtual Machine
func checkUTMVirtualMachine() bool {
	// Check for UTM application
	if _, err := os.Stat("/Applications/UTM.app"); err == nil {
		return true
	}

	// Check for UTM guest agent process
	if processRunningDarwin("utm") || processRunningDarwin("qemu") {
		return true
	}

	// Check UTM VM environment variables
	if os.Getenv("QEMU_SYSTEM") != "" {
		return true
	}

	return false
}

// checkColima detects Colima container environment
func checkColima() bool {
	// Check for Colima Docker socket
	colimaSocket := "/Users/" + os.Getenv("USER") + "/.colima/docker.sock"
	if _, err := os.Stat(colimaSocket); err == nil {
		return true
	}

	// Check for Colima VM
	colimaVM := "/Users/" + os.Getenv("USER") + "/.colima"
	if _, err := os.Stat(colimaVM); err == nil {
		// Check if it's actually being used
		if processRunningDarwin("colima") {
			return true
		}
	}

	return false
}

// checkQemuDarwin detects QEMU on macOS
func checkQemuDarwin() bool {
	// Check for QEMU installation
	qemuPaths := []string{
		"/usr/local/bin/qemu-system-x86_64",
		"/opt/homebrew/bin/qemu-system-x86_64",
	}

	for _, path := range qemuPaths {
		if _, err := os.Stat(path); err == nil {
			// Just having QEMU installed doesn't mean we're in it
			// Check for running QEMU process
			if processRunningDarwin("qemu") {
				return true
			}
		}
	}

	// Check CPUID for QEMU hypervisor
	if checkCPUIDQEMU() {
		return true
	}

	return false
}

// Helper functions

func hasSandboxEntitlements() bool {
	// Check if current process has sandbox entitlements
	// Would require reading and parsing the app's entitlements plist
	// Stub implementation
	return false
}

func sysctlContains(key, search string) bool {
	cmd := exec.Command("sysctl", "-n", key)
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(string(output)), strings.ToLower(search))
}

func processRunningDarwin(processName string) bool {
	cmd := exec.Command("pgrep", "-x", processName)
	err := cmd.Run()
	return err == nil
}

func checkCPUIDQEMU() bool {
	// Would use CPUID to detect QEMU hypervisor signature
	// Stub implementation
	return false
}

// GetSandboxConfinementType returns the confinement level for macOS sandboxed apps
func GetSandboxConfinementType() string {
	if checkMacOSAppSandbox() {
		// Would inspect entitlements to determine confinement level
		return "strict"
	}
	return "none"
}

// isSandboxedWindows is a stub for Darwin builds
func isSandboxedWindows() bool {
	return false
}

// isSandboxedLinux is a stub for Darwin builds
func isSandboxedLinux() bool {
	return false
}

// detectSandboxTypeWindows is a stub for Darwin builds
func detectSandboxTypeWindows() (string, int) {
	return "", 0
}

// detectSandboxTypeLinux is a stub for Darwin builds
func detectSandboxTypeLinux() (string, int) {
	return "", 0
}

// getSandboxIndicatorsWindows is a stub for Darwin builds
func getSandboxIndicatorsWindows() []string {
	return nil
}

// getSandboxIndicatorsLinux is a stub for Darwin builds
func getSandboxIndicatorsLinux() []string {
	return nil
}
