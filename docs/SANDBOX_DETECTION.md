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

### DetectSandboxType() (string, int)

Returns the most likely environment type and confidence in [0, 100].

Behavior:

- Returns ("none", 0) when no signal is present
- Confidence is clamped to [0, 100]
- Type is the highest-scoring class for current platform heuristics

```go
sandboxType, confidence := security.DetectSandboxType()
if confidence >= 70 {
    log.Printf("detected %s (%d%%)", sandboxType, confidence)
}
```

### GetSandboxIndicators() []string

Returns normalized, deduplicated indicator strings used by the detector.

```go
for _, indicator := range security.GetSandboxIndicators() {
    log.Println(indicator)
}
```

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
- Environment markers: SANDBOXIE, CUCKOO, VBOX_INSTALL_PATH
- Process markers from tasklist: vmtoolsd.exe, vmwaretray.exe, vboxservice.exe,
  vboxtray.exe, xenservice.exe, qemu-ga.exe, sbiectrl.exe,
  sandboxiedcomlaunch.exe

Potential types include: VMware, VirtualBox, Xen, KVM/QEMU, Sandboxie, Cuckoo

### macOS

Virtualization/sandbox heuristics currently include:

- Environment markers: APP_SANDBOX_CONTAINER_ID, DYLD_INSERT_LIBRARIES,
  COLIMA_HOME
- Application/file markers: /Applications/VMware Fusion.app,
  /Applications/VirtualBox.app, /Applications/Parallels Desktop.app,
  /Applications/UTM.app, /Users/Shared/Parallels, /opt/homebrew/bin/colima
- Process markers from ps -axo comm: vmware-vmx, vboxservice, vboxclient,
  prl_tools, qemu-system, colima

Potential types include: macOS App Sandbox, Colima, VMware Fusion, VirtualBox,
Parallels, UTM, QEMU

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

On sandbox detection:

- Event recorded: sandbox_detected
- Base error: ErrSandboxDetected
- Response policy resolved by existing tamper policy mechanism

Default response behavior:

- Secure mode: terminate
- Compat mode: warn
- Override via MUTANT_TAMPER_RESPONSE (warn|delay|terminate)

## Telemetry

Sandbox detection contributes to security telemetry snapshot/export:

- sandbox_detected

Audit stream integration (when MUTANT_SECURITY_AUDIT=1):

- event=sandbox_detected stage=<stage>

Telemetry export file is controlled by:

- MUTANT_SECURITY_TELEMETRY_FILE

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
