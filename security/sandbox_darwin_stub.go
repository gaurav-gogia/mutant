//go:build !darwin
// +build !darwin

package security

func detectSandboxDarwin() (sandboxDetection, error) {
	return sandboxDetection{}, nil
}
