package lua

import (
	"bytes"
	"fmt"
	"sync"
	"time"

	lua "github.com/Shopify/go-lua"
)

// SandboxConfig defines the isolation and resource constraints for a Lua VM instance.
type SandboxConfig struct {
	MaxMemoryMB    int
	MaxExecutionMS int
	AllowedLibs    []string // "math", "string", "table" only
}

// SandboxedVM wraps a Lua state with security constraints.
type SandboxedVM struct {
	state         *lua.State
	config        SandboxConfig
	mu            sync.Mutex
	executionDone chan struct{}
	chunkLoaded   bool
}

// DefaultSandboxConfig returns a secure default configuration.
func DefaultSandboxConfig() SandboxConfig {
	return SandboxConfig{
		MaxMemoryMB:    64,
		MaxExecutionMS: 5000,
		AllowedLibs:    []string{"math", "string", "table"},
	}
}

// NewSandboxedVM creates a new isolated Lua environment with the specified configuration.
// The VM is NOT open/ready for execution until Open() is called.
func NewSandboxedVM(config SandboxConfig) *SandboxedVM {
	return &SandboxedVM{
		config:        config,
		executionDone: make(chan struct{}),
		chunkLoaded:   false,
	}
}

// Open initializes the Lua state and loads only the whitelisted standard libraries.
func (vm *SandboxedVM) Open() error {
	vm.mu.Lock()
	defer vm.mu.Unlock()

	if vm.state != nil {
		return fmt.Errorf("sandbox already open")
	}

	vm.state = lua.NewState()
	if vm.state == nil {
		return fmt.Errorf("failed to create Lua state")
	}

	// Load only whitelisted standard libraries.
	for _, libName := range vm.config.AllowedLibs {
		switch libName {
		case "math":
			lua.Require(vm.state, "math", lua.MathOpen, true)
		case "string":
			lua.Require(vm.state, "string", lua.StringOpen, true)
		case "table":
			lua.Require(vm.state, "table", lua.TableOpen, true)
		case "base":
			lua.Require(vm.state, "_G", lua.BaseOpen, true)
		default:
			return fmt.Errorf("unsupported library: %s", libName)
		}
	}

	// Defense in depth: always strip dangerous globals even if loaded by mistake.
	stripUnsafeGlobals(vm.state)

	return nil
}

// LoadBytecode loads Lua bytecode into the VM.
// bytecode should be plaintext Lua 5.2 bytecode (e.g., from luac -o bytecode.lua script.lua).
func (vm *SandboxedVM) LoadBytecode(bytecode []byte, chunkName string) error {
	vm.mu.Lock()
	defer vm.mu.Unlock()

	if vm.state == nil {
		return fmt.Errorf("sandbox not open")
	}

	if len(bytecode) == 0 {
		return fmt.Errorf("empty lua bytecode")
	}

	if chunkName == "" {
		chunkName = "mutant_lua_patch"
	}

	err := vm.state.Load(bytes.NewReader(bytecode), chunkName, "bt")
	if err != nil {
		return fmt.Errorf("failed to load lua bytecode: %w", err)
	}

	vm.chunkLoaded = true
	return nil
}

// Execute runs the loaded Lua chunk with timeout enforcement.
// Returns the result or error. Result is converted to string for safety.
func (vm *SandboxedVM) Execute() (string, error) {
	vm.mu.Lock()
	defer vm.mu.Unlock()

	if vm.state == nil {
		return "", fmt.Errorf("sandbox not open")
	}

	if !vm.chunkLoaded {
		return "", fmt.Errorf("no loaded chunk")
	}

	if vm.config.MaxExecutionMS > 0 {
		deadline := time.Now().Add(time.Duration(vm.config.MaxExecutionMS) * time.Millisecond)
		lua.SetDebugHook(vm.state, func(l *lua.State, _ lua.Debug) {
			if time.Now().After(deadline) {
				lua.Errorf(l, "execution timeout")
				panic("unreachable")
			}
		}, lua.MaskCount, 1000)
		defer lua.SetDebugHook(vm.state, nil, 0, 0)
	}

	err := vm.state.ProtectedCall(0, 1, 0)
	if err != nil {
		return "", fmt.Errorf("lua error: %w", err)
	}

	// Get result from stack
	result, ok := vm.state.ToString(-1)
	if !ok {
		vm.state.Pop(1)
		return "", nil
	}
	vm.state.Pop(1)
	vm.chunkLoaded = false

	return result, nil
}

// Close cleans up the Lua state and frees all resources.
func (vm *SandboxedVM) Close() error {
	vm.mu.Lock()
	defer vm.mu.Unlock()

	if vm.state != nil {
		vm.state.SetTop(0)
		vm.state = nil
		vm.chunkLoaded = false
	}

	return nil
}

// GetState returns the underlying Lua state for advanced operations (for internal use only).
func (vm *SandboxedVM) GetState() *lua.State {
	vm.mu.Lock()
	defer vm.mu.Unlock()
	return vm.state
}

func stripUnsafeGlobals(l *lua.State) {
	unsafeNames := []string{
		"os",
		"io",
		"debug",
		"package",
		"require",
		"dofile",
		"load",
		"loadfile",
		"loadstring",
		"collectgarbage",
	}

	for _, name := range unsafeNames {
		l.PushNil()
		l.SetGlobal(name)
	}
}
