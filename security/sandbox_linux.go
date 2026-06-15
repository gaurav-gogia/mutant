//go:build linux
// +build linux

package security

import (
	"os"
	"strings"
)

func detectSandboxLinux() (sandboxDetection, error) {
	var detection sandboxDetection

	typeScore := map[string]int{}
	indicators := make([]string, 0, 8)

	add := func(kind string, confidence int, indicator string) {
		if confidence <= 0 {
			return
		}
		typeScore[kind] += confidence
		indicators = append(indicators, indicator)
	}

	if fileExists(LNX_DCKR_ENV_0) {
		add(DOCKER, 85, LNX_DCKR_ENV_1)
	}
	if fileExists("/run/.containerenv") {
		add("Container", 75, "linux:file:/run/.containerenv")
	}
	if envSet("KUBERNETES_SERVICE_HOST") || envSet("KUBERNETES_SERVICE_PORT") {
		add("Kubernetes", 85, "linux:env:kubernetes_service")
	}
	if envSet("WSL_INTEROP") || envSet("WSL_DISTRO_NAME") {
		add("WSL", 95, "linux:env:wsl")
	}

	for _, cgroupPath := range []string{"/proc/1/cgroup", "/proc/self/cgroup"} {
		data, err := os.ReadFile(cgroupPath)
		if err != nil {
			return detection, err
		}

		cgroup := strings.ToLower(string(data))
		if strings.Contains(cgroup, "kubepods") {
			add("Kubernetes", 80, "linux:cgroup:kubepods")
		}

		dckrs := []string{"docker", "containerd", "podman", "libpod", "crio"}
		if containsAny(cgroup, dckrs) {
			add(DOCKER, 70, "linux:cgroup:container_runtime")
		}

		if strings.Contains(cgroup, "lxc") {
			add("LXC", 75, "linux:cgroup:lxc")
		}
		if strings.Contains(cgroup, "machine.slice") && strings.Contains(cgroup, "systemd") {
			add("systemd-nspawn", 60, "linux:cgroup:systemd_nspawn")
		}
	}

	if data, err := os.ReadFile("/proc/cpuinfo"); err == nil {
		cpu := strings.ToLower(string(data))
		if strings.Contains(cpu, "hypervisor") {
			add("VM", 30, "linux:cpu:hypervisor_flag")
		}
		if strings.Contains(cpu, "kvm") || strings.Contains(cpu, "qemu") {
			add("KVM/QEMU", 80, "linux:cpu:kvm_qemu")
		}
		if strings.Contains(cpu, "vmware") {
			add("VMware", 80, "linux:cpu:vmware")
		}
		if strings.Contains(cpu, "virtualbox") {
			add("VirtualBox", 80, "linux:cpu:virtualbox")
		}
		if strings.Contains(cpu, "xen") {
			add("Xen", 80, "linux:cpu:xen")
		}
	}

	if data, err := os.ReadFile("/proc/version"); err == nil {
		kernel := strings.ToLower(string(data))
		if strings.Contains(kernel, "microsoft") {
			add("WSL", 90, "linux:kernel:microsoft")
		}
	}

	for _, dmiPath := range []string{"/sys/class/dmi/id/product_name", "/sys/class/dmi/id/sys_vendor", "/sys/class/dmi/id/board_vendor"} {
		if data, err := os.ReadFile(dmiPath); err == nil {
			v := strings.ToLower(string(data))
			if strings.Contains(v, "vmware") {
				add("VMware", 75, "linux:dmi:vmware")
			}
			if strings.Contains(v, "virtualbox") {
				add("VirtualBox", 75, "linux:dmi:virtualbox")
			}
			if strings.Contains(v, "kvm") || strings.Contains(v, "qemu") {
				add("KVM/QEMU", 75, "linux:dmi:kvm_qemu")
			}
			if strings.Contains(v, "xen") {
				add("Xen", 75, "linux:dmi:xen")
			}
			if strings.Contains(v, "microsoft corporation") && strings.Contains(v, "virtual") {
				add("Hyper-V", 75, "linux:dmi:hyperv")
			}
		}
	}

	if fileExists("/proc/xen") {
		add("Xen", 85, "linux:file:/proc/xen")
	}

	bestType := ""
	bestScore := 0
	for kind, score := range typeScore {
		if score > bestScore {
			bestType = kind
			bestScore = score
		}
	}

	if bestScore > 100 {
		bestScore = 100
	}

	detection.Type = bestType
	detection.Confidence = bestScore
	detection.Indicators = uniqueStrings(indicators)
	return detection, nil
}
