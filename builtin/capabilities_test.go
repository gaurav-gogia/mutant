package builtin

import (
	"os"
	"strings"
	"testing"

	"mutant/object"
	"mutant/security"
)

func TestMinimalProfileAllowsRiskyBuiltinsByDefault(t *testing.T) {
	t.Setenv(BuiltinCapabilitiesEnv, "")
	t.Setenv(security.ProtectionProfileEnv, security.ProtectionProfileMinimal)

	if result := FsExists(&object.String{Value: t.TempDir()}); func() bool {
		_, ok := result.(*object.Error)
		return ok
	}() {
		t.Fatalf("expected minimal profile to allow filesystem builtin default behavior")
	}

	if result := NetResolve(&object.String{Value: "localhost"}); func() bool {
		_, ok := result.(*object.Error)
		return ok
	}() {
		t.Fatalf("expected minimal profile to allow network builtin default behavior")
	}
}

func TestFilesystemBuiltinsBlockedWithoutCapability(t *testing.T) {
	t.Setenv(BuiltinCapabilitiesEnv, "")

	result := FsRead(&object.String{Value: "does-not-matter.txt"})
	if _, ok := result.(*object.Error); !ok {
		t.Fatalf("expected filesystem builtin to be blocked, got=%T", result)
	}

	if !strings.Contains(result.(*object.Error).Message, CapabilityFilesystem) {
		t.Fatalf("expected filesystem capability mention, got=%q", result.(*object.Error).Message)
	}
}

func TestFilesystemBuiltinsAllowedWithCapability(t *testing.T) {
	t.Setenv(BuiltinCapabilitiesEnv, CapabilityFilesystem)

	filePath := t.TempDir() + string(os.PathSeparator) + "payload.txt"
	if result := FsWrite(&object.String{Value: filePath}, &object.String{Value: "mutant"}); func() bool {
		_, ok := result.(*object.Hash)
		return ok
	}() == false {
		t.Fatalf("expected fs_write to return hash")
	}

	result := FsRead(&object.String{Value: filePath})
	value, ok := result.(*object.String)
	if !ok {
		t.Fatalf("expected filesystem builtin to be allowed, got=%T", result)
	}
	if value.Value != "mutant" {
		t.Fatalf("unexpected file contents: %q", value.Value)
	}
}

func TestNetworkBuiltinsBlockedWithoutCapability(t *testing.T) {
	t.Setenv(BuiltinCapabilitiesEnv, "")

	result := NetResolve(&object.String{Value: "localhost"})
	if _, ok := result.(*object.Error); !ok {
		t.Fatalf("expected network builtin to be blocked, got=%T", result)
	}

	if !strings.Contains(result.(*object.Error).Message, CapabilityNetwork) {
		t.Fatalf("expected network capability mention, got=%q", result.(*object.Error).Message)
	}
}

func TestNetworkBuiltinsAllowedWithCapability(t *testing.T) {
	t.Setenv(BuiltinCapabilitiesEnv, CapabilityNetwork)

	result := NetDial(&object.String{Value: "127.0.0.1:1"}, &object.Integer{Value: 1})
	if _, ok := result.(*object.Hash); !ok {
		t.Fatalf("expected network builtin to be allowed, got=%T", result)
	}
}
