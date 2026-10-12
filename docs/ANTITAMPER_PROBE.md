# Anti-Tamper Probe Integration

This document explains how anti-tamper probes work today in Mutant, including
the exact enablement gates and how runner enforcement differs from builtin
diagnostic calls.

## 1. Architecture

Core files:

1. `security/antitamper_probe.go` (engine + enablement gate)
2. `security/antitamper_routing.go` (probe dispatch)
3. `security/antitamper_detectors.go` (cross-platform heuristics)
4. `security/antitamper_windows.go` (windows process-protection heuristics)
5. `runner/runner.go` (enforcement)
6. `builtin/security_status.go` (diagnostic exposure)

Telemetry events used by this subsystem:

1. `anti_tamper_probe_invoked`
2. `anti_tamper_probe_error`
3. `process_protection_detected`

## 2. Enablement Model (Important)

**Both gates are on, always. Neither is configurable.** They used to be
environment variables; Mutant takes no configuration from the environment, and
these are not per-run decisions. See
[CONFIGURATION_POLICY.md](CONFIGURATION_POLICY.md).

1. Master probe gate: `antiTamperProbeEnabled`
   (`security/antitamper_probe.go`), a package-level `true`.
   `SetAntiTamperProbeEnabledForTesting` exists only so tests can turn probing
   off.
2. Runner process-protection gate: `isProcessProtectionEnabled`
   (`runner/runner.go`), which returns `true`.

Behavior:

1. `RunAntiTamperProbe` returns `enabled=true` and runs the requested probes.
   It returns `enabled=false` only inside a test that has flipped gate #1.
2. Gate #2 governs runner enforcement only. `enforceProcessProtection` consults
   it, then runs the five process-protection probes.
3. What still varies per run is the *response*, not the probing: a signal at
   confidence `>= 80` (`processProtectionTerminateConfidence`) terminates in
   secure mode and warns under `--compat`/`--dev`. See
   [ANTITAMPER_PROBE_ENABLEMENT_LLD.md](ANTITAMPER_PROBE_ENABLEMENT_LLD.md).

## 3. Probe Output Shape

Each probe returns one `AntiTamperSignal` with:

1. `name`
2. `measured`
3. `detected`
4. `confidence`
5. `detail`

Interpretation:

1. `measured` says whether the probe looked. A probe that cannot run on the
   host, or whose own check failed, answers `measured=false`, `detected=false`,
   confidence `0`, and a `detail` saying why. Its `detected=false` is then not
   a finding: "not detected" and "not looked for" are different answers, and
   before 2.6.0 they looked the same.
2. `detected` is evidence for that probe only.
3. `confidence` is a per-probe confidence score, not a global verdict.
4. policy action is decided by caller logic (runner or builtin consumer).

## 4. Implemented Probes (Current)

1. `timing`
2. `frida_ptrace`
3. `sleep_drift`
4. `iat_got`
5. `syscall_table`
6. `trampoline`
7. `process_injection`
8. `module_integrity`
9. `memory_page_anomaly`

Six names were listed here until 2.6.0 and are no longer probes; asking for
one returns the `unknown probe` signal.

- `acpi_pci` re-reported the sandbox detector's verdict, which `sandbox_status`
  reports in its own fields, and read no ACPI table or PCI device.
- `cpuid_hypervisor` re-reported the same verdict and read no CPUID of its own,
  so on a host whose CPUID reports a hypervisor it could answer "no hypervisor
  signal". The CPUID reading the sandbox detector takes is one of
  `sandbox_status`'s indicators.
- `hardware_breakpoint` and `syscall` re-reported the debugger detector's
  verdict -- a parent process named `gdb` came out as a hardware breakpoint at
  confidence 70 and as a syscall finding at 80 -- and neither read a debug
  register or made any check of its own. The checks the debugger detector ran
  are `debug_status`'s indicators (section 5).
- `gpu_feature` was a placeholder that answered "not implemented yet".
- `ld_preload` answered "env-based preload checks disabled". It could not have
  seen anything: a release is built with `CGO_ENABLED=0` and statically linked,
  so the dynamic loader that `LD_PRELOAD` works through never runs.

One more was renamed: `sleep_drift` was called `rdtsc_drift`, which named an
RDTSC check it never made. It times short sleeps on the OS clock and reports,
in `sleep_ms` and `drift_ms`, how far their total ran from what they asked for.

## 5. Runner Enforcement vs Builtin Diagnostics

Runner enforcement probe set (hardcoded in `runner/runner.go`):

1. `process_injection`
2. `trampoline`
3. `iat_got`
4. `module_integrity`
5. `memory_page_anomaly`

Runner threshold:

1. any signal with `detected=true` and `confidence >= 80` triggers
   `process_protection_detected` policy flow.

All five read Windows process structures, so they run on Windows only. On Linux
and macOS every one answers `measured=false`: process protection looks at
nothing there, the stage passes, and the run says so once on stderr --
`[security] process protection measured nothing on linux: ...`.

Builtin diagnostics (`builtin/security_status.go`) use broader probe sets for
visibility and troubleshooting. This is expected and independent of runner
enforcement scope.

`debug_status` reports one reading of the debugger detector beside its probes:

1. `detected` is the platform detector's verdict, by its own rule. The Windows
   detector, for one, wants two heuristics before it reports a debugger on
   heuristics alone.
2. `indicators` are the checks that fired, as `security_diagnostics` names them
   (`windows:is_debugger_present`, `linux:parent_debugger_process`, ...).
3. `confidence` follows from which checks those are. A direct check reads the
   operating system's own record of a debugger: `windows:is_debugger_present`,
   `windows:check_remote_debugger_present`, `windows:process_debug_port`,
   `windows:process_debug_object_handle`,
   `windows:system_debug_control_query_version`, `linux:tracer_pid` and
   `darwin:p_traced`. When one fired, `confidence` is 100. Every other check --
   a debugger's name on the parent process, a debugger's environment variable,
   OutputDebugString timing, a debugger's DLL loaded -- is a heuristic, and
   heuristics alone give 35 for one and 70 for two or more. Nothing fired is 0.

Before 2.6.0 `confidence` was 100 for any detection and `indicators` was
always empty, so a parent process that happened to be named `gdb` read exactly
as a debugger the kernel says is attached. Because `detected` is the
detector's rule and `confidence` is this one, a reading can be detected below
100, or not detected above 0: one heuristic on Windows reads
`detected: false` with `confidence: 35`.

## 6. Platform Notes

Windows:

1. `process_injection` uses tasklist marker heuristics. It reads no
   environment variable: the five it read before 2.6.0 configure the .NET
   runtime, a Java VM and symbol handlers such as dbghelp's, none of which runs
   in Mutant's process (see
   [PROCESS_INJECTION_DETECTION_LLD.md](PROCESS_INJECTION_DETECTION_LLD.md),
   section 4.1).
2. `trampoline` checks selected API prologues for common hook patterns.
3. `iat_got` checks sensitive export memory ownership against expected module.
4. `module_integrity` checks suspicious loaded module markers.
5. `memory_page_anomaly` checks for RWX pages on sensitive API addresses.

Linux:

1. `frida_ptrace` checks FRIDA env markers and `/proc/self/status` tracer PID.

Linux, macOS and anything else:

1. `iat_got`, `syscall_table`, `trampoline`, `process_injection`,
   `module_integrity` and `memory_page_anomaly` answer `measured=false`, with
   `not measured on <os>: this probe reads Windows process structures`.

## 7. Student Notes

1. Probe enablement and enforcement are not the same thing.
2. Builtin probe output is diagnostic; runner probe output can become policy
   action.
3. Always read `detail` before acting on a single signal.
