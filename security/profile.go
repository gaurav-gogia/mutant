package security

import (
	"crypto/sha256"
	"os"
	"strings"
)

const ProtectionProfileEnv = "MUTANT_PROTECTION_PROFILE"

const (
	ProtectionProfileMinimal  = "minimal"
	ProtectionProfileStandard = "standard"
	ProtectionProfileParanoid = "paranoid"
	defaultProtectionProfile  = ProtectionProfileStandard
)

func ResolveProtectionProfile() string {
	configured := strings.ToLower(strings.TrimSpace(os.Getenv(ProtectionProfileEnv)))
	switch configured {
	case ProtectionProfileMinimal, ProtectionProfileStandard, ProtectionProfileParanoid:
		return configured
	default:
		return defaultProtectionProfile
	}
}

func ResolveProtectionProfileCode() byte {
	switch ResolveProtectionProfile() {
	case ProtectionProfileMinimal:
		return 1
	case ProtectionProfileStandard:
		return 2
	case ProtectionProfileParanoid:
		return 3
	default:
		return 2
	}
}

func ProtectionProfileFromCode(code byte) (string, bool) {
	switch code {
	case 1:
		return ProtectionProfileMinimal, true
	case 2:
		return ProtectionProfileStandard, true
	case 3:
		return ProtectionProfileParanoid, true
	default:
		return "", false
	}
}

func defaultTamperResponseForProfile(secureMode bool) string {
	switch ResolveProtectionProfile() {
	case ProtectionProfileMinimal:
		return TamperResponseWarn
	case ProtectionProfileParanoid:
		return TamperResponseTerminate
	default:
		if secureMode {
			return TamperResponseTerminate
		}
		return TamperResponseWarn
	}
}

func DefaultBuiltinCapabilityPolicy() map[string]struct{} {
	switch ResolveProtectionProfile() {
	case ProtectionProfileMinimal:
		return map[string]struct{}{"all": {}}
	default:
		return map[string]struct{}{}
	}
}

func DeriveStandaloneProvenance(payload []byte, checksum []byte, profileCode byte) [32]byte {
	seed := make([]byte, 0, len(payload)+len(checksum)+1)
	seed = append(seed, payload...)
	seed = append(seed, checksum...)
	seed = append(seed, profileCode)
	return sha256.Sum256(seed)
}
