package lua

import (
	"testing"

	"mutant/global"

	lua "github.com/yuin/gopher-lua"
)

// mutant.version() answered "2.1.0", the version it was written in, on every
// release since.
func TestMutantVersionIsTheRunningVersion(t *testing.T) {
	v := NewSandboxedVM(SandboxConfig{MaxMemoryMB: 64, MaxExecutionMS: 1000, AllowedLibs: []string{"base"}})
	if err := v.Open(); err != nil {
		t.Fatalf("failed to open sandbox: %v", err)
	}
	defer v.Close()
	if err := RegisterMutantAPI(v, &APIContext{}); err != nil {
		t.Fatalf("RegisterMutantAPI: %v", err)
	}

	state := v.GetState()
	if err := state.DoString("answer = mutant.version()"); err != nil {
		t.Fatalf("mutant.version(): %v", err)
	}
	if got := state.GetGlobal("answer"); got != lua.LString(global.Version) {
		t.Fatalf("mutant.version() = %v, want %q", got, global.Version)
	}
}
