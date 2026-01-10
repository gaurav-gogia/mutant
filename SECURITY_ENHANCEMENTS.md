# Mutant Security Enhancements Plan

## Executive Summary
This document outlines comprehensive security enhancements for the Mutant programming language, focusing on usable security where users don't need to worry about implementation details.

---

## 1. ENCRYPTION & KEY DERIVATION

### Current Issues
- **Key derivation uses SHA256 of data itself** (`security/crypto.go:16`) - this is deterministic but weak
- **Key is stored alongside ciphertext** in the same structure (crypto.go:38-39)
- **No user password/passphrase integration**
- **No key derivation function (KDF)** - vulnerable to brute force
- **MD5 used for integrity** (signatures.go) - cryptographically broken

### Proposed Solution: Hybrid Approach

#### Option A: Pure Deterministic (Recommended for CLI tools)
**Advantages:**
- No password prompts needed
- Works for automated build systems
- Reproducible builds
- User-friendly for simple scripts

**Implementation:**
```go
// Use deterministic key derivation from source code + metadata
key = HKDF-SHA256(
    source_code_hash,
    salt = project_name + version + timestamp_rounded_to_day,
    info = "mutant-bytecode-encryption-v1"
)
```

#### Option B: Password-Based (For sensitive applications)
**Advantages:**
- Stronger security
- User controls access
- Can prevent unauthorized execution

**Implementation:**
```go
// Use Argon2id (winner of Password Hashing Competition)
key = Argon2id(
    password,
    salt = source_code_hash,
    time=1, memory=64MB, threads=4
)
```

#### Option C: Hybrid (RECOMMENDED)
```go
// Compile time: optional password flag
mutant release -src app.mut -password "secret"  // or prompt

// Runtime: verify password if set, otherwise use deterministic
mutant app.mu -password "secret"  // or prompt
```

### Implementation Plan

1. **Replace MD5 with SHA-256 for integrity** (immediate)
2. **Implement Argon2id KDF** for password mode
3. **Add HKDF-SHA256** for deterministic mode
4. **Store KDF parameters** (not keys!) in bytecode metadata
5. **Separate key material** from encrypted data

---

## 2. MEMORY SECURITY

### Current Issues Identified

#### A. Global Objects Remain Unencrypted
- `global/global.go` has singleton True/False/Null objects
- These are referenced throughout execution and never cleared
- Live in memory for entire process lifetime

#### B. Stack Objects Partially Protected
- Local variables are encrypted (`vm.go:179-186`)
- But immediately decrypted on access (`vm.go:196-200`)
- Decrypted values stay on stack until overwritten

#### C. Constants Never Encrypted at Runtime
- Constants decrypted once and stay in `vm.constants[]`
- String literals, numbers all exposed in memory

### Proposed Solutions

#### 2.1 Global Object Protection
```go
// Create encrypted global references that decrypt on-demand
type SecureGlobal struct {
    encryptedValue []byte
    objectType     object.ObjectType
    lastAccess     time.Time
}

func (s *SecureGlobal) Get(key []byte) object.Object {
    // Decrypt only when accessed
    // Auto-encrypt after use
}
```

#### 2.2 Memory Zeroing
```go
// Add explicit memory clearing
func (vm *VM) clearSensitiveData(obj object.Object) {
    switch v := obj.(type) {
    case *object.String:
        // Zero out string memory
        for i := range v.Value {
            v.Value[i] = 0
        }
    case *object.Integer:
        v.Value = 0
    }
}
```

#### 2.3 Stack Guard Implementation
```go
// Implement automatic stack encryption
type SecureStack struct {
    data       []object.Object
    autoEncrypt bool
    lastTouch   []time.Time
}

// Auto-encrypt items not accessed in last N instructions
func (s *SecureStack) autoProtect() {
    for i, lastUse := range s.lastTouch {
        if time.Since(lastUse) > threshold {
            s.data[i] = encrypt(s.data[i])
        }
    }
}
```

#### 2.4 Constant Pool Protection
```go
// Lazy decryption of constants
type SecureConstantPool struct {
    encrypted [][]byte
    cache     map[int]object.Object
    cacheSize int
}

func (s *SecureConstantPool) Get(idx int) object.Object {
    if obj, ok := s.cache[idx]; ok {
        return obj
    }

    obj := decrypt(s.encrypted[idx])

    // LRU cache with auto-cleanup
    if len(s.cache) > s.cacheSize {
        s.evictOldest()
    }

    s.cache[idx] = obj
    return obj
}
```

---

## 3. POLYMORPHIC/METAMORPHIC BYTECODE

### Concept
Generate functionally equivalent but structurally different bytecode on each compilation to:
- Prevent signature-based detection
- Thwart static analysis
- Make reverse engineering harder

### Implementation Strategies

#### 3.1 Instruction Polymorphism
```go
// Multiple ways to achieve same result
// Example: x = 5 can be:
// - OpConstant 5
// - OpConstant 3, OpConstant 2, OpAdd
// - OpConstant 10, OpConstant 2, OpDiv
```

#### 3.2 NOP Insertion
```go
// Insert random no-operation instructions
type NOPInstruction byte

const (
    NOP_PUSH_POP     // Push then immediately pop
    NOP_XOR_SELF     // XOR value with itself twice
    NOP_ADD_ZERO     // Add zero
    NOP_MUL_ONE      // Multiply by one
)

func insertRandomNOPs(instructions []byte) []byte {
    // Randomly insert NOPs at safe points
}
```

#### 3.3 Instruction Reordering
```go
// Reorder independent instructions
// Before: OpConstant 5, OpConstant 10, OpAdd
// After:  OpConstant 10, OpConstant 5, OpAdd (order doesn't matter)

func reorderInstructions(insts []byte) []byte {
    // Analyze data dependencies
    // Reorder where safe
}
```

#### 3.4 Opcode Mutation
```go
// Change opcode values per compilation
type OpcodeMap map[Opcode]Opcode

func generateRandomOpcodeMap() OpcodeMap {
    opcodes := []Opcode{OpAdd, OpSub, OpMul, ...}
    shuffled := shuffle(opcodes)

    mapping := make(OpcodeMap)
    for i, op := range opcodes {
        mapping[op] = shuffled[i]
    }
    return mapping
}

// Store mapping in encrypted bytecode header
```

#### 3.5 Register Allocation Randomization
```go
// Use different local variable slots on each compile
func randomizeLocals(compiler *Compiler) {
    // Shuffle symbol table indices
    // Maintain correctness but change layout
}
```

#### 3.6 Control Flow Obfuscation
```go
// Add fake branches that never execute
func addBogusBranches(instructions []byte) []byte {
    // if (always_false) { dead_code }
    // Analyzer can't determine it's dead without execution
}
```

### Metamorphic Code Generation

```go
type CodeMutator struct {
    mutationLevel int // 0-10 scale
    randomSeed    int64
}

func (m *CodeMutator) Mutate(bytecode *ByteCode) *ByteCode {
    stages := []MutationStage{
        insertNOPs,           // 5-10% code inflation
        reorderInstructions,  // Safe reordering
        mutateOpcodes,        // Remap opcodes
        insertDeadCode,       // 2-5% bogus branches
        randomizeConstants,   // Shuffle constant pool
        encryptWithRandomKey, // Different encryption each time
    }

    for _, stage := range stages {
        bytecode = stage(bytecode, m.randomSeed)
    }

    return bytecode
}
```

---

## 4. ADDITIONAL SECURITY ENHANCEMENTS

### 4.1 Anti-Debugging
```go
// Detect debugger presence
func detectDebugger() bool {
    // Check for common debugger artifacts
    // - ptrace detection on Linux
    // - IsDebuggerPresent on Windows
    // - Timing checks
}

// Add to VM initialization
if detectDebugger() && !allowDebug {
    return errors.New("debugging not allowed")
}
```

### 4.2 Code Signing & Verification
```go
// Use proper digital signatures (not just MD5)
// - Ed25519 or ECDSA for signing
// - Verify signature before execution

type CodeSignature struct {
    PublicKey  []byte
    Signature  []byte
    Algorithm  string // "Ed25519"
    Timestamp  int64
}

func SignBytecode(bytecode []byte, privateKey []byte) *CodeSignature
func VerifyBytecode(bytecode []byte, sig *CodeSignature) bool
```

### 4.3 Sandboxing
```go
// Restrict VM capabilities based on policy
type SecurityPolicy struct {
    AllowFileIO      bool
    AllowNetworkIO   bool
    AllowExec        bool
    MaxMemory        int64
    MaxExecutionTime time.Duration
}

func (vm *VM) EnforcePolicy(policy *SecurityPolicy) {
    // Check before each potentially dangerous operation
}
```

### 4.4 Secure Random Generation
```go
// Replace math/rand with crypto/rand for security operations
// Current issue: security/crypto.go:104 uses math/rand with predictable seed

func secureRandByte() byte {
    b := make([]byte, 1)
    cryptoRand.Read(b)
    return b[0]
}
```

### 4.5 Constant-Time Operations
```go
// Prevent timing attacks on cryptographic operations
import "crypto/subtle"

func secureCompare(a, b []byte) bool {
    return subtle.ConstantTimeCompare(a, b) == 1
}
```

### 4.6 Source Code Hash Verification
```go
// Store hash of original source in bytecode
// Verify at runtime that bytecode matches expected source

type BytecodeMetadata struct {
    SourceHash    [32]byte // SHA-256 of source
    CompileTime   int64
    CompilerVer   string
    MutationSeed  int64
}
```

### 4.7 Runtime Integrity Checks
```go
// Periodically verify VM state hasn't been tampered with
func (vm *VM) checkIntegrity() bool {
    // Verify instruction checksums
    // Check stack boundaries
    // Validate frame pointers
}
```

### 4.8 Secure Deletion
```go
// Implement secure deletion for temporary files
func secureDelete(path string) error {
    // Overwrite file multiple times before deletion
    file, _ := os.OpenFile(path, os.O_WRONLY, 0)

    info, _ := file.Stat()
    size := info.Size()

    // 3-pass overwrite (DoD 5220.22-M standard)
    patterns := []byte{0xFF, 0x00, 0xAA}
    for _, pattern := range patterns {
        file.Seek(0, 0)
        data := bytes.Repeat([]byte{pattern}, int(size))
        file.Write(data)
        file.Sync()
    }

    file.Close()
    return os.Remove(path)
}
```

---

## 5. IMPLEMENTATION PRIORITY

### Phase 1 (High Priority - Security Fixes)
1. ✅ Replace MD5 with SHA-256 for integrity
2. ✅ Fix crypto/rand usage (remove math/rand from security code)
3. ✅ Implement proper key derivation (Argon2id)
4. ✅ Separate keys from ciphertext
5. ✅ Add code signing with Ed25519

### Phase 2 (Medium Priority - Memory Security)
1. ✅ Implement secure global objects
2. ✅ Add memory zeroing
3. ✅ Implement stack guard with auto-encryption
4. ✅ Secure constant pool with LRU cache

### Phase 3 (Advanced - Polymorphic Code)
1. ✅ Basic NOP insertion
2. ✅ Instruction reordering
3. ✅ Opcode mutation
4. ✅ Control flow obfuscation

### Phase 4 (Hardening)
1. ✅ Anti-debugging
2. ✅ Sandboxing policies
3. ✅ Runtime integrity checks
4. ✅ Secure file deletion

---

## 6. USABILITY CONSIDERATIONS

### Keep It Simple for Users

```bash
# Basic usage - no password, automatic security
mutant myapp.mut          # Compile with default security
mutant myapp.mu           # Run

# Enhanced security - optional password
mutant release -src myapp.mut -secure-password
# Prompts: "Enter password for bytecode encryption:"
# Prompts: "Confirm password:"

mutant myapp.mu -secure-password
# Prompts: "Enter password:"

# Advanced - full control
mutant release -src myapp.mut \
    -security-level high \      # Enables all protections
    -anti-debug \               # Enable anti-debugging
    -mutation-level 7 \         # Polymorphic strength (0-10)
    -password "secret"          # Or use password file

# For CI/CD - deterministic mode
mutant release -src myapp.mut \
    -security-level medium \
    -deterministic              # No random mutations, reproducible
```

### Configuration File Support

```toml
# mutant.security.toml
[security]
level = "high"              # low, medium, high, paranoid
anti_debug = true
mutation_level = 7
password_mode = "optional"  # none, optional, required

[memory]
auto_encrypt_stack = true
clear_on_exit = true
constant_pool_size = 100

[sandbox]
allow_file_io = true
allow_network = false
max_memory_mb = 512
max_execution_sec = 300
```

---

## 7. BACKWARD COMPATIBILITY

- Keep `-o` flag for output path
- Keep existing `.mu` and `.mut` extensions
- Add version number to bytecode format
- Old VMs reject new bytecode gracefully
- New VMs support old bytecode (legacy mode)

---

## 8. TESTING STRATEGY

```go
// Security test suite
func TestEncryptionStrength(t *testing.T)
func TestMemoryLeaks(t *testing.T)
func TestPolymorphicVariability(t *testing.T)
func TestAntiDebugging(t *testing.T)
func TestPasswordValidation(t *testing.T)
func BenchmarkEncryptionOverhead(b *testing.B)
```

---

## 9. DOCUMENTATION NEEDS

1. Security whitepaper
2. Threat model documentation
3. Best practices guide
4. API documentation for security features
5. Examples of secure coding patterns

---

## CONCLUSION

These enhancements provide defense-in-depth while maintaining usability. The user doesn't need to understand cryptography - they just run `mutant` and get secure execution automatically. Advanced users can tune settings via CLI flags or config files.

**Key Principle: Security by Default, Flexibility by Choice**
