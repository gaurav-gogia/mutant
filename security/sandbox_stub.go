package security

// Platform-specific stub implementations for build tag compatibility
// These are only compiled on platforms where the actual implementation doesn't exist

// isSandboxedWindowsStub is a stub for non-Windows platforms
func isSandboxedWindowsStub() bool {
	return false
}

// isSandboxedLinuxStub is a stub for non-Linux platforms
func isSandboxedLinuxStub() bool {
	return false
}

// isSandboxedDarwinStub is a stub for non-macOS platforms
func isSandboxedDarwinStub() bool {
	return false
}

// detectSandboxTypeWindowsStub is a stub for non-Windows platforms
func detectSandboxTypeWindowsStub() (string, int) {
	return "", 0
}

// detectSandboxTypeLinuxStub is a stub for non-Linux platforms
func detectSandboxTypeLinuxStub() (string, int) {
	return "", 0
}

// detectSandboxTypeDarwinStub is a stub for non-macOS platforms
func detectSandboxTypeDarwinStub() (string, int) {
	return "", 0
}

// getSandboxIndicatorsWindowsStub is a stub for non-Windows platforms
func getSandboxIndicatorsWindowsStub() []string {
	return nil
}

// getSandboxIndicatorsLinuxStub is a stub for non-Linux platforms
func getSandboxIndicatorsLinuxStub() []string {
	return nil
}

// getSandboxIndicatorsDarwinStub is a stub for non-macOS platforms
func getSandboxIndicatorsDarwinStub() []string {
	return nil
}
