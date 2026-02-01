# Sandbox Detection - Quick Start Guide

## Basic Usage

### Check if Running in Sandbox
```go
if security.IsSandboxed() {
    // Handle sandbox environment
}
```

### Identify Specific Environment
```go
sandboxType, confidence := security.DetectSandboxType()
// sandboxType: "Docker", "VMware", "VirtualBox", etc.
// confidence: 0-100 (higher = more certain)
```

### Get All Indicators
```go
indicators := security.GetSandboxIndicators()
// Returns: []string of detected indicators
```

## Common Patterns

### Block Execution in VM
```go
if security.IsSandboxed() {
    log.Fatal("Cannot run in virtualized environment")
}
```

### Warn on Suspicion
```go
if _, conf := security.DetectSandboxType(); conf > 50 {
    log.Println("WARNING: Possible sandbox detected")
}
```

### Detailed Logging
```go
if security.IsSandboxed() {
    indicators := security.GetSandboxIndicators()
    log.Printf("Sandbox detected with %d indicators:\n", len(indicators))
    for _, ind := range indicators {
        log.Printf("  - %s\n", ind)
    }
}
```

### Tiered Response
```go
sandboxType, conf := security.DetectSandboxType()
switch {
case conf > 90:
    // Definite sandbox - terminate
    log.Fatal("Sandbox confirmed: " + sandboxType)
case conf > 70:
    // Likely sandbox - restricted mode
    log.Println("Restricted mode - sandbox likely: " + sandboxType)
case conf > 50:
    // Possible sandbox - warning
    log.Println("WARNING: Possible sandbox: " + sandboxType)
}
```

## What Gets Detected

| Platform | Detects |
|----------|---------|
| **Windows** | VMware, VirtualBox, Hyper-V, Parallels, Sandboxie, Windows Defender Sandbox, Cuckoo, WINE, Xen, KVM/QEMU |
| **Linux** | Docker, Kubernetes, LXC, systemd-nspawn, KVM/QEMU, VMware, VirtualBox, Xen, Cuckoo |
| **macOS** | Parallels, VMware Fusion, VirtualBox, UTM, Colima, macOS App Sandbox, QEMU |

## Confidence Levels

- **90-100**: Definite - almost certain detection
- **70-89**: High - very likely correct
- **50-69**: Medium - possible but not certain
- **30-49**: Low - weak indicator
- **0-29**: Very Low - barely detectable
- **0**: Not detected

## Performance

- **IsSandboxed()**: ~430µs
- **DetectSandboxType()**: ~400µs
- **GetSandboxIndicators()**: ~590µs

Suitable for startup checks and periodic monitoring.

## Files Added

| File | Purpose |
|------|---------|
| `sandbox.go` | Main API |
| `sandbox_windows.go` | Windows implementation |
| `sandbox_linux.go` | Linux implementation |
| `sandbox_darwin.go` | macOS implementation |
| `sandbox_test.go` | Tests & benchmarks |
| `SANDBOX_DETECTION.md` | Full documentation |
| `sandbox_detection_examples.go` | Usage examples |

## Integration with Security Stack

```go
// Full security check
if security.IsDebuggerPresent() {
    log.Fatal("Debugger detected")
}
if security.IsSandboxed() {
    log.Fatal("Sandbox detected")
}
// Continue if both checks pass
```

## Environment Examples

### Docker Container
```
indicators: ["Docker container detected"]
type: "Docker"
confidence: 95%
```

### VMware VM
```
indicators: ["VMware detected"]
type: "VMware"
confidence: 90%
```

### Windows Defender Sandbox
```
indicators: ["Windows Defender Sandbox detected"]
type: "Windows Defender Sandbox"
confidence: 95%
```

### Native System
```
indicators: []
type: ""
confidence: 0%
```

## Tips & Tricks

1. **Startup Check**: Run once at application start
2. **Periodic Monitoring**: Check in security heartbeat
3. **Detailed Analysis**: Use `GetSandboxIndicators()` for logging
4. **Combine Checks**: Use with `IsDebuggerPresent()` for better coverage
5. **Threshold Detection**: Set custom confidence thresholds for your app

## Related Functions

- `security.IsDebuggerPresent()` - Debugger detection
- `security.SecureGlobal` - Memory encryption
- `security.SignData()` - Cryptographic signing
- `security.RandomBytes()` - Secure randomness
