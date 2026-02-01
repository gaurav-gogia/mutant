//go:build windows
// +build windows

package security

import (
	"os"
	"strings"
	"syscall"
)

var (
	kernel32WMI            = syscall.NewLazyDLL("wmi.dll")
	kernel32WinReg         = syscall.NewLazyDLL("advapi32.dll")
	kernel32SystemInfo     = syscall.NewLazyDLL("kernel32.dll")
	procGetComputerNameEx  = kernel32.NewProc("GetComputerNameExW")
	procGetDiskFreeSpaceEx = kernel32.NewProc("GetDiskFreeSpaceExW")
	procGetSystemMetrics   = kernel32.NewProc("GetSystemMetrics")
	procGetSystemInfo      = kernel32SystemInfo.NewProc("GetSystemInfo")
	procGetProcessorCount  = kernel32.NewProc("GetActiveProcessorCount")
)

// isSandboxedWindows performs multiple sandbox/VM detection techniques on Windows
func isSandboxedWindows() bool {
	// Check for VM/Sandbox indicators
	if checkVirtualMachineHardware() {
		return true
	}
	if checkWindowsDefenderSandbox() {
		return true
	}
	if checkCuckooCuckooEnvironment() {
		return true
	}
	if checkCommonSandboxFiles() {
		return true
	}
	if checkSuspiciousSystemMetrics() {
		return true
	}
	if checkRegistryVMIndicators() {
		return true
	}
	if checkProcessNamesSandbox() {
		return true
	}
	if checkVirtualNetworkAdapters() {
		return true
	}

	return false
}

// detectSandboxTypeWindows identifies the specific sandbox/VM type
func detectSandboxTypeWindows() (string, int) {
	// VMware detection
	if checkVMware() {
		return "VMware", 90
	}
	// VirtualBox detection
	if checkVirtualBox() {
		return "VirtualBox", 85
	}
	// Hyper-V detection
	if checkHyperV() {
		return "Hyper-V", 80
	}
	// KVM/QEMU detection
	if checkKVMQEMU() {
		return "KVM/QEMU", 75
	}
	// Windows Defender Sandbox
	if checkWindowsDefenderSandbox() {
		return "Windows Defender Sandbox", 95
	}
	// Parallels detection
	if checkParallels() {
		return "Parallels", 80
	}
	// Xen detection
	if checkXen() {
		return "Xen", 75
	}
	// Cuckoo Sandbox
	if checkCuckooCuckooEnvironment() {
		return "Cuckoo", 90
	}
	// Sandboxie detection
	if checkSandboxie() {
		return "Sandboxie", 85
	}
	// WINE detection
	if checkWINE() {
		return "WINE", 80
	}

	return "", 0
}

// getSandboxIndicatorsWindows returns all detected indicators
func getSandboxIndicatorsWindows() []string {
	var indicators []string

	if checkVMware() {
		indicators = append(indicators, "VMware detected")
	}
	if checkVirtualBox() {
		indicators = append(indicators, "VirtualBox detected")
	}
	if checkHyperV() {
		indicators = append(indicators, "Hyper-V detected")
	}
	if checkKVMQEMU() {
		indicators = append(indicators, "KVM/QEMU detected")
	}
	if checkWindowsDefenderSandbox() {
		indicators = append(indicators, "Windows Defender Sandbox detected")
	}
	if checkParallels() {
		indicators = append(indicators, "Parallels detected")
	}
	if checkXen() {
		indicators = append(indicators, "Xen detected")
	}
	if checkCuckooCuckooEnvironment() {
		indicators = append(indicators, "Cuckoo Sandbox detected")
	}
	if checkSandboxie() {
		indicators = append(indicators, "Sandboxie detected")
	}
	if checkWINE() {
		indicators = append(indicators, "WINE detected")
	}
	if checkCommonSandboxFiles() {
		indicators = append(indicators, "Suspicious sandbox files detected")
	}
	if checkProcessNamesSandbox() {
		indicators = append(indicators, "Sandbox-related processes detected")
	}

	return indicators
}

// checkVirtualMachineHardware checks for VM indicators in system
func checkVirtualMachineHardware() bool {
	return checkVMware() || checkVirtualBox() || checkHyperV() || checkKVMQEMU() || checkParallels() || checkXen()
}

// checkVMware detects VMware environment
func checkVMware() bool {
	// Check registry for VMware
	if checkRegistryValue("HKLM", `SOFTWARE\VMware, Inc.`) {
		return true
	}

	// Check for VMware process
	if osCommandExists("vmtoolsd.exe") || osCommandExists("vmware-tray.exe") {
		return true
	}

	// Check for VMware DLLs
	if checkDLLLoaded("vmGuestLib.dll") || checkDLLLoaded("vmhgfs.dll") {
		return true
	}

	// Check for VMware MAC address prefix (00:0C:29, 00:50:F2, 00:05:69)
	if checkVirtualMACAddress([]string{"00:0c:29", "00:50:f2", "00:05:69"}) {
		return true
	}

	// Check processor brand
	if strings.Contains(strings.ToLower(getProcessorBrand()), "vmware") {
		return true
	}

	return false
}

// checkVirtualBox detects VirtualBox environment
func checkVirtualBox() bool {
	// Check registry
	if checkRegistryValue("HKLM", `SOFTWARE\Oracle\VirtualBox Guest Additions`) {
		return true
	}

	// Check for VirtualBox specific files
	if fileExists("C:\\Program Files\\Oracle\\VirtualBox Guest Additions\\") {
		return true
	}
	if fileExists("C:\\Program Files (x86)\\Oracle\\VirtualBox Guest Additions\\") {
		return true
	}

	// Check for VirtualBox processes
	if osCommandExists("VBoxService.exe") || osCommandExists("VBoxTray.exe") {
		return true
	}

	// Check for VirtualBox DLLs
	if checkDLLLoaded("VBoxMRXNP.dll") || checkDLLLoaded("VBoxOGL.dll") {
		return true
	}

	// Check MAC address (08:00:27)
	if checkVirtualMACAddress([]string{"08:00:27"}) {
		return true
	}

	// Check processor brand
	if strings.Contains(strings.ToLower(getProcessorBrand()), "virtualbox") {
		return true
	}

	return false
}

// checkHyperV detects Hyper-V environment
func checkHyperV() bool {
	// Check for Hyper-V specific registry values
	if checkRegistryValue("HKLM", `SOFTWARE\Microsoft\Hyper-V`) {
		return true
	}

	// Check for Hyper-V specific files
	if fileExists("C:\\Windows\\System32\\hvconfig.exe") {
		return true
	}

	// Check CPUID for hypervisor
	if checkCPUIDHypervisor() {
		return true
	}

	// Check processor brand
	if strings.Contains(strings.ToLower(getProcessorBrand()), "hyper-v") {
		return true
	}

	// Check for Hyper-V processes
	if osCommandExists("vmms.exe") {
		return true
	}

	return false
}

// checkKVMQEMU detects KVM/QEMU environment
func checkKVMQEMU() bool {
	// Check processor brand
	if strings.Contains(strings.ToLower(getProcessorBrand()), "kvm") {
		return true
	}
	if strings.Contains(strings.ToLower(getProcessorBrand()), "qemu") {
		return true
	}

	// Check for QEMU specific processes (if running under WSL)
	if osCommandExists("qemu-ga.exe") {
		return true
	}

	return false
}

// checkParallels detects Parallels Desktop environment
func checkParallels() bool {
	// Check registry for Parallels
	if checkRegistryValue("HKLM", `SOFTWARE\Parallels`) {
		return true
	}

	// Check for Parallels processes
	if osCommandExists("prl_cc.exe") || osCommandExists("prl_tools_service.exe") {
		return true
	}

	// Check MAC address (00:1C:42)
	if checkVirtualMACAddress([]string{"00:1c:42"}) {
		return true
	}

	return false
}

// checkXen detects Xen environment
func checkXen() bool {
	// Check processor brand
	if strings.Contains(strings.ToLower(getProcessorBrand()), "xen") {
		return true
	}

	// Check for xenbus registry
	if checkRegistryValue("HKLM", `SYSTEM\CurrentControlSet\Services\xenbus`) {
		return true
	}

	return false
}

// checkWindowsDefenderSandbox detects Windows Defender Sandbox
func checkWindowsDefenderSandbox() bool {
	// Check for Device Guard/HVCI indicators
	if checkRegistryValue("HKLM", `SYSTEM\CurrentControlSet\Control\DeviceGuard\Scenarios\HypervisorEnforcedCodeIntegrity`) {
		return true
	}

	// Check for Windows Defender Application Guard
	if fileExists("C:\\Windows\\System32\\wdagutilx.exe") {
		return true
	}

	// Check environment variables
	if os.Getenv("SANDBOXED_ENV") == "1" {
		return true
	}

	// Check for MPAS sandbox environment
	if strings.Contains(os.Getenv("Path"), "mpas") {
		return true
	}

	return false
}

// checkSandboxie detects Sandboxie environment
func checkSandboxie() bool {
	// Check registry for Sandboxie
	if checkRegistryValue("HKLM", `SYSTEM\CurrentControlSet\Services\SbieDrv`) {
		return true
	}

	// Check for Sandboxie DLL
	if checkDLLLoaded("sbiedll.dll") {
		return true
	}

	// Check for Sandboxie files
	if fileExists("C:\\Program Files\\Sandboxie\\") || fileExists("C:\\Program Files (x86)\\Sandboxie\\") {
		return true
	}

	// Check environment
	if os.Getenv("SANDBOXIE") != "" {
		return true
	}

	return false
}

// checkWINE detects WINE environment
func checkWINE() bool {
	// Check registry for WINE
	if checkRegistryValue("HKCU", `Software\Wine`) {
		return true
	}

	// Check for wine-preloader
	if fileExists("C:\\windows\\system32\\wine-preloader") {
		return true
	}

	// Check environment variables
	if os.Getenv("WINELOADER") != "" {
		return true
	}
	if os.Getenv("WINEPRELOAD") != "" {
		return true
	}

	return false
}

// checkCuckooCuckooEnvironment detects Cuckoo Sandbox
func checkCuckooCuckooEnvironment() bool {
	// Check for Cuckoo agent
	if fileExists("C:\\cuckoo\\") {
		return true
	}

	// Check for Python analysis environment
	if fileExists("C:\\Python27\\") || fileExists("C:\\Python36\\") {
		// Combined with other indicators
		if checkProcessNamesSandbox() {
			return true
		}
	}

	// Check for Cuckoo monitor DLL
	if checkDLLLoaded("cuckoomon.dll") {
		return true
	}

	// Check for Cuckoo specific environment
	if os.Getenv("CUCKOO") != "" {
		return true
	}

	return false
}

// checkRegistryVMIndicators checks registry for VM/sandbox indicators
func checkRegistryVMIndicators() bool {
	indicators := []string{
		`HKLM\SYSTEM\CurrentControlSet\Control\Class\{4D36E968-E325-11CE-BFC1-08002BE10318}\0`,
		`HKLM\HARDWARE\DEVICEMAP\Scsi\Scsi Port 0\Scsi Bus 0\Target Id 0\Logical Unit Id 0`,
	}

	for _, ind := range indicators {
		if checkRegistryValue("", ind) {
			if strings.Contains(ind, "VMWARE") || strings.Contains(ind, "VBOX") || strings.Contains(ind, "QEMU") {
				return true
			}
		}
	}

	return false
}

// checkCommonSandboxFiles checks for files commonly found in sandbox environments
func checkCommonSandboxFiles() bool {
	sandboxPaths := []string{
		"C:\\analyzer\\",
		"C:\\cuckoo\\",
		"C:\\sandboxed\\",
		"C:\\sandbox\\",
	}

	for _, path := range sandboxPaths {
		if fileExists(path) {
			return true
		}
	}

	return false
}

// checkProcessNamesSandbox checks for sandbox-related processes
func checkProcessNamesSandbox() bool {
	sandboxProcesses := []string{
		"winafl",
		"FILEMON",
		"REGMON",
		"NETMON",
		"sample.exe",
		"analysis",
		"malware",
		"bin",
		"guest",
	}

	for _, proc := range sandboxProcesses {
		if osCommandExists(proc + ".exe") {
			return true
		}
	}

	return false
}

// checkVirtualNetworkAdapters checks for virtual network adapter indicators
func checkVirtualNetworkAdapters() bool {
	// Check registry for virtual adapter descriptions
	// This would typically check network adapter registry entries
	// Simplified version - in production, enumerate network adapters
	if checkRegistryValue("HKLM", `SYSTEM\CurrentControlSet\Control\Class\{4D36E972-E325-11CE-BFC1-08002BE10318}`) {
		// Found network adapters registry key - would need to enumerate
		return true
	}

	return false
}

// checkSuspiciousSystemMetrics checks for suspicious system metric values
func checkSuspiciousSystemMetrics() bool {
	// Check for unrealistically low processor count (indicator of VM)
	if getProcessorCount() < 1 {
		return true
	}

	// Check for unrealistically low RAM
	if getSystemRAM() < 512*1024*1024 { // Less than 512 MB
		return true
	}

	// Check for low disk space in analysis environment
	if getFreeDiskSpace() < 100*1024*1024 { // Less than 100 MB
		return true
	}

	return false
}

// Helper functions

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func osCommandExists(cmd string) bool {
	// Simplified - would use Windows API to check running processes
	return false // Stub
}

func checkDLLLoaded(dllName string) bool {
	// Would use GetModuleHandle to check if DLL is loaded
	// Stub implementation
	return false
}

func checkRegistryValue(root, path string) bool {
	// Stub implementation - would use Windows registry API
	return false
}

func checkVirtualMACAddress(prefixes []string) bool {
	// Would enumerate network adapters and check MAC address prefixes
	// Stub implementation
	return false
}

func getProcessorBrand() string {
	// Would use CPUID to get processor brand
	// Stub implementation
	return ""
}

func checkCPUIDHypervisor() bool {
	// Would check CPUID for hypervisor presence
	// Stub implementation
	return false
}

func getProcessorCount() int {
	// Would return actual processor count
	return 1
}

func getSystemRAM() uint64 {
	// Would return actual system RAM
	return 4 * 1024 * 1024 * 1024 // 4GB default
}

func getFreeDiskSpace() uint64 {
	// Would return actual free disk space
	return 100 * 1024 * 1024 * 1024 // 100GB default
}

// isSandboxedLinux is a stub for Windows builds
func isSandboxedLinux() bool {
	return false
}

// isSandboxedDarwin is a stub for Windows builds
func isSandboxedDarwin() bool {
	return false
}

// detectSandboxTypeLinux is a stub for Windows builds
func detectSandboxTypeLinux() (string, int) {
	return "", 0
}

// detectSandboxTypeDarwin is a stub for Windows builds
func detectSandboxTypeDarwin() (string, int) {
	return "", 0
}

// getSandboxIndicatorsLinux is a stub for Windows builds
func getSandboxIndicatorsLinux() []string {
	return nil
}

// getSandboxIndicatorsDarwin is a stub for Windows builds
func getSandboxIndicatorsDarwin() []string {
	return nil
}
