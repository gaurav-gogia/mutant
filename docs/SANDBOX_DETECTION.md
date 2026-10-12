# Sandbox Detection Feature

## Overview

The sandbox detection module identifies containerized and virtualized runtimes
and feeds that signal into the runtime tamper-response pipeline.

Current implementation status:

- API implemented: IsSandboxed, DetectSandboxType, GetSandboxIndicators
- Platform detectors implemented: Windows, Linux, macOS
- Runtime enforcement integrated in bytecode execution path (runner.Run
  pre-execution)
- Telemetry integrated: sandbox_detected counter and audit event

## API

### IsSandboxed() bool

Returns true when detection confidence is greater than or equal to the built-in
threshold (currently 70).

```go
if security.IsSandboxed() {
    // High-confidence sandbox/container/vm signal
}
```

### DetectSandboxType() (string, int, error)

Returns the most likely environment type, confidence in [0, 100], and any error
the platform detector reported.

Behavior:

- Returns ("none", 0, nil) when every check ran and no signal is present
- Confidence is clamped to [0, 100]
- Type is the highest-scoring class for current platform heuristics
- An error comes beside the type and confidence, never in place of them. It
  names checks the detector could not make -- on Linux, a cgroup file it could
  not read, as /proc/1/cgroup is for an unprivileged process where /proc is
  mounted hidepid=2 -- and the type and confidence are what the other checks
  found. A check that was not made can only have hidden a signal, so the
  confidence is a floor: at or above the threshold it is a detection either
  way, and IsSandboxed answers true.

```go
sandboxType, confidence, err := security.DetectSandboxType()
if confidence >= 70 {
    log.Printf("detected %s (%d%%)", sandboxType, confidence)
}
if err != nil {
    log.Printf("detection incomplete: %v", err)
}
```

### GetSandboxIndicators() ([]string, error)

Returns normalized, deduplicated indicator strings used by the detector, with
the error DetectSandboxType returns beside them rather than in place of them.
The slice is a copy, so a caller cannot edit what later callers see.

```go
indicators, err := security.GetSandboxIndicators()
for _, indicator := range indicators {
    log.Println(indicator)
}
if err != nil {
    log.Printf("indicators incomplete: %v", err)
}
```

## Cost and caching

Detection is expensive. The Windows detector alone spawns `tasklist`, three
`wmic` queries, a `powershell` CIM query, and four `reg query` calls; together
they take about a second. macOS shells out to `ps`, Linux reads a dozen files
under /proc and /sys.

The result is therefore computed once per process and cached. That is safe
because the answer cannot change while the process runs -- a program does not
move between a hypervisor and bare metal mid-execution -- and it is necessary
because the VM calls the detector from its instruction stream (see below), so an
uncached probe charged roughly a second per injected check a program reached.

## Implemented Detection Signals

### Linux

Container and VM heuristics currently include:

- Marker files: /.dockerenv, /run/.containerenv, /proc/xen
- Environment markers: KUBERNETES_SERVICE_HOST, KUBERNETES_SERVICE_PORT
- Cgroup markers from /proc/1/cgroup and /proc/self/cgroup: docker, containerd,
  podman, libpod, crio, kubepods, lxc, systemd patterns
- CPU markers from /proc/cpuinfo: hypervisor, kvm, qemu, vmware, virtualbox, xen
- DMI markers from /sys/class/dmi/id/*: vmware, virtualbox, kvm/qemu, xen,
  hyper-v fingerprints

Potential types include: Docker, Container, Kubernetes, LXC, systemd-nspawn, VM,
KVM/QEMU, VMware, VirtualBox, Xen, Hyper-V

### Windows

VM/sandbox heuristics currently include:

- Driver/library file markers: vmmouse.sys, vmhgfs.sys, VBoxMouse.sys,
  VBoxGuest.sys, xenbus.sys, SbieDll.dll
- Environment markers: SANDBOXIE, CUCKOO. VBOX_INSTALL_PATH and
  VBOX_MSI_INSTALL_PATH are not read: VirtualBox's installers set them on the
  machine they install VirtualBox on, which is a host that runs guests rather
  than a guest, and a VirtualBox guest is recognised by the drivers and
  processes of its Guest Additions.
- Process markers from tasklist: vmtoolsd.exe, vmwaretray.exe, vboxservice.exe,
  vboxtray.exe, xenservice.exe, qemu-ga.exe, sbiectrl.exe,
  sandboxiedcomlaunch.exe
- Parent-process and working-directory markers for WSL, a bash.exe parent for an
  MSYS/Cygwin shell or the legacy WSL launcher, and the WDAG account and profile
  path for Windows Sandbox
- CPUID hypervisor vendor leaf
- Baseboard and computer-system manufacturer/model, via wmic with a powershell
  Get-CimInstance fallback
- PnP device names, raw SMBIOS firmware table, and registry markers under
  HKLM\SOFTWARE\Microsoft\VirtualMachine and HKLM\HARDWARE\DESCRIPTION\System\BIOS

Potential types include: VMware, VirtualBox, Xen, KVM/QEMU, Sandboxie, Cuckoo,
Hyper-V, WSL, Windows Sandbox, MSYS/Cygwin or legacy WSL

"MSYS/Cygwin or legacy WSL" is the type a bash.exe parent process gets, and it
names both because that name cannot be narrowed: the parent is read from
tasklist, whose image-name column is a basename and never a path, so a Git for
Windows, MSYS2 or Cygwin shell and the legacy WSL launcher are indistinguishable
there. A run that is really under WSL scores WSL from its context environment
and its UNC working directory as well, and is reported as WSL.

### macOS

Virtualization/sandbox heuristics currently include:

- Environment markers: APP_SANDBOX_CONTAINER_ID, DYLD_INSERT_LIBRARIES
- Guest-tool process markers from ps -axo comm: vboxservice, vboxclient
  (VirtualBox Guest Additions), prl_tools (Parallels Tools)
- CPUID hypervisor vendor leaf

Potential types include: macOS App Sandbox, VirtualBox, Parallels, VM

Hosting virtual machines does not make a Mac a guest, so nothing a Mac carries
for hosting them is scored: VMware Fusion, VirtualBox, Parallels Desktop or UTM
installed under /Applications, /Users/Shared/Parallels, a Homebrew colima,
COLIMA_HOME, or vmware-vmx, qemu-system or colima running. Each of those is
found on the machine that runs guests and says nothing about whether this one
is a guest.

## Runtime Enforcement Integration

Sandbox detection is enforced in runner.Run at pre-execution stage, after decode
and after anti-debug pre-execution check.

Current flow:

1. Signature verification
2. Anti-debug pre-decode
3. Decode/decrypt
4. Anti-debug pre-execution
5. Anti-sandbox pre-execution
6. VM run

Detection is also enforced *inside* the VM: the compiler injects OpChkSnd
instructions, and executing one calls `security.IsSandboxed()`. That is a second
enforcement point, reached however many times the program's control flow reaches
an injected check -- which is why the detector is cached (see "Cost and
caching").

On sandbox detection:

- Event recorded: sandbox_detected
- Base error: ErrSandboxDetected
- Response policy resolved by existing tamper policy mechanism

Default response behavior:

- Secure mode: terminate
- Compat mode (`--compat`): warn
- Dev mode (`--dev`): warn

The response is derived from the execution mode by `ResolveTamperResponse`
(`security/response_policy.go`). It is not overridable: see
[CONFIGURATION_POLICY.md](CONFIGURATION_POLICY.md).

A terminating hit prints the detector, its type and confidence, the signals that
fired, and `--compat` as the remedy, before the run stops. The sandbox detail
comes from the cached detection the gate already performed, so explaining a
termination costs nothing. See
[EXECUTION_MODES.md](EXECUTION_MODES.md).

## Telemetry

Sandbox detection contributes to security telemetry snapshot/export:

- sandbox_detected

The counter is readable in-process through `SecurityTelemetrySnapshot()` and
`SecurityTelemetryJSON()`, and writable to a file by a caller that passes a path
to `ExportSecurityTelemetry(path)`. There is no audit stream and no automatic
export: `auditEvent` is a no-op today, and nothing in the runner calls
`ExportSecurityTelemetry`.

## Testing

Relevant tests:

- security package sandbox API contract and telemetry coverage
- runner package anti-sandbox policy behavior (secure terminate, compat warn)

Run:

```bash
go test ./security ./runner
```

## Known Limitations

1. Heuristic coverage is intentionally lightweight and may miss sophisticated
   environments.
2. False positives are possible in legitimate virtualized/containerized
   deployments.
3. Some signals depend on process/file visibility and OS permissions.
4. Confidence scores are heuristic, not cryptographic guarantees.

## Future Enhancements

1. Cloud metadata-based environment detection.
2. Additional sandbox frameworks and emulator fingerprints.
3. Policy controls specific to sandbox events (separate from debugger/integrity
   policies).
4. Configurable confidence threshold for IsSandboxed.
