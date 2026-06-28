# Anti-Tamper Probe Integration

This document describes the current anti-tamper probe integration used by Mutant
security builtins.

## Architecture

- Probe engine location: `security/antitamper_probe.go`
- Runtime consumers:
  - `builtin/security_status.go`
  - `security` telemetry counters (`anti_tamper_probe_invoked`,
    `anti_tamper_probe_error`)
- Runtime gate env var: `MUTANT_ENABLE_ANTITAMPER_PROBE=1`

## Probe Coverage (Current)

The probe engine returns structured signals with:

- `name`
- `detected`
- `confidence`
- `detail`

Implemented probes currently include:

- `hardware_breakpoint`
- `timing`
- `syscall`
- `frida_ptrace`
- `ld_preload`
- `cpuid_hypervisor`
- `rdtsc_drift`
- `acpi_pci`
- `gpu_feature` (placeholder)
- `iat_got` (placeholder)
- `syscall_table` (placeholder)
- `trampoline` (placeholder)

## Platform Notes

### Windows

- `syscall` and `hardware_breakpoint` signals are derived from existing
  debugger-detection methods in `security/antidebug_windows.go`.
- `frida_ptrace` includes FRIDA env marker checks and `tasklist` marker checks.
- `ld_preload` maps to Windows injection-style environment markers:
  - `COR_ENABLE_PROFILING`
  - `COR_PROFILER`
  - `COR_PROFILER_PATH`
  - `__COMPAT_LAYER`

### Linux

- `frida_ptrace` checks FRIDA env markers and `/proc/self/status` tracer PID.
- `ld_preload` reads `LD_PRELOAD`.

### macOS / Other

- Unsupported heuristics return `detected=false` with detail text.

## Expected Output Shape

Example response fields in builtin output:

```json
{
  "probe_enabled": true,
  "probe_error": "",
  "probe_signals": [
    {
      "name": "syscall",
      "detected": false,
      "confidence": 0,
      "detail": "no debugger API signal detected"
    }
  ]
}
```

Interpretation guidelines:

- Treat `detected` as per-signal evidence, not final policy action.
- Use `confidence` to rank severity and combine with other signals.
- Use `detail` for operator diagnostics and triage.

## Build and Release

No external toolchain or cgo linkage is required for anti-tamper probe
execution.

Release asset generation continues to build Go-only runtime binaries with
`CGO_ENABLED=0`.
