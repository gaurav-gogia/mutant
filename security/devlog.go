package security

import (
	"fmt"
	"os"
	"strings"
)

const SecurityDevLogEnv = "MUTANT_SECURITY_DEV_LOG"
const SecurityLogLevelEnv = "MUTANT_SECURITY_LOG_LEVEL"
const SecurityDevModeEnv = "MUTANT_DEV_MODE"

const (
	securityLogLevelNone  = 0
	securityLogLevelError = 1
	securityLogLevelInfo  = 2
	securityLogLevelDebug = 3
	securityLogLevelTrace = 4
)

func boolEnvTrue(name string) bool {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv(name)))
	switch raw {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func securityDevModeEnabled() bool {
	return boolEnvTrue(SecurityDevModeEnv)
}

func resolveSecurityLogLevel() int {
	if boolEnvTrue(SecurityDevLogEnv) {
		// Backward compatibility for the old binary switch.
		return securityLogLevelDebug
	}

	raw := strings.TrimSpace(strings.ToLower(os.Getenv(SecurityLogLevelEnv)))
	switch raw {
	case "", "none", "off", "silent":
		return securityLogLevelNone
	case "error", "err":
		return securityLogLevelError
	case "info":
		return securityLogLevelInfo
	case "debug":
		return securityLogLevelDebug
	case "trace":
		return securityLogLevelTrace
	default:
		return securityLogLevelNone
	}
}

func securityDevLogEnabled() bool {
	if !securityDevModeEnabled() {
		return false
	}
	return resolveSecurityLogLevel() >= securityLogLevelDebug
}

func securityDevLogf(format string, args ...any) {
	if !securityDevLogEnabled() {
		return
	}
	fmt.Fprintf(os.Stderr, "[security-dev] "+format+"\n", args...)
}
