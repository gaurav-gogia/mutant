# Rust Anti-Tamper Integration

This document describes the Rust static library integration used by Mutant
security builtins.

## Go Side

- Go package: native/rustffi
- Build tags for Rust-backed provider: cgo and mutant_rust
- Default behavior without tags/toolchain: stub provider returns unavailable

## Rust Side

- Crate location: native/rustffi/lib
- Crate type: staticlib
- Exported symbols:
  - mutant_rust_probe
  - mutant_rust_free

## Probe Coverage (Current)

The Rust probe engine returns structured signals with:

- name
- detected
- confidence
- detail

Implemented probes currently include:

- hardware_breakpoint
- timing
- syscall
- frida_ptrace
- ld_preload
- cpuid_hypervisor
- rdtsc_drift
- acpi_pci
- gpu_feature (placeholder)
- iat_got (placeholder)
- syscall_table (placeholder)
- trampoline (placeholder)

### Windows-Specific Notes

- hardware_breakpoint:
  - Implemented for x86_64 and x86.
  - Uses Win32 thread context APIs and inspects DR0-DR3 and DR7 enable bits.
- syscall:
  - Uses IsDebuggerPresent and CheckRemoteDebuggerPresent API heuristics.
- frida_ptrace:
  - Checks FRIDA-related environment markers and tasklist process markers.
- ld_preload:
  - Maps to Windows injection-style environment markers (COR_ENABLE_PROFILING,
    COR_PROFILER, COR_PROFILER_PATH, __COMPAT_LAYER).
- acpi_pci:
  - Uses Windows system metadata (wmic/systeminfo) and virtualization markers.

### Linux-Specific Notes

- syscall/frida_ptrace inspect /proc/self/status TracerPid.
- ld_preload inspects LD_PRELOAD.
- acpi_pci inspects DMI/sysfs metadata.

### Other Platforms

- Unsupported probes return detected=false with descriptive detail strings.

## Expected Sample Output

Example probe response (trimmed):

```json
{
  "version": 1,
  "ok": true,
  "error": "",
  "signals": [
    {
      "name": "hardware_breakpoint",
      "detected": false,
      "confidence": 0,
      "detail": "dr7=0x0;enabled_mask=0x0;active_slots=0"
    },
    {
      "name": "syscall",
      "detected": true,
      "confidence": 80,
      "detail": "api_hits=IsDebuggerPresent"
    },
    {
      "name": "cpuid_hypervisor",
      "detected": true,
      "confidence": 70,
      "detail": "ecx=0xfeda3223"
    }
  ]
}
```

Clean baseline example (no suspicious signals):

```json
{
  "version": 1,
  "ok": true,
  "error": "",
  "signals": [
    {
      "name": "hardware_breakpoint",
      "detected": false,
      "confidence": 0,
      "detail": "dr7=0x0;enabled_mask=0x0;active_slots=0"
    },
    {
      "name": "timing",
      "detected": false,
      "confidence": 5,
      "detail": "loop_us=2450;acc=0"
    },
    {
      "name": "frida_ptrace",
      "detected": false,
      "confidence": 0,
      "detail": "no frida/ptrace heuristic triggered"
    }
  ]
}
```

Interpretation guidelines:

- Treat `detected` as per-signal evidence, not final policy action.
- Use `confidence` to rank severity and combine with other signals.
- Use `detail` for operator diagnostics and incident triage.
- Respect `ok=false` as probe-level failure and inspect `error`.

## Build Commands

### Host build

pwsh ./native/rustffi/build_rust.ps1

### Cross-target examples

pwsh ./native/rustffi/build_rust.ps1 -Target x86_64-pc-windows-msvc pwsh
./native/rustffi/build_rust.ps1 -Target x86_64-unknown-linux-gnu pwsh
./native/rustffi/build_rust.ps1 -Target aarch64-unknown-linux-gnu pwsh
./native/rustffi/build_rust.ps1 -Target x86_64-apple-darwin
./native/rustffi/build_rust.ps1 -Target aarch64-apple-darwin

## Enable Rust Probes at Runtime

Set:

- MUTANT_ENABLE_RUST_ANTITAMPER=1

When disabled or unavailable, builtins remain advisory and return
rust_enabled=false with empty rust_signals.

## Strict Release Preconditions

The release asset generator supports strict precheck env flags:

- MUTANT_REQUIRE_RUST_STATICLIB=1
- MUTANT_RUST_STATICLIB_PATH=<path to static library>
- MUTANT_RUST_RELEASE_REQUIRE_CGO=1

If enabled and prerequisites are missing, release asset generation fails fast.
