//go:build !windows
// +build !windows

package security

func detectSandboxWindows() (sandboxDetection, error) {
	return sandboxDetection{}, nil
}
