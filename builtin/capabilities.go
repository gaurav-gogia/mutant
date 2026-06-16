package builtin

import (
	"os"
	"strings"

	"mutant/object"
	"mutant/security"
)

const BuiltinCapabilitiesEnv = "MUTANT_BUILTIN_CAPABILITIES"

const CapabilityCommandExec = "command_exec"
const CapabilityFilesystem = "filesystem"
const CapabilityNetwork = "network"

func requireBuiltinCapability(capability string, stage string) (bool, string) {
	if capability == "" {
		return true, "allowed"
	}

	allowed := builtinCapabilitySet()
	if _, ok := allowed["all"]; ok {
		return true, "allowed"
	}
	if _, ok := allowed[capability]; ok {
		return true, "allowed"
	}

	security.RecordCommandBlocked(stage)
	return false, "blocked_missing_capability"
}

func builtinCapabilitySet() map[string]struct{} {
	configured := strings.TrimSpace(strings.ToLower(strings.ReplaceAll(os.Getenv(BuiltinCapabilitiesEnv), ",", " ")))
	if configured == "" {
		return security.DefaultBuiltinCapabilityPolicy()
	}

	result := make(map[string]struct{})
	for _, entry := range strings.Fields(configured) {
		result[entry] = struct{}{}
	}
	return result
}

func blockedCommandResult(decision, message string) object.Object {
	return commandResultHash(security.CommandResult{
		Allowed:        false,
		PolicyDecision: decision,
		ErrorMessage:   message,
	})
}

func blockedCapabilityError(capability, decision string) object.Object {
	return newError("builtin capability %q blocked (%s)", capability, decision)
}
