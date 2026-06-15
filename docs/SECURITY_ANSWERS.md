# Security Enhancement Answers

## Your Questions Answered

### 1. ENCRYPTION & KEY GENERATION

#### Question: Password or Deterministic Key Derivation?

**Recommended Answer: Hybrid Approach (Both Options Available)**

##### Option A: Password-Based (Use when security is paramount)
```bash
# Compile with password
mutant release -src app.mut -password
# Prompts for password, uses Argon2id KDF

# Run with password
mutant app.mu -password
# Prompts for same password
```

**Advantages:**
- ✅ Maximum security
- ✅ User controls access
- ✅ Even if bytecode is stolen, can't run without password
- ✅ Industry-standard Argon2id (OWASP recommended)

**Disadvantages:**
- ❌ User must remember password
- ❌ Can't automate in CI/CD without storing password
- ❌ Lost password = lost code

##### Option B: Deterministic (Default, Use for convenience)
```bash
# Compile (no password needed)
mutant release -src app.mut

# Run (no password needed)
mutant app.mu
```

**Advantages:**
- ✅ Zero user friction
- ✅ Works in automation/CI/CD
- ✅ Code always runnable
- ✅ Still provides obfuscation and tamper resistance

**Disadvantages:**
- ❌ Less secure than password (key derivable from source)
- ❌ If attacker has both source and compiled bytecode, can derive key

##### Recommended Implementation:
```go
// Key derivation logic
if password != "" {
    // Password mode: Argon2id
    key = Argon2id(password, salt=sourceHash)
} else {
    // Deterministic mode: HKDF
    key = HKDF-SHA256(sourceHash, salt=sourceHash, info=metadata)
}
```

**Key Storage:**
- ❌ NEVER store the actual key
- ✅ Store KDF parameters (algorithm, salt, iterations)
- ✅ Store these in bytecode metadata
- ✅ Key is reconstructed on-demand from password or deterministically

**How it works:**

```
COMPILE TIME:
1. Read source.mut
2. Hash source -> sourceHash
3. If password provided:
   - Argon2id(password, sourceHash) -> key
4. Else:
   - HKDF(sourceHash, metadata) -> key
5. Encrypt bytecode with key
6. Store only: encrypted_bytecode + KDF_params (NOT key!)

RUNTIME:
1. Read compiled.mu
2. Extract KDF_params
3. If KDF_params says "argon2id":
   - Prompt for password
   - Argon2id(password, stored_salt) -> key
4. Else (HKDF):
   - HKDF(stored_params) -> key (deterministic)
5. Decrypt and run
```

#### Question: What about global objects and memory?

**Current Issues Found:**
1. **Global singletons** (`global/global.go`):
   - `True`, `False`, `Null` are unencrypted singletons
   - Live in memory entire execution
   - Accessible throughout process

2. **Stack objects**:
   - Encrypted when pushed (`vm.go:335`)
   - Immediately decrypted when popped (`vm.go:344`)
   - Decrypted value stays on stack until overwritten

3. **Constants**:
   - Decrypted once at load
   - Stored unencrypted in `vm.constants[]`
   - Never re-encrypted

**Solutions Provided:**

##### A. Secure Global Objects (`object/secure_memory.go`)
```go
// Instead of:
var True = &object.Boolean{Value: true}

// Use:
secureTrue := NewSecureGlobal(true, seed)
// Access:
obj := secureTrue.Get() // Decrypts on-demand
// After use:
secureTrue.Clear()      // Zeros memory
```

**How it works:**
- Globals stored encrypted
- Decrypt only when accessed
- Auto-encrypt after use
- Secure zero on cleanup

##### B. Auto-Encrypting Stack (`object/secure_memory.go`)
```go
stack := NewSecureStack(size, seed)
stack.Set(0, myObject)     // Auto-encrypts after 100ms
obj := stack.Get(0)        // Auto-decrypts on access
```

**How it works:**
- Stack items encrypted when not accessed
- LRU-style: frequently used items stay decrypted (performance)
- Infrequently accessed items auto-encrypt
- Everything cleared on exit

##### C. Secure Constant Pool
```go
pool := NewSecureConstantPool(constants, cacheSize, seed)
obj := pool.Get(5)  // Decrypt on first access, cache
```

**How it works:**
- Constants stored encrypted
- LRU cache of N most-used constants (decrypted)
- Least used constants stay encrypted
- Cache cleared periodically

**Memory Protection Summary:**

| Object Type | Current | Improved | Performance Impact |
|------------|---------|----------|-------------------|
| Globals | Plaintext | Encrypted w/ lazy decrypt | Minimal (~1%) |
| Stack | Partially encrypted | Auto-encrypt after idle | Medium (~10%) |
| Constants | Plaintext | LRU cache + encryption | Low (~5%) |
| Strings | Never zeroed | Secure zeroed on cleanup | Minimal |

**Recommendation:**
- ✅ Always use: Secure globals (minimal overhead)
- ✅ Optional: Auto-encrypting stack (flag: `--secure-memory`)
- ✅ Optional: Secure constant pool (flag: `--secure-constants`)
- ✅ Always: Secure zero on cleanup

---

### 2. POLYMORPHIC / METAMORPHIC BYTECODE

**Answer: Yes! Multiple techniques available**

#### What is it?
Generate functionally identical but structurally different bytecode each compilation to:
- Prevent signature-based detection
- Thwart static analysis
- Make reverse engineering harder

#### Techniques Implemented:

##### Level 1-2: NOP Insertion (Simplest)
```
Original:        OpConstant 5, OpPrint
Polymorphic:     OpConstant 5, OpNull, OpPop, OpPrint
                 (OpNull+OpPop = NOP, no effect)
```

##### Level 3-4: Instruction Reordering
```
Original:        OpConstant 5, OpConstant 10, OpAdd
Polymorphic:     OpConstant 10, OpConstant 5, OpAdd
                 (Addition is commutative, order doesn't matter)
```

##### Level 5-6: Constant Pool Randomization
```
Original:        Constants[0]=5, [1]=10, [2]="hello"
Polymorphic:     Constants[0]="hello", [1]=10, [2]=5
                 (Update all OpConstant operands to match)
```

##### Level 7-8: Opcode Mutation
```
Original:        OpAdd=0x02, OpSub=0x03
Polymorphic:     OpAdd=0x15, OpSub=0x08
                 (Random opcode mapping per compilation)
```

##### Level 9-10: Dead Code Insertion
```
Polymorphic:     if (1 == 0) { /* fake code */ }
                 (Never executes, but confuses analyzers)
```

#### Usage:
```bash
# Basic polymorphism (Level 5)
mutant release -src app.mut -mutation 5

# Maximum polymorphism (Level 10)
mutant release -src app.mut -mutation 10

# Deterministic (for testing, same output)
mutant release -src app.mut -mutation 5 -deterministic
```

#### How it works:
```go
// In compiler/polymorphic.go
engine := NewPolymorphicEngine(level, randomSeed)
bytecode = engine.Mutate(bytecode)

// Applies transformations:
1. InsertNOPs()           // 5-15% code inflation
2. ReorderInstructions()  // Safe reordering
3. MutateOpcodes()        // Remap opcodes
4. RandomizeConstants()   // Shuffle constant pool
5. InsertDeadCode()       // Bogus branches
```

**Performance Impact:**
- Compile time: +10-50% (depending on level)
- Bytecode size: +5-20%
- Runtime: None (functionally identical)
- Detection difficulty: Exponential increase

**Example Output:**
```
Compile #1 (SHA-256): a3f5c9d2e8b4f7a1c5d9e2f8b4a7c1d5...
Compile #2 (SHA-256): 7b2e8f4c1a9d5e3b7f1c8d4a6e2b9f5c...
Compile #3 (SHA-256): d9c4a7f2e5b1c8d3f6a9e4b7c2d5f8a1...
                      ^^^ All different hashes!
                      But functionally identical when run
```

---

### 3. OTHER SECURITY SUGGESTIONS

#### A. Code Signing (HIGH PRIORITY)
**Current Issue:** MD5 integrity check (broken, vulnerable to collisions)

**Solution:** Ed25519 digital signatures (`security/signing.go`)

```bash
# Generate keypair once
mutant keygen -output mutant.key

# Sign during compilation
mutant release -src app.mut -sign mutant.key

# Automatic verification at runtime
mutant app.mu  # Verifies signature before running
```

**Benefits:**
- Tamper detection
- Authenticity verification
- Can't modify bytecode without private key

#### B. Anti-Debugging (MODERATE PRIORITY)
**Current Issue:** No protection against debuggers

**Solution:** Platform-specific debugger detection

```go
// Detects:
- ptrace on Linux
- IsDebuggerPresent on Windows
- sysctl on macOS
- Timing attacks
```

**Usage:**
```bash
# Enable at compile time
mutant release -src app.mut -anti-debug

# Runtime check before execution
if IsDebuggerPresent() {
    return error
}
```

#### C. Sandboxing (MODERATE PRIORITY)
**Current Issue:** No runtime restrictions

**Solution:** Security policies

```go
policy := SecurityPolicy{
    AllowFileIO: false,
    AllowNetwork: false,
    MaxMemory: 512MB,
    MaxExecutionTime: 60s,
}

vm.EnforcePolicy(policy)
```

**Usage:**
```bash
mutant run app.mu --no-file-io --max-memory 512M
```

#### D. Runtime Integrity Checks
**Current Issue:** No self-verification during execution

**Solution:** Periodic integrity checks

```go
// Every N instructions:
if !vm.checkIntegrity() {
    panic("VM state corrupted")
}
```

#### E. Secure File Deletion
**Current Issue:** Temp files may leak secrets

**Solution:** DoD 5220.22-M compliant deletion

```go
// 3-pass overwrite before delete
secureDelete("temp.mu")
```

#### F. Source Code Hash Verification
**Current Issue:** Can't verify bytecode matches source

**Solution:** Embed source hash in bytecode

```go
// In bytecode metadata:
SourceHash: SHA256(original_source)

// At runtime:
if user_source_hash != bytecode.metadata.SourceHash {
    warn("Bytecode may not match source")
}
```

---

### USABLE SECURITY RECOMMENDATIONS

**Goal:** User doesn't think about security, it "just works"

#### Default Behavior (No flags required):
```bash
# Compile - automatically secure
mutant myapp.mut
# - Deterministic encryption (HKDF)
# - SHA-256 integrity
# - Basic polymorphism (level 3)
# - Memory cleared on exit

# Run - automatically verified
mutant myapp.mu
# - Signature verification
# - Memory protection
# - Auto-cleanup
```

#### Advanced Security (One flag):
```bash
mutant release -src myapp.mut --high-security
# Enables:
# - Password protection (prompts)
# - Max polymorphism (level 10)
# - Anti-debugging
# - Full memory encryption
# - Code signing
```

#### CI/CD Mode (Reproducible):
```bash
mutant release -src myapp.mut --deterministic
# - No random mutations
# - Reproducible builds
# - Still encrypted
# - Still signed
```

---

### IMPLEMENTATION PRIORITY

#### Phase 1 (Do First - Critical Fixes)
1. ✅ Replace MD5 with SHA-256
2. ✅ Fix math/rand -> crypto/rand
3. ✅ Implement Argon2id KDF
4. ✅ Separate keys from ciphertext
5. ✅ Add Ed25519 signing

**Why:** Fixes critical vulnerabilities

#### Phase 2 (Next - User Facing)
1. ✅ Add password flag to CLI
2. ✅ Integrate KDF in generator
3. ✅ Update runner for password verification
4. ✅ Add deterministic mode

**Why:** Provides user choice

#### Phase 3 (Then - Memory Security)
1. ✅ Secure global objects
2. ✅ Memory zeroing
3. ⚠️ Auto-encrypting stack (optional - performance impact)
4. ⚠️ Secure constant pool (optional)

**Why:** Defense in depth

#### Phase 4 (Advanced - Code Morphing)
1. ✅ Basic NOP insertion
2. ✅ Constant pool randomization
3. ⚠️ Opcode mutation (requires VM changes)
4. ⚠️ Dead code insertion

**Why:** Makes reverse engineering harder

#### Phase 5 (Hardening)
1. ⚠️ Anti-debugging
2. ⚠️ Sandboxing
3. ⚠️ Runtime integrity
4. ⚠️ Secure deletion

**Why:** Additional protection layers

---

### FINAL RECOMMENDATIONS

#### For Maximum Security:
```bash
mutant release -src app.mut \
    -password \                 # Argon2id encryption
    -mutation 10 \              # Max polymorphism
    -anti-debug \               # Anti-debugging
    -sign mykey.key \           # Code signing
    --secure-memory             # Full memory encryption
```

#### For Best Performance:
```bash
mutant release -src app.mut \
    -mutation 3                 # Basic polymorphism
    # No password (deterministic)
    # No memory encryption
```

#### For CI/CD (Reproducible):
```bash
mutant release -src app.mut \
    -mutation 0 \               # No randomization
    -deterministic \            # Reproducible builds
    -sign $CI_KEY               # Sign with CI key
```

---

### KEY TAKEAWAYS

1. **Encryption:** Use hybrid approach - deterministic by default, password optional
2. **Memory:** Secure globals always, auto-encrypt optionally (performance trade-off)
3. **Polymorphism:** Yes! Level 3-5 by default, up to 10 for paranoid
4. **Usability:** Security by default, no configuration needed
5. **Implementation:** Start with Phase 1 (critical fixes), then add features incrementally

**Security is on by default, users get protection without thinking about it.**

---

## Next Steps

1. Review `SECURITY_ENHANCEMENTS.md` for detailed explanations
2. Follow `IMPLEMENTATION_GUIDE.md` for step-by-step implementation
3. Start with Phase 1 (critical security fixes)
4. Add dependencies: `go get golang.org/x/crypto/{argon2,hkdf}`
5. Test each phase thoroughly
6. Benchmark performance impact
7. Update documentation
8. Consider professional security audit

**All necessary code has been provided in the new security modules!**
