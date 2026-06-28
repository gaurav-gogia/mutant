package security

import (
	"runtime"
	"strings"
)

type sandboxDetection struct {
	Type       string
	Confidence int
	Indicators []string
}

const sandboxDetectedThreshold = 70

// IsSandboxed returns true when environment signals indicate a high-confidence
// container, virtualization, or sandboxed runtime.
func IsSandboxed() bool {
	_, confidence, err := DetectSandboxType()
	if err != nil {
		return false
	}
	return confidence >= sandboxDetectedThreshold
}

// DetectSandboxType returns the most likely sandbox type and confidence (0-100).
func DetectSandboxType() (string, int, error) {
	result, err := detectSandbox()
	if err != nil {
		securityDevLogf("sandbox detect error=%v", err)
		return "none", 0, err
	}
	if result.Confidence < 0 {
		result.Confidence = 0
	}
	if result.Confidence > 100 {
		result.Confidence = 100
	}
	if result.Type == "" || result.Confidence == 0 {
		securityDevLogf("sandbox detected=false type=none confidence=0 indicators=")
		return "none", 0, nil
	}
	securityDevLogf(
		"sandbox detected=%t type=%s confidence=%d indicators=%s",
		result.Confidence >= sandboxDetectedThreshold,
		result.Type,
		result.Confidence,
		strings.Join(result.Indicators, ","),
	)
	return result.Type, result.Confidence, err
}

// GetSandboxIndicators returns normalized detection indicators for audit/logging.
func GetSandboxIndicators() ([]string, error) {
	result, err := detectSandbox()
	if err != nil {
		return nil, err
	}
	if len(result.Indicators) == 0 {
		return nil, nil
	}

	indicators := make([]string, len(result.Indicators))
	copy(indicators, result.Indicators)
	return indicators, nil
}

func detectSandbox() (sandboxDetection, error) {
	switch runtime.GOOS {
	case "windows":
		return detectSandboxWindows()
	case "linux":
		return detectSandboxLinux()
	case "darwin":
		return detectSandboxDarwin()
	default:
		return sandboxDetection{}, nil
	}
}
