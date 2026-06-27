use serde::{Deserialize, Serialize};
use std::ffi::{CStr, CString};
use std::os::raw::c_char;
use std::time::Instant;

#[cfg(target_os = "linux")]
use std::fs;
#[cfg(target_os = "windows")]
use std::process::Command;

#[cfg(target_os = "windows")]
fn has_virtualization_marker(input: &str) -> Option<&'static str> {
    let lower = input.to_lowercase();
    let markers = [
        ("vmware", "vmware"),
        ("virtualbox", "virtualbox"),
        ("qemu", "qemu"),
        ("kvm", "kvm"),
        ("xen", "xen"),
        ("hyper-v", "hyper-v"),
        ("virtual machine", "virtual machine"),
        ("parallels", "parallels"),
    ];

    for (needle, label) in markers {
        if lower.contains(needle) {
            return Some(label);
        }
    }
    None
}

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
        #[cfg(target_arch = "x86_64")]
        {
            type WinBool = i32;

            const CONTEXT_AMD64: u32 = 0x0010_0000;
            const CONTEXT_DEBUG_REGISTERS: u32 = CONTEXT_AMD64 | 0x0000_0010;

            #[repr(C, align(16))]
            #[derive(Copy, Clone)]
            struct M128A {
                low: u64,
                high: i64,
            }

            #[repr(C)]
            #[derive(Copy, Clone)]
            struct XmmSaveArea32 {
                control_word: u16,
                status_word: u16,
                tag_word: u8,
                reserved1: u8,
                error_opcode: u16,
                error_offset: u32,
                error_selector: u16,
                reserved2: u16,
                data_offset: u32,
                data_selector: u16,
                reserved3: u16,
                mx_csr: u32,
                mx_csr_mask: u32,
                float_registers: [M128A; 8],
                xmm_registers: [M128A; 16],
                reserved4: [u8; 96],
            }

            #[repr(C, align(16))]
            struct Context {
                p1_home: u64,
                p2_home: u64,
                p3_home: u64,
                p4_home: u64,
                p5_home: u64,
                p6_home: u64,
                context_flags: u32,
                mx_csr: u32,
                seg_cs: u16,
                seg_ds: u16,
                seg_es: u16,
                seg_fs: u16,
                seg_gs: u16,
                seg_ss: u16,
                eflags: u32,
                dr0: u64,
                dr1: u64,
                dr2: u64,
                dr3: u64,
                dr6: u64,
                dr7: u64,
                rax: u64,
                rcx: u64,
                rdx: u64,
                rbx: u64,
                rsp: u64,
                rbp: u64,
                rsi: u64,
                rdi: u64,
                r8: u64,
                r9: u64,
                r10: u64,
                r11: u64,
                r12: u64,
                r13: u64,
                r14: u64,
                r15: u64,
                rip: u64,
                flt_save: XmmSaveArea32,
                vector_register: [M128A; 26],
                vector_control: u64,
                debug_control: u64,
                last_branch_to_rip: u64,
                last_branch_from_rip: u64,
                last_exception_to_rip: u64,
                last_exception_from_rip: u64,
            }

            #[link(name = "kernel32")]
            extern "system" {
                fn GetCurrentThread() -> *mut core::ffi::c_void;
                fn GetThreadContext(
                    hthread: *mut core::ffi::c_void,
                    lpcontext: *mut Context,
                ) -> WinBool;
            }

            // SAFETY: Context is plain-old-data and zero initialization matches Win32 API expectations.
            let mut ctx: Context = unsafe { std::mem::zeroed() };
            ctx.context_flags = CONTEXT_DEBUG_REGISTERS;

            // SAFETY: GetCurrentThread returns a pseudo-handle valid for the current thread,
            // and GetThreadContext writes into the provided CONTEXT structure.
            let ok = unsafe { GetThreadContext(GetCurrentThread(), &mut ctx as *mut Context) };
            if ok == 0 {
                return make_signal("hardware_breakpoint", false, 0, "GetThreadContext failed");
            }

            let enabled_mask = ctx.dr7 & 0xFF;
            let addr_regs = [ctx.dr0, ctx.dr1, ctx.dr2, ctx.dr3];
            let active_slots = addr_regs.iter().filter(|&&r| r != 0).count();

            let suspicious = enabled_mask != 0 || active_slots > 0;
            if suspicious {
                let confidence = if enabled_mask != 0 && active_slots > 0 {
                    95
                } else if enabled_mask != 0 {
                    85
                } else {
                    75
                };

                return make_signal(
                    "hardware_breakpoint",
                    true,
                    confidence,
                    format!(
                        "dr0=0x{:x};dr1=0x{:x};dr2=0x{:x};dr3=0x{:x};dr7=0x{:x};enabled_mask=0x{:x};active_slots={}",
                        ctx.dr0, ctx.dr1, ctx.dr2, ctx.dr3, ctx.dr7, enabled_mask, active_slots
                    ),
                );
            }

            return make_signal(
                "hardware_breakpoint",
                false,
                0,
                format!(
                    "dr7=0x{:x};enabled_mask=0x{:x};active_slots=0",
                    ctx.dr7, enabled_mask
                ),
            );
        }

        #[cfg(target_arch = "x86")]
        {
            type WinBool = i32;

            const CONTEXT_I386: u32 = 0x0001_0000;
            const CONTEXT_DEBUG_REGISTERS: u32 = CONTEXT_I386 | 0x0000_0010;

            #[repr(C)]
            struct FloatingSaveArea {
                control_word: u32,
                status_word: u32,
                tag_word: u32,
                error_offset: u32,
                error_selector: u32,
                data_offset: u32,
                data_selector: u32,
                register_area: [u8; 80],
                cr0_npx_state: u32,
            }

            #[repr(C)]
            struct Context32 {
                context_flags: u32,
                dr0: u32,
                dr1: u32,
                dr2: u32,
                dr3: u32,
                dr6: u32,
                dr7: u32,
                float_save: FloatingSaveArea,
                seg_gs: u32,
                seg_fs: u32,
                seg_es: u32,
                seg_ds: u32,
                edi: u32,
                esi: u32,
                ebx: u32,
                edx: u32,
                ecx: u32,
                eax: u32,
                ebp: u32,
                eip: u32,
                seg_cs: u32,
                eflags: u32,
                esp: u32,
                seg_ss: u32,
                extended_registers: [u8; 512],
            }

            #[link(name = "kernel32")]
            extern "system" {
                fn GetCurrentThread() -> *mut core::ffi::c_void;
                fn GetThreadContext(
                    hthread: *mut core::ffi::c_void,
                    lpcontext: *mut Context32,
                ) -> WinBool;
            }

            // SAFETY: Context is plain-old-data and zero initialization matches Win32 API expectations.
            let mut ctx: Context32 = unsafe { std::mem::zeroed() };
            ctx.context_flags = CONTEXT_DEBUG_REGISTERS;

            // SAFETY: GetCurrentThread returns a pseudo-handle valid for the current thread,
            // and GetThreadContext writes into the provided CONTEXT structure.
            let ok = unsafe { GetThreadContext(GetCurrentThread(), &mut ctx as *mut Context32) };
            if ok == 0 {
                return make_signal("hardware_breakpoint", false, 0, "GetThreadContext failed");
            }

            let enabled_mask = ctx.dr7 & 0xFF;
            let addr_regs = [ctx.dr0, ctx.dr1, ctx.dr2, ctx.dr3];
            let active_slots = addr_regs.iter().filter(|&&r| r != 0).count();

            let suspicious = enabled_mask != 0 || active_slots > 0;
            if suspicious {
                let confidence = if enabled_mask != 0 && active_slots > 0 {
                    95
                } else if enabled_mask != 0 {
                    85
                } else {
                    75
                };

                return make_signal(
                    "hardware_breakpoint",
                    true,
                    confidence,
                    format!(
                        "dr0=0x{:x};dr1=0x{:x};dr2=0x{:x};dr3=0x{:x};dr7=0x{:x};enabled_mask=0x{:x};active_slots={}",
                        ctx.dr0, ctx.dr1, ctx.dr2, ctx.dr3, ctx.dr7, enabled_mask, active_slots
                    ),
                );
            }

            return make_signal(
                "hardware_breakpoint",
                false,
                0,
                format!(
                    "dr7=0x{:x};enabled_mask=0x{:x};active_slots=0",
                    ctx.dr7, enabled_mask
                ),
            );
        }

        #[cfg(not(any(target_arch = "x86_64", target_arch = "x86")))]
        {
            return make_signal(
                "hardware_breakpoint",
                false,
                0,
                "unsupported windows arch for debug register probe",
            );
        }
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

    #[cfg(target_os = "windows")]
    {
        type WinBool = i32;

        #[link(name = "kernel32")]
        extern "system" {
            fn IsDebuggerPresent() -> WinBool;
            fn GetCurrentProcess() -> *mut core::ffi::c_void;
            fn CheckRemoteDebuggerPresent(
                hprocess: *mut core::ffi::c_void,
                debugger_present: *mut WinBool,
            ) -> WinBool;
        }

        let mut reasons: Vec<&str> = vec![];

        // SAFETY: These are leaf Win32 APIs without borrowed pointers except the output flag.
        unsafe {
            if IsDebuggerPresent() != 0 {
                reasons.push("IsDebuggerPresent");
            }

            let mut remote: WinBool = 0;
            let ok = CheckRemoteDebuggerPresent(GetCurrentProcess(), &mut remote as *mut WinBool);
            if ok != 0 && remote != 0 {
                reasons.push("CheckRemoteDebuggerPresent");
            }
        }

        if reasons.is_empty() {
            return make_signal("syscall", false, 0, "no debugger API signal detected");
        }

        return make_signal(
            "syscall",
            true,
            80,
            format!("api_hits={}", reasons.join(",")),
        );
    }

    #[cfg(not(any(target_os = "linux", target_os = "windows")))]
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

    #[cfg(target_os = "windows")]
    {
        if let Ok(out) = Command::new("tasklist").output() {
            let tasks = String::from_utf8_lossy(&out.stdout).to_lowercase();
            let suspicious = ["frida", "frida-helper", "frida-server", "frida-agent"];
            for marker in suspicious {
                if tasks.contains(marker) {
                    return make_signal(
                        "frida_ptrace",
                        true,
                        85,
                        format!("tasklist marker: {marker}"),
                    );
                }
            }
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
    #[cfg(target_os = "windows")]
    {
        let windows_injection_markers = [
            "COR_ENABLE_PROFILING",
            "COR_PROFILER",
            "COR_PROFILER_PATH",
            "__COMPAT_LAYER",
        ];

        let mut present: Vec<String> = vec![];
        for name in windows_injection_markers {
            if let Ok(value) = std::env::var(name) {
                if !value.trim().is_empty() {
                    present.push(format!("{name}={value}"));
                }
            }
        }

        if present.is_empty() {
            return make_signal("ld_preload", false, 0, "no windows injection env markers");
        }

        return make_signal(
            "ld_preload",
            true,
            55,
            format!("windows_env_markers={}", present.join(";")),
        );
    }

    #[cfg(not(target_os = "windows"))]
    {
        let preload = std::env::var("LD_PRELOAD").unwrap_or_default();
        if preload.trim().is_empty() {
            return make_signal("ld_preload", false, 0, "LD_PRELOAD empty");
        }

        return make_signal("ld_preload", true, 85, format!("LD_PRELOAD={preload}"));
    }
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

    #[cfg(target_os = "windows")]
    {
        let mut details: Vec<String> = vec![];

        let commands = [
            (
                "wmic computersystem",
                Command::new("wmic")
                    .args(["computersystem", "get", "manufacturer,model"])
                    .output(),
            ),
            ("systeminfo", Command::new("systeminfo").output()),
        ];

        let mut marker_hit: Option<&'static str> = None;
        for (source, output) in commands {
            if let Ok(out) = output {
                let text = String::from_utf8_lossy(&out.stdout).to_string();
                if !text.trim().is_empty() {
                    details.push(format!("{source}={}", text.replace('\n', " ").trim()));
                }
                if marker_hit.is_none() {
                    marker_hit = has_virtualization_marker(&text);
                }
            }
        }

        if let Some(marker) = marker_hit {
            return make_signal(
                "acpi_pci",
                true,
                60,
                format!("windows_platform_marker={marker}"),
            );
        }

        return make_signal(
            "acpi_pci",
            false,
            0,
            if details.is_empty() {
                "no windows system metadata available".to_string()
            } else {
                "windows system metadata present without virtualization marker".to_string()
            },
        );
    }

    #[cfg(not(any(target_os = "linux", target_os = "windows")))]
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
