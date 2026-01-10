# Security Architecture Diagrams

## Overall Security Flow

```
┌─────────────────────────────────────────────────────────────────┐
│                      MUTANT SECURITY LAYERS                      │
└─────────────────────────────────────────────────────────────────┘

 SOURCE CODE (myapp.mut)
      │
      ▼
┌─────────────────────────┐
│   COMPILATION PHASE     │
├─────────────────────────┤
│ 1. Parse & Compile      │◄─── User Input
│ 2. Generate Bytecode    │     (Optional: password, mutation level)
│ 3. Apply Security       │
└─────────────────────────┘
      │
      ├──► Encryption Layer
      │    ├─ Password? → Argon2id KDF
      │    └─ No pwd? → HKDF (deterministic)
      │
      ├──► Polymorphic Engine
      │    ├─ NOP insertion (5-15%)
      │    ├─ Instruction reordering
      │    ├─ Constant shuffling
      │    └─ Opcode mutation
      │
      ├──► Code Signing
      │    └─ Ed25519 signature
      │
      ▼
┌─────────────────────────┐
│  ENCRYPTED BYTECODE     │
│  + Metadata             │
│  + Signature            │
└─────────────────────────┘
      │
      │ (myapp.mu file written)
      │
      ▼
┌─────────────────────────┐
│   EXECUTION PHASE       │
├─────────────────────────┤
│ 1. Verify Signature     │
│ 2. Decrypt Bytecode     │◄─── Password if required
│ 3. Initialize VM        │
│ 4. Run with Protection  │
└─────────────────────────┘
      │
      ├──► Memory Security
      │    ├─ Secure globals (encrypted)
      │    ├─ Auto-encrypt stack
      │    └─ Secure constant pool
      │
      ├──► Anti-Debugging
      │    └─ Debugger detection
      │
      ├──► Runtime Checks
      │    ├─ Integrity verification
      │    └─ Sandbox policies
      │
      ▼
 OUTPUT / RESULT
```

## Key Derivation Flow

### Option A: Password-Based (High Security)

```
┌──────────────┐
│ USER INPUT   │
│  Password:   │
│  ********    │
└──────────────┘
       │
       ▼
┌──────────────────────────────────────┐
│  Source Code Hash (SHA-256)          │
│  Used as salt for determinism        │
└──────────────────────────────────────┘
       │
       ▼
┌──────────────────────────────────────┐
│  Argon2id Key Derivation Function    │
│  ├─ Time cost: 1 iteration           │
│  ├─ Memory: 64 MB                    │
│  ├─ Parallelism: 4 threads           │
│  └─ Output: 32 bytes (256 bits)      │
└──────────────────────────────────────┘
       │
       ▼
┌──────────────────────────────────────┐
│  Encryption Key (32 bytes)           │
│  ⚠️ NEVER STORED                     │
└──────────────────────────────────────┘
       │
       ▼
┌──────────────────────────────────────┐
│  AES-256-GCM Encryption              │
│  ├─ Key: derived key                 │
│  ├─ Nonce: random (12 bytes)         │
│  └─ AAD: "MUTANT" signature          │
└──────────────────────────────────────┘
       │
       ▼
┌──────────────────────────────────────┐
│  STORED IN FILE:                     │
│  ├─ Encrypted bytecode               │
│  ├─ Nonce (safe to store)            │
│  ├─ KDF params (safe to store)       │
│  │   ├─ Algorithm: "argon2id"        │
│  │   ├─ Salt: source hash            │
│  │   ├─ Time: 1                      │
│  │   ├─ Memory: 65536 KB             │
│  │   └─ Threads: 4                   │
│  └─ ❌ KEY IS NOT STORED!            │
└──────────────────────────────────────┘
```

### Option B: Deterministic (Convenience)

```
┌──────────────────────────────────────┐
│  Source Code (myapp.mut)             │
└──────────────────────────────────────┘
       │
       ▼
┌──────────────────────────────────────┐
│  SHA-256 Hash of Source              │
│  e.g., a3f5c9d2e8b4f7a1...           │
└──────────────────────────────────────┘
       │
       ├─── Used as input material
       │
       ▼
┌──────────────────────────────────────┐
│  Metadata String                     │
│  filename|version|timestamp          │
│  "myapp.mut|2.1.0|2025-10-20"        │
└──────────────────────────────────────┘
       │
       ▼
┌──────────────────────────────────────┐
│  HKDF (HMAC-based KDF)               │
│  ├─ Hash: SHA-256                    │
│  ├─ Input: source hash               │
│  ├─ Salt: source hash (deterministic)│
│  ├─ Info: "mutant-v1|" + metadata    │
│  └─ Output: 32 bytes                 │
└──────────────────────────────────────┘
       │
       ▼
┌──────────────────────────────────────┐
│  Encryption Key (32 bytes)           │
│  ⚠️ NEVER STORED                     │
│  Can be re-derived from source       │
└──────────────────────────────────────┘
       │
       ▼
┌──────────────────────────────────────┐
│  AES-256-GCM Encryption              │
│  (Same as password mode)             │
└──────────────────────────────────────┘
       │
       ▼
┌──────────────────────────────────────┐
│  STORED IN FILE:                     │
│  ├─ Encrypted bytecode               │
│  ├─ Nonce                            │
│  ├─ KDF params                       │
│  │   ├─ Algorithm: "hkdf-sha256"     │
│  │   ├─ Salt: source hash            │
│  │   └─ Info: metadata string        │
│  └─ ❌ KEY IS NOT STORED!            │
└──────────────────────────────────────┘
```

## Memory Security Architecture

```
┌──────────────────────────────────────────────────────┐
│                   PROGRAM MEMORY                      │
├──────────────────────────────────────────────────────┤
│                                                       │
│  ┌────────────────────────────────────────────┐     │
│  │  GLOBAL OBJECTS (Secure)                   │     │
│  ├────────────────────────────────────────────┤     │
│  │  True:  [encrypted: 0x4F, 0x2A, 0x91...]   │     │
│  │  False: [encrypted: 0x1C, 0x7B, 0x44...]   │     │
│  │  Null:  [encrypted: 0x9E, 0x53, 0xA2...]   │     │
│  │                                             │     │
│  │  Get() → Decrypt on demand                 │     │
│  │  Clear() → Secure zero                     │     │
│  └────────────────────────────────────────────┘     │
│                                                       │
│  ┌────────────────────────────────────────────┐     │
│  │  VM STACK (Auto-Encrypt)                   │     │
│  ├────────────────────────────────────────────┤     │
│  │  [0]: String "hello" ─┐                    │     │
│  │  [1]: Integer 42     ←─┼─ Recently used   │     │
│  │  [2]: [encrypted]    ←─┼─ Idle > 100ms    │     │
│  │  [3]: [encrypted]      │   Auto-encrypted │     │
│  │  [4]: Boolean true   ←─┘                   │     │
│  │                                             │     │
│  │  AutoProtect() runs every 100 instructions │     │
│  └────────────────────────────────────────────┘     │
│                                                       │
│  ┌────────────────────────────────────────────┐     │
│  │  CONSTANT POOL (LRU Cache)                 │     │
│  ├────────────────────────────────────────────┤     │
│  │  Encrypted: [0xA3, 0x4F, 0x2B, ...]        │     │
│  │                    ↓                        │     │
│  │  Cache (LRU):                              │     │
│  │  ┌─────────────────────────────┐           │     │
│  │  │ [5]: "world" (accessed 2ms ago)         │     │
│  │  │ [2]: 100 (accessed 5ms ago) │           │     │
│  │  │ [7]: "test" (accessed 8ms ago)          │     │
│  │  └─────────────────────────────┘           │     │
│  │  Cache full → Evict oldest                │     │
│  └────────────────────────────────────────────┘     │
│                                                       │
└──────────────────────────────────────────────────────┘

Access Pattern:
1. Request constant[5]
2. Check cache → HIT → Return decrypted
3. Request constant[99]
4. Check cache → MISS → Decrypt from storage
5. Cache full → Evict [7] (oldest) → Add [99]
```

## Polymorphic Bytecode Generation

```
┌──────────────────────────────────────────────────────┐
│         ORIGINAL BYTECODE (Same every time)           │
├──────────────────────────────────────────────────────┤
│  OpConstant 0    // Load 5                           │
│  OpConstant 1    // Load 10                          │
│  OpAdd           // Add them                         │
│  OpPrint         // Print result                     │
├──────────────────────────────────────────────────────┤
│  Constants: [5, 10]                                  │
└──────────────────────────────────────────────────────┘
                      │
                      ▼
┌──────────────────────────────────────────────────────┐
│         POLYMORPHIC ENGINE (Mutation Level 5)         │
└──────────────────────────────────────────────────────┘
                      │
        ┌─────────────┼─────────────┐
        │             │             │
        ▼             ▼             ▼
  ┌─────────┐   ┌─────────┐   ┌──────────┐
  │ NOPs    │   │ Shuffle │   │ Reorder  │
  │ Insert  │   │Constants│   │ Instruct │
  └─────────┘   └─────────┘   └──────────┘
        │             │             │
        └─────────────┼─────────────┘
                      ▼
┌──────────────────────────────────────────────────────┐
│         COMPILE #1 (Different each time!)             │
├──────────────────────────────────────────────────────┤
│  OpNull          // NOP inserted                     │
│  OpPop           //                                  │
│  OpConstant 1    // Load from shuffled constant 1    │
│  OpConstant 0    // Reordered!                       │
│  OpAdd           //                                  │
│  OpTrue          // NOP inserted                     │
│  OpPop           //                                  │
│  OpPrint         //                                  │
├──────────────────────────────────────────────────────┤
│  Constants: [10, 5]  // Shuffled order!              │
│  SHA-256: a3f5c9d2e8b4f7a1c5d9e2f8b4a7c1d5...       │
└──────────────────────────────────────────────────────┘

┌──────────────────────────────────────────────────────┐
│         COMPILE #2 (Different again!)                 │
├──────────────────────────────────────────────────────┤
│  OpConstant 0    // Different shuffle                │
│  OpTrue          // Different NOPs                   │
│  OpPop           //                                  │
│  OpConstant 1    //                                  │
│  OpAdd           //                                  │
│  OpPrint         //                                  │
├──────────────────────────────────────────────────────┤
│  Constants: [5, 10]  // Different shuffle!           │
│  SHA-256: 7b2e8f4c1a9d5e3b7f1c8d4a6e2b9f5c...       │
└──────────────────────────────────────────────────────┘

┌──────────────────────────────────────────────────────┐
│         COMPILE #3 (Different yet again!)             │
├──────────────────────────────────────────────────────┤
│  OpConstant 1    //                                  │
│  OpConstant 0    //                                  │
│  OpAdd           //                                  │
│  OpNull          // Different NOP placement          │
│  OpPop           //                                  │
│  OpNull          //                                  │
│  OpPop           //                                  │
│  OpPrint         //                                  │
├──────────────────────────────────────────────────────┤
│  Constants: [10, 5]  // Another shuffle!             │
│  SHA-256: d9c4a7f2e5b1c8d3f6a9e4b7c2d5f8a1...       │
└──────────────────────────────────────────────────────┘

        ALL THREE PRODUCE SAME RESULT: 15
        But signatures are completely different!
```

## Complete Security Stack

```
┌─────────────────────────────────────────────────────────┐
│                    APPLICATION LAYER                     │
│                    (User's Mutant Code)                  │
└─────────────────────────────────────────────────────────┘
                           ▲
                           │
┌──────────────────────────┼──────────────────────────────┐
│                          │                               │
│  ┌────────────────┐  ┌──┴───────────────┐              │
│  │ Anti-Debugging │  │  VM Security     │              │
│  │ ├─ Debugger    │  │  ├─ Stack guard  │              │
│  │ │  detection   │  │  ├─ Integrity    │              │
│  │ └─ Timing      │  │  │  checks       │              │
│  │    checks      │  │  └─ Sandbox      │              │
│  └────────────────┘  └──────────────────┘              │
│                                                          │
│  ┌─────────────────────────────────────────┐           │
│  │       Memory Security Layer             │           │
│  │  ├─ Secure globals (encrypted)          │           │
│  │  ├─ Auto-encrypt stack                  │           │
│  │  ├─ Secure constant pool (LRU)          │           │
│  │  └─ Memory zeroing on cleanup           │           │
│  └─────────────────────────────────────────┘           │
│                                                          │
│  ┌─────────────────────────────────────────┐           │
│  │       Bytecode Execution Layer          │           │
│  │  ├─ Decrypt instructions on-the-fly     │           │
│  │  ├─ XOR with secure keys                │           │
│  │  └─ Clear after execution               │           │
│  └─────────────────────────────────────────┘           │
│                                                          │
└──────────────────────────────────────────────────────────┘
                           ▲
                           │
┌──────────────────────────┼──────────────────────────────┐
│  ┌────────────────┐  ┌──┴───────────────┐              │
│  │ Code Signing   │  │  Encryption      │              │
│  │ ├─ Ed25519     │  │  ├─ AES-256-GCM  │              │
│  │ │  signature   │  │  ├─ Argon2id KDF │              │
│  │ └─ Verify      │  │  │  or HKDF      │              │
│  │    integrity   │  │  └─ Secure keys  │              │
│  └────────────────┘  └──────────────────┘              │
│                                                          │
│  ┌─────────────────────────────────────────┐           │
│  │    Polymorphic Bytecode Layer           │           │
│  │  ├─ NOP insertion (5-15%)               │           │
│  │  ├─ Instruction reordering              │           │
│  │  ├─ Constant pool randomization         │           │
│  │  ├─ Opcode mutation                     │           │
│  │  └─ Dead code insertion                 │           │
│  └─────────────────────────────────────────┘           │
│                                                          │
└──────────────────────────────────────────────────────────┘
                           ▲
                           │
┌──────────────────────────┼──────────────────────────────┐
│              COMPILED BYTECODE FILE (.mu)                │
│  ├─ Header: "MUT"                                       │
│  ├─ Signature (Ed25519)                                 │
│  ├─ Encryption metadata (no keys!)                      │
│  ├─ Polymorphic marker (mutation level)                 │
│  ├─ Encrypted bytecode                                  │
│  ├─ Integrity hash (SHA-256)                            │
│  └─ Footer: "ANT"                                       │
└──────────────────────────────────────────────────────────┘
```

## Threat Model Coverage

```
┌─────────────────────────────────────────────────────────┐
│                      THREATS                             │
├─────────────────────────────────────────────────────────┤
│                                                          │
│  Static Analysis Attack                                 │
│  ├─ Threat: Reverse engineer bytecode                   │
│  └─ Defense: Polymorphic code (different each compile)  │
│                                                          │
│  Debugging Attack                                        │
│  ├─ Threat: Attach debugger, inspect runtime            │
│  └─ Defense: Anti-debugging checks                      │
│                                                          │
│  Memory Dump Attack                                      │
│  ├─ Threat: Dump process memory, extract secrets        │
│  └─ Defense: Auto-encrypt idle objects, secure zero     │
│                                                          │
│  Tampering Attack                                        │
│  ├─ Threat: Modify bytecode, inject malicious code      │
│  └─ Defense: Ed25519 signatures, integrity checks       │
│                                                          │
│  Brute Force Attack                                      │
│  ├─ Threat: Try to decrypt without password             │
│  └─ Defense: Argon2id (slow, memory-hard)               │
│                                                          │
│  Timing Attack                                           │
│  ├─ Threat: Use timing to extract cryptographic keys    │
│  └─ Defense: Constant-time comparisons                  │
│                                                          │
│  Replay Attack                                           │
│  ├─ Threat: Reuse old signed bytecode                   │
│  └─ Defense: Timestamps in signatures                   │
│                                                          │
│  Side Channel Attack                                     │
│  ├─ Threat: Extract info via power/EM/cache             │
│  └─ Defense: Memory encryption, auto-clear              │
│                                                          │
└─────────────────────────────────────────────────────────┘
```

---

*Note: These diagrams illustrate the multi-layered security architecture of Mutant. Each layer provides independent protection, creating defense-in-depth.*
