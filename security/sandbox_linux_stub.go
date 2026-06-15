//go:build !linux
// +build !linux

package security

func detectSandboxLinux() (sandboxDetection, error) {
	return sandboxDetection{}, nil
}
