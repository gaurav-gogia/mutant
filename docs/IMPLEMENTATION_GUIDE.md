# Security Enhancement Implementation Guide

This guide provides step-by-step instructions for implementing the security enhancements in Mutant.

## Prerequisites

First, add required dependencies to go.mod:

```bash
go get golang.org/x/crypto/argon2
go get golang.org/x/crypto/hkdf
```

## Phase 1: Critical Security Fixes (Immediate)

### 1.1 Fix Insecure Random Number Generation

**File**: `security/crypto.go`

Replace the `randByte` function:

```go
// OLD - INSECURE
func randByte(seed int64) byte {
	src := mathRand.NewSource(seed)
	newrand := mathRand.New(src)
	number := newrand.Int()
	return byte(number)
}

// NEW - Use secure_random.go functions instead
```

Update XOR functions to use `SecureXOR`:

```go
// In crypto.go, update XOR and XOROne to use secure versions
// Or better, import from secure_random.go
```

### 1.2 Replace MD5 with SHA-256

**File**: `security/signatures.go`

```go
// OLD
func SignCode(encodedString string) []byte {
	integrity := md5.New().Sum([]byte(encodedString))
	// ...
}

// NEW
func SignCode(encodedString string) []byte {
	integrity := sha256.Sum256([]byte(encodedString))
	integString := hex.EncodeToString(integrity[:])
	// ...
}

// Update VerifyCode similarly
func VerifyCode(signedCode []byte) error {
	// ...
	integrity := sha256.Sum256([]byte(values[1]))
	integString := hex.EncodeToString(integrity[:])
	// ...
}
```

### 1.3 Fix Key Storage

**File**: `security/crypto.go`

The current `AESEncrypt` stores the key alongside ciphertext. For backward compatibility, keep it but add warnings:

```go
// Add comment warning
// DEPRECATED: This function stores keys insecurely.
// Use crypto_improved.go functions instead.
func AESEncrypt(data []byte) (string, error) {
	// Keep existing implementation for compatibility
}
```

## Phase 2: Add Password-Based Encryption

### 2.1 Update CLI to Accept Password

**File**: `cli/cli.go` (create if doesn't exist or update main.go)

Add password flag support:

```go
func CompileCodeWithPassword(srcpath, password string) {
	data, err := ioutil.ReadFile(srcpath)
	if err != nil {
		// handle error
	}

	var key []byte
	var kdfParams *security.KDFParams

	if password != "" {
		// Use password-based encryption
		sourceHash := security.HashSourceCode(data)
		key, kdfParams, err = security.DeriveKeyFromPassword(password, sourceHash)
		if err != nil {
			// handle error
		}
	} else {
		// Use deterministic encryption
		sourceHash := security.HashSourceCode(data)
		metadata := security.GenerateMetadata(srcpath, global.VERSION)
		key, kdfParams, err = security.DeriveKeyDeterministic(sourceHash, metadata)
		if err != nil {
			// handle error
		}
	}

	// Pass key and kdfParams to generator
	// Store kdfParams in bytecode metadata
}
```

### 2.2 Update Generator

**File**: `generator/generate.go`

Add support for using provided keys:

```go
func GenerateWithKey(srcpath, dstpath string, key []byte, kdfParams *security.KDFParams) error {
	// ... existing compile logic ...

	// In encode function, use the provided key
	encodedByteCode, err := encodeWithKey(comp.ByteCode(), key, kdfParams)
	// ...
}

func encodeWithKey(compByteCode *compiler.ByteCode, key []byte, kdfParams *security.KDFParams) ([]byte, error) {
	var content bytes.Buffer

	compByteCode = mutil.EncryptByteCode(compByteCode)

	registerTypes()
	enc := gob.NewEncoder(&content)
	if err := enc.Encode(compByteCode); err != nil {
		return nil, err
	}

	byteCode := content.Bytes()

	// Use improved encryption with provided key
	ciphertext, metadata, err := security.AESEncryptWithKey(byteCode, key)
	if err != nil {
		return nil, err
	}

	// Store KDF params in metadata
	metadata.KDFParams = kdfParams.Encode()

	// Encode with metadata
	encoded := security.EncodeEncryptedData(ciphertext, metadata)
	signedCode := security.SignCodeImproved(encoded)

	return signedCode, nil
}
```

### 2.3 Update Runner

**File**: `runner/runner.go`

Add password verification:

```go
func RunWithPassword(srcpath, password string) (error, errrs.ErrorType) {
	signedCode, err := ioutil.ReadFile(srcpath)
	if err != nil {
		return err, errrs.ERROR
	}

	// Verify signature
	if err := security.VerifyCode(signedCode); err != nil {
		return err, errrs.ERROR
	}

	// Decode to get metadata
	ciphertext, metadata, err := security.DecodeEncryptedData(
		security.GetEncryptedCode(signedCode),
	)
	if err != nil {
		return err, errrs.ERROR
	}

	// Reconstruct key from password and KDF params
	kdfParams, err := security.DecodeParams(metadata.KDFParams)
	if err != nil {
		return err, errrs.ERROR
	}

	var key []byte
	if kdfParams.Algorithm == "argon2id" {
		// Password required
		if password == "" {
			return errors.New("password required"), errrs.ERROR
		}
		key, err = security.ReconstructKey(password, kdfParams)
	} else {
		// Deterministic - reconstruct from stored params
		key, err = security.ReconstructKey("", kdfParams)
	}
	if err != nil {
		return err, errrs.ERROR
	}

	// Decrypt bytecode
	decrypted, err := security.AESDecryptWithKey(ciphertext, key, metadata)
	if err != nil {
		return err, errrs.ERROR
	}

	// Continue with normal execution...
}
```

## Phase 3: Memory Security

### 3.1 Protect Global Objects

**File**: `global/global.go`

```go
package global

import (
	"mutant/object"
	"mutant/security"
	"sync"
)

var (
	// Use secure wrappers
	secureTrue  *object.SecureGlobal
	secureFalse *object.SecureGlobal
	secureNull  *object.SecureGlobal

	initOnce sync.Once
	seed     int64 = 12345 // Change per session
)

// Initialize creates secure global objects
func Initialize() {
	initOnce.Do(func() {
		trueObj := &object.Boolean{Value: true}
		falseObj := &object.Boolean{Value: false}
		nullObj := &object.Null{}

		secureTrue, _ = object.NewSecureGlobal(trueObj, seed)
		secureFalse, _ = object.NewSecureGlobal(falseObj, seed)
		secureNull, _ = object.NewSecureGlobal(nullObj, seed)
	})
}

// GetTrue returns the decrypted True object
func GetTrue() object.Object {
	Initialize()
	obj, _ := secureTrue.Get()
	return obj
}

// GetFalse returns the decrypted False object
func GetFalse() object.Object {
	Initialize()
	obj, _ := secureFalse.Get()
	return obj
}

// GetNull returns the decrypted Null object
func GetNull() object.Object {
	Initialize()
	obj, _ := secureNull.Get()
	return obj
}

// Cleanup securely wipes global objects
func Cleanup() {
	if secureTrue != nil {
		secureTrue.Clear()
	}
	if secureFalse != nil {
		secureFalse.Clear()
	}
	if secureNull != nil {
		secureNull.Clear()
	}
}
```

Update all references from `global.True` to `global.GetTrue()`, etc.

### 3.2 Implement Auto-Encrypting Stack (Optional - Performance Impact)

**File**: `vm/vm.go`

```go
// Add to VM struct
type VM struct {
	// ... existing fields ...
	secureStack *object.SecureStack
}

// In New function
func New(bc *compiler.ByteCode) *VM {
	// ... existing code ...

	vm := &VM{
		// ... existing fields ...
		secureStack: object.NewSecureStack(global.StackSize, int64(len(bc.Instructions))),
	}

	return vm
}

// Add periodic auto-protect call
func (vm *VM) Run() error {
	var ip int
	var ins code.Instructions
	var op code.Opcode

	instructionCount := 0

	for vm.currentFrame().ip < len(vm.currentFrame().Instructions())-1 {
		vm.currentFrame().ip++

		// Auto-protect stack every 100 instructions
		instructionCount++
		if instructionCount%100 == 0 {
			vm.secureStack.AutoProtect()
		}

		// ... rest of execution ...
	}

	// Clean up on exit
	vm.secureStack.Clear()
	global.Cleanup()

	return nil
}
```

## Phase 4: Polymorphic Bytecode

### 4.1 Integrate Polymorphic Engine

**File**: `generator/generate.go`

```go
func Generate(srcpath, dstpath, goos, goarch string, release bool, mutationLevel int) (error, errrs.ErrorType, []string) {
	// ... existing compile logic ...

	bytecode, err, errtype, errors := compile(data)
	if err != nil {
		return err, errtype, errors
	}

	// Apply polymorphic transformations
	if mutationLevel > 0 {
		engine := compiler.NewPolymorphicEngine(mutationLevel, time.Now().UnixNano())
		bytecodeObj := deserializeBytecode(bytecode) // Implement this
		bytecodeObj = engine.Mutate(bytecodeObj)
		bytecode = serializeBytecode(bytecodeObj) // Implement this
	}

	// ... rest of generation ...
}
```

### 4.2 Add CLI Flag

**File**: `main.go`

```go
releasecmd.IntVar(&mutationLevel, "mutation", 5, "Polymorphic mutation level (0-10)")
releasecmd.StringVar(&password, "password", "", "Password for bytecode encryption")
```

## Phase 5: Anti-Debugging (Platform Specific)

### 5.1 Create Anti-Debug Module

**File**: `security/antidebug.go`

```go
// +build !android

package security

import (
	"runtime"
)

// IsDebuggerPresent checks if a debugger is attached
func IsDebuggerPresent() bool {
	switch runtime.GOOS {
	case "windows":
		return isDebuggerPresentWindows()
	case "linux":
		return isDebuggerPresentLinux()
	case "darwin":
		return isDebuggerPresentDarwin()
	default:
		return false
	}
}

// Platform-specific implementations in separate files
```

**File**: `security/antidebug_windows.go`

```go
// +build windows

package security

import "syscall"

func isDebuggerPresentWindows() bool {
	// Use Windows API
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	proc := kernel32.NewProc("IsDebuggerPresent")
	ret, _, _ := proc.Call()
	return ret != 0
}
```

### 5.2 Integrate Anti-Debug Check

**File**: `vm/vm.go`

```go
func (vm *VM) Run() error {
	// Check for debugger at start
	if security.IsDebuggerPresent() && !global.AllowDebug {
		return errors.New("debugging not permitted")
	}

	// ... rest of Run ...
}
```

## Testing

### Unit Tests

Create `security/security_test.go`:

```go
func TestKDFDeterministic(t *testing.T) {
	source := []byte("let x = 5;")
	hash := HashSourceCode(source)

	key1, _, _ := DeriveKeyDeterministic(hash, "test")
	key2, _, _ := DeriveKeyDeterministic(hash, "test")

	if !bytes.Equal(key1, key2) {
		t.Error("Deterministic KDF should produce same key")
	}
}

func TestPasswordKDF(t *testing.T) {
	password := "mypassword"
	salt := make([]byte, 32)
	rand.Read(salt)

	key, params, err := DeriveKeyFromPassword(password, salt)
	if err != nil {
		t.Fatal(err)
	}

	// Reconstruct key
	key2, err := ReconstructKey(password, params)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(key, key2) {
		t.Error("Password KDF reconstruction failed")
	}
}
```

## Migration Path

1. **v2.2.0**: Add new security features, keep old ones for compatibility
2. **v2.3.0**: Default to new security, provide flag `--legacy-crypto` for old format
3. **v3.0.0**: Remove old crypto entirely

## Performance Considerations

- Argon2id: ~50-100ms overhead at compile/run time
- Polymorphic mutations: 5-15% bytecode size increase
- Memory security: 10-20% runtime overhead (optional, disable with flag)

## Configuration Example

**File**: `mutant.security.toml` (optional)

```toml
[security]
level = "high"
password_required = false
mutation_level = 5
anti_debug = true

[memory]
auto_encrypt_stack = false  # Disable for performance
clear_on_exit = true
```

## Summary of Files Created/Modified

### New Files
- ✅ `security/kdf.go` - Key derivation functions
- ✅ `security/signing.go` - Ed25519 code signing
- ✅ `security/secure_random.go` - Secure random generation
- ✅ `security/crypto_improved.go` - Improved encryption
- ✅ `compiler/polymorphic.go` - Polymorphic bytecode engine
- ✅ `object/secure_memory.go` - Memory security primitives
- ✅ `security/antidebug.go` - Anti-debugging (platform-specific)

### Modified Files (Required)
- `security/crypto.go` - Add deprecation warnings
- `security/signatures.go` - Replace MD5 with SHA-256
- `global/global.go` - Wrap in SecureGlobal
- `generator/generate.go` - Add KDF and polymorphic support
- `runner/runner.go` - Add password verification
- `main.go` - Add CLI flags
- `vm/vm.go` - Add anti-debug checks, memory cleanup

### Dependencies to Add
```bash
go get golang.org/x/crypto/argon2
go get golang.org/x/crypto/hkdf
```

## Next Steps

1. Install dependencies
2. Implement Phase 1 (critical fixes) first
3. Test thoroughly with existing code
4. Add Phase 2 (password support) as optional feature
5. Benchmark performance impact
6. Add comprehensive tests
7. Update documentation
8. Consider security audit

For questions or issues, refer to `SECURITY_ENHANCEMENTS.md` for detailed explanations.
