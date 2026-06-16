use serde::{Deserialize, Serialize};
use std::ffi::{CStr, CString};
use std::os::raw::c_char;
use std::time::Instant;

#[cfg(target_os = "linux")]
use std::fs;

#[derive(Debug, Deserialize)]
struct ProbeRequest {
    #[serde(default = "default_version")]
    version: i32,
    #[serde(default)]
    platform: String,
    #[serde(default)]
    arch: String,
    #[serde(default)]
    requested: Vec<String>,
}

#[derive(Debug, Serialize)]
struct ProbeSignal {
    name: String,
    detected: bool,
    confidence: i32,
    detail: String,
}

#[derive(Debug, Serialize)]
struct ProbeResponse {
    version: i32,
    ok: bool,
    error: String,
    signals: Vec<ProbeSignal>,
}

fn default_version() -> i32 {
    1
}

fn success(signals: Vec<ProbeSignal>) -> ProbeResponse {
    ProbeResponse {
        version: 1,
        ok: true,
        error: String::new(),
        signals,
    }
}

fn failure(message: &str) -> ProbeResponse {
    ProbeResponse {
        version: 1,
        ok: false,
        error: message.to_string(),
        signals: vec![],
    }
}

fn make_signal(
    name: &str,
    detected: bool,
    confidence: i32,
    detail: impl Into<String>,
) -> ProbeSignal {
    ProbeSignal {
        name: name.to_string(),
        detected,
        confidence,
        detail: detail.into(),
    }
}

fn detect_hardware_breakpoint() -> ProbeSignal {
    #[cfg(target_os = "windows")]
    {
        // Placeholder until context inspection is added through Windows debug APIs.
        return make_signal(
            "hardware_breakpoint",
            false,
            0,
            "not implemented on windows yet",
        );
    }

    #[cfg(not(target_os = "windows"))]
    {
        make_signal(
            "hardware_breakpoint",
            false,
            0,
            "not implemented on this platform",
        )
    }
}

fn detect_timing() -> ProbeSignal {
    let start = Instant::now();
    let mut acc: u64 = 0;
    for i in 0..200_000u64 {
        acc ^= i.wrapping_mul(0x9E37_79B9);
    }
    let elapsed_us = start.elapsed().as_micros();

    // This threshold is intentionally conservative to avoid false positives.
    let suspicious = elapsed_us > 200_000;
    let confidence = if suspicious { 40 } else { 5 };
    make_signal(
        "timing",
        suspicious,
        confidence,
        format!("loop_us={elapsed_us};acc={acc}"),
    )
}

fn detect_syscall() -> ProbeSignal {
    #[cfg(target_os = "linux")]
    {
        let status = fs::read_to_string("/proc/self/status").unwrap_or_default();
        let tracer = status
            .lines()
            .find(|l| l.starts_with("TracerPid:"))
            .and_then(|l| l.split(':').nth(1))
            .and_then(|v| v.trim().parse::<u32>().ok())
            .unwrap_or(0);

        if tracer > 0 {
            return make_signal("syscall", true, 80, format!("tracer_pid={tracer}"));
        }

        return make_signal("syscall", false, 0, "no tracer pid detected");
    }

    #[cfg(not(target_os = "linux"))]
    {
        make_signal("syscall", false, 0, "unsupported on this platform")
    }
}

fn detect_frida_ptrace() -> ProbeSignal {
    let frida_markers = ["FRIDA", "FRIDA_AGENT", "FRIDA_GADGET"];
    for marker in frida_markers {
        if std::env::var(marker).is_ok() {
            return make_signal(
                "frida_ptrace",
                true,
                90,
                format!("env marker present: {marker}"),
            );
        }
    }

    #[cfg(target_os = "linux")]
    {
        let status = fs::read_to_string("/proc/self/status").unwrap_or_default();
        let tracer = status
            .lines()
            .find(|l| l.starts_with("TracerPid:"))
            .and_then(|l| l.split(':').nth(1))
            .and_then(|v| v.trim().parse::<u32>().ok())
            .unwrap_or(0);

        if tracer > 0 {
            return make_signal(
                "frida_ptrace",
                true,
                75,
                format!("ptrace tracer pid: {tracer}"),
            );
        }
    }

    make_signal(
        "frida_ptrace",
        false,
        0,
        "no frida/ptrace heuristic triggered",
    )
}

fn detect_ld_preload() -> ProbeSignal {
    let preload = std::env::var("LD_PRELOAD").unwrap_or_default();
    if preload.trim().is_empty() {
        return make_signal("ld_preload", false, 0, "LD_PRELOAD empty");
    }

    make_signal("ld_preload", true, 85, format!("LD_PRELOAD={preload}"))
}

fn detect_cpuid_hypervisor() -> ProbeSignal {
    #[cfg(any(target_arch = "x86", target_arch = "x86_64"))]
    {
        #[cfg(target_arch = "x86")]
        use std::arch::x86::__cpuid;
        #[cfg(target_arch = "x86_64")]
        use std::arch::x86_64::__cpuid;

        let leaf1 = __cpuid(1);
        let hypervisor = (leaf1.ecx & (1 << 31)) != 0;
        return make_signal(
            "cpuid_hypervisor",
            hypervisor,
            if hypervisor { 70 } else { 0 },
            format!("ecx=0x{:08x}", leaf1.ecx),
        );
    }

    #[cfg(not(any(target_arch = "x86", target_arch = "x86_64")))]
    {
        make_signal("cpuid_hypervisor", false, 0, "unsupported arch")
    }
}

fn detect_rdtsc_drift() -> ProbeSignal {
    let start = Instant::now();
    for _ in 0..3 {
        std::thread::sleep(std::time::Duration::from_millis(1));
    }
    let elapsed = start.elapsed().as_millis() as i64;
    let drift = (elapsed - 3).abs();

    let suspicious = drift > 10;
    let confidence = if suspicious { 35 } else { 0 };
    make_signal(
        "rdtsc_drift",
        suspicious,
        confidence,
        format!("sleep_ms={elapsed};drift_ms={drift}"),
    )
}

fn detect_acpi_pci() -> ProbeSignal {
    #[cfg(target_os = "linux")]
    {
        let candidates = [
            "/sys/class/dmi/id/product_name",
            "/sys/class/dmi/id/sys_vendor",
            "/sys/devices/virtual/dmi/id/product_name",
        ];

        let mut details: Vec<String> = vec![];
        let mut suspicious = false;
        for path in candidates {
            if let Ok(content) = fs::read_to_string(path) {
                let lower = content.to_lowercase();
                if lower.contains("vmware")
                    || lower.contains("virtualbox")
                    || lower.contains("qemu")
                {
                    suspicious = true;
                }
                details.push(format!("{path}={}", content.trim()));
            }
        }

        return make_signal(
            "acpi_pci",
            suspicious,
            if suspicious { 55 } else { 0 },
            if details.is_empty() {
                "no dmi sources available".to_string()
            } else {
                details.join(";")
            },
        );
    }

    #[cfg(not(target_os = "linux"))]
    {
        make_signal("acpi_pci", false, 0, "unsupported on this platform")
    }
}

fn detect_gpu_feature() -> ProbeSignal {
    make_signal("gpu_feature", false, 0, "not implemented yet")
}

fn detect_iat_got() -> ProbeSignal {
    make_signal("iat_got", false, 0, "not implemented yet")
}

fn detect_syscall_table() -> ProbeSignal {
    make_signal("syscall_table", false, 0, "not implemented yet")
}

fn detect_trampoline() -> ProbeSignal {
    make_signal("trampoline", false, 0, "not implemented yet")
}

fn probe_one(name: &str) -> ProbeSignal {
    match name {
        "hardware_breakpoint" => detect_hardware_breakpoint(),
        "timing" => detect_timing(),
        "syscall" => detect_syscall(),
        "frida_ptrace" => detect_frida_ptrace(),
        "ld_preload" => detect_ld_preload(),
        "cpuid_hypervisor" => detect_cpuid_hypervisor(),
        "rdtsc_drift" => detect_rdtsc_drift(),
        "acpi_pci" => detect_acpi_pci(),
        "gpu_feature" => detect_gpu_feature(),
        "iat_got" => detect_iat_got(),
        "syscall_table" => detect_syscall_table(),
        "trampoline" => detect_trampoline(),
        other => make_signal(other, false, 0, "unknown probe"),
    }
}

fn run_requested(req: &ProbeRequest) -> ProbeResponse {
    let mut signals: Vec<ProbeSignal> = vec![];
    for item in &req.requested {
        signals.push(probe_one(item));
    }

    let _ = (&req.version, &req.platform, &req.arch);
    success(signals)
}

fn as_json_string(resp: &ProbeResponse) -> CString {
    let body = serde_json::to_string(resp).unwrap_or_else(|_| {
        String::from("{\"version\":1,\"ok\":false,\"error\":\"json_encode_failed\",\"signals\":[]}")
    });

    CString::new(body).unwrap_or_else(|_| {
        CString::new(
            "{\"version\":1,\"ok\":false,\"error\":\"cstring_encode_failed\",\"signals\":[]}",
        )
        .expect("static fallback CString should always succeed")
    })
}

#[no_mangle]
pub extern "C" fn mutant_rust_probe(request: *const c_char) -> *mut c_char {
    if request.is_null() {
        return as_json_string(&failure("nil request pointer")).into_raw();
    }

    // SAFETY: pointer is validated non-null and expected to be a valid C string.
    let c_request = unsafe { CStr::from_ptr(request) };
    let req_str = match c_request.to_str() {
        Ok(v) => v,
        Err(_) => return as_json_string(&failure("request is not valid utf-8")).into_raw(),
    };

    let req: ProbeRequest = match serde_json::from_str(req_str) {
        Ok(v) => v,
        Err(e) => {
            return as_json_string(&failure(&format!("invalid request json: {e}"))).into_raw()
        }
    };

    let response = run_requested(&req);
    as_json_string(&response).into_raw()
}

#[no_mangle]
pub extern "C" fn mutant_rust_free(ptr: *mut c_char) {
    if ptr.is_null() {
        return;
    }

    // SAFETY: pointer must come from CString::into_raw in mutant_rust_probe.
    unsafe {
        let _ = CString::from_raw(ptr);
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn parse_and_run() {
        let req = ProbeRequest {
            version: 1,
            platform: "linux".to_string(),
            arch: "amd64".to_string(),
            requested: vec!["timing".to_string(), "ld_preload".to_string()],
        };
        let resp = run_requested(&req);
        assert!(resp.ok);
        assert_eq!(resp.signals.len(), 2);
    }
}
