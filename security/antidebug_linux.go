//go:build linux
// +build linux

package security

import (
	"os"
	"strconv"
	"strings"
)

// isDebuggerPresentLinux checks if a debugger is attached by examining /proc/self/status.
// The TracerPid field indicates the PID of the process tracing this one.
// A value of 0 means no tracer, any other value means we're being debugged.
func isDebuggerPresentLinux() bool {
	data, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return false
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "TracerPid:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				pid, err := strconv.Atoi(fields[1])
				if err == nil && pid != 0 {
					return true
				}
			}
			break
		}
	}

	return false
}
