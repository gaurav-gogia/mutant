package security

import (
	"fmt"
	"os"
	"strings"
)

const SecurityDevLogEnv = "MUTANT_SECURITY_DEV_LOG"

func securityDevLogEnabled() bool {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv(SecurityDevLogEnv)))
	switch raw {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func securityDevLogf(format string, args ...any) {
	if !securityDevLogEnabled() {
		return
	}
	fmt.Fprintf(os.Stderr, "[security-dev] "+format+"\n", args...)
}
