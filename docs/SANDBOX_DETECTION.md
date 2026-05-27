# Sandbox Detection Feature

## Overview

The sandbox detection module provides comprehensive detection of virtualized environments, containerized systems, and various sandbox implementations across Windows, Linux, and macOS platforms. This feature complements the existing anti-debugging and memory encryption security measures.

## Why Sandbox Detection?

Sandbox detection is crucial for security-conscious applications because:

1. **Analysis Prevention**: Detects when the application runs in analysis/testing environments where security measures might be bypassed
2. **Compliance**: Helps enforce licensing and DRM policies by preventing execution in virtualized/containerized environments
3. **Threat Intelligence**: Identifies when code is running under observation during incident response
4. **Security Research**: Prevents malware analysis and reverse engineering in controlled environments
5. **Behavioral Defense**: Combined with other security features, creates a comprehensive defense strategy

## API Functions

### 1. `IsSandboxed() bool`

Returns true if the process is running in any detected sandbox or virtualized environment.

```go
if security.IsSandboxed() {
    log.Println("Running in sandboxed environment")
    // Take appropriate action
}
```

### 2. `DetectSandboxType() (string, int)`

Returns the specific type of sandbox/VM detected and a confidence level (0-100).

```go
sandboxType, confidence := security.DetectSandboxType()
if confidence > 70 {
    log.Printf("Detected: %s (confidence: %d%%)\n", sandboxType, confidence)
}
```

**Possible values:**
- Windows: VMware, VirtualBox, Hyper-V, KVM/QEMU, Windows Defender Sandbox, Parallels, Xen, Cuckoo, Sandboxie, WINE
- Linux: Docker, Kubernetes, LXC, KVM/QEMU, Cuckoo, systemd-nspawn, VMware, VirtualBox, Xen
- macOS: macOS App Sandbox, Parallels, VMware Fusion, VirtualBox, UTM, Colima, QEMU

### 3. `GetSandboxIndicators() []string`

Returns a detailed list of all detected sandbox indicators for logging and analysis.

```go
indicators := security.GetSandboxIndicators()
for _, indicator := range indicators {
    log.Println("Sandbox indicator:", indicator)
}
```

## Detection Techniques by Platform

### Windows

1. **Virtual Machine Detection**
   - VMware: Registry keys, processes, DLLs, MAC address prefix (00:0c:29), CPU brand
   - VirtualBox: Registry keys, processes, MAC address (08:00:27), DLLs
   - Hyper-V: Registry keys, CPU features (CPUID), processes
   - Parallels: Registry keys, processes, MAC address (00:1c:42)
   - Xen: Registry keys, CPU brand, xenbus drivers
   - KVM/QEMU: CPU brand indicators

2. **Sandbox Detection**
   - Windows Defender Sandbox: Device Guard indicators, HVCI, MPAS environment
   - Sandboxie: Registry, DLL injection detection, environment variables
   - Cuckoo: Cuckoo DLL detection, environment variables, specific file paths
   - WINE: Registry, WINE-specific DLLs, environment variables

3. **System Metrics Analysis**
   - Processor count validation
   - RAM size analysis
   - Disk space verification

### Linux

1. **Container Detection**
   - Docker: `/.dockerenv`, cgroup markers, hostname patterns, Docker environment vars
   - Kubernetes: Service host/port variables, certificate detection
   - LXC: Process markers, cgroup identification, AppArmor profiles
   - systemd-nspawn: Environment variable detection, cgroup analysis

2. **Virtual Machine Detection**
   - KVM/QEMU: CPUID hypervisor detection, DMI product name, `/proc/cpuinfo` analysis
   - VMware: DMI identification, `/proc/scsi/scsi` analysis
   - VirtualBox: DMI detection, Guest Additions, kernel modules
   - Xen: `/proc/xen` detection, DMI markers, xenbus modules

3. **Sandbox Detection**
   - Cuckoo: Process detection, `/cuckoo/` path, analysis markers
   - Behavioral sandboxes: Cgroup-based detection

### macOS

1. **Virtualization Detection**
   - Parallels: Application bundles, LaunchDaemon detection, process identification
   - VMware Fusion: VMware application files, sysctl properties, process detection
   - VirtualBox: VirtualBox app, Guest Additions, kernel extensions
   - UTM: UTM application, QEMU processes
   - QEMU: QEMU executables, CPUID detection

2. **Sandbox Detection**
   - macOS App Sandbox: Environment variables, Home directory container markers, entitlements
   - Colima: Docker socket detection, Colima VM directories

## Integration with Other Security Features

The sandbox detection feature integrates seamlessly with existing security measures:

```go
package main

import (
    "log"
    "mutant/security"
)

func main() {
    // Combined security checks
    if security.IsDebuggerPresent() {
        log.Fatal("Debugger detected - exiting")
    }

    if security.IsSandboxed() {
        log.Fatal("Running in sandbox - exiting")
    }

    sandboxType, confidence := security.DetectSandboxType()
    if confidence > 80 {
        log.Printf("High-confidence sandbox detected: %s\n", sandboxType)
    }

    // Continue with application logic
}
```

## Configuration and Customization

### Adjusting Detection Sensitivity

Different applications may need different sensitivity levels:

```go
// High security: Detect even low-confidence indicators
indicators := security.GetSandboxIndicators()
if len(indicators) > 0 {
    log.Fatal("Any sandbox indicators detected")
}

// Medium security: Require type detection
if sandboxType, conf := security.DetectSandboxType(); conf > 70 {
    log.Fatal("Likely sandbox environment")
}

// Low security: Only block on definite detection
if security.IsSandboxed() {
    log.Fatal("Sandbox definitely detected")
}
```

## Performance Considerations

- **Startup Impact**: Minimal - checks are fast and parallelizable
- **Memory Usage**: Negligible - primarily reads system information
- **Platform Optimization**: Platform-specific implementations for efficiency

Benchmark results show:
- `IsSandboxed()`: ~1-5ms per call
- `DetectSandboxType()`: ~1-5ms per call
- `GetSandboxIndicators()`: ~2-10ms per call

## Security Best Practices

1. **Combine with Anti-Debugging**: Use both `IsDebuggerPresent()` and `IsSandboxed()` for comprehensive protection
2. **Don't Advertise**: Avoid logging which detection method failed
3. **Gradual Response**: Consider having different response levels (warn, restrict, terminate)
4. **Regular Updates**: VM and sandbox detection signatures evolve - keep the library updated
5. **Timing Analysis**: Be aware that multiple detection calls might be noticeable

## Future Enhancements

Potential additions to the sandbox detection module:

1. **Cloud Platform Detection** (AWS, Azure, GCP metadata services)
2. **Emulator Detection** (Android emulators, game console emulators)
3. **Network-based Detection** (analyzing network stack for VM indicators)
4. **Memory Signature Analysis** (detecting hypervisor memory patterns)
5. **Custom Sandbox Registry** (allowing users to define custom detection rules)

## Testing

The sandbox detection module includes comprehensive tests:

```bash
go test ./security -v -run TestSandbox
go test ./security -bench BenchmarkSandbox -benchmem
```

## Platform Support

- ✅ **Windows** (Vista and later)
- ✅ **Linux** (all distributions)
- ✅ **macOS** (10.10+)
- ⚠️ **Other platforms** (stub implementations available)

## Known Limitations

1. Some sophisticated VMs may not be detected
2. Container detection is environment-specific
3. Sandboxes with full OS simulation are harder to detect
4. False positives possible in legitimate virtualized environments
5. Requires appropriate file system and process permissions

## Related Functions

- `security.IsDebuggerPresent()` - Debugger detection
- `security.SecureMemory` - Memory encryption
- `security.RandomBytes()` - Cryptographic randomness
- `security.SignData()` - Cryptographic signing
