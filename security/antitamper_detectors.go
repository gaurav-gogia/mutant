package security

import (
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

func detectTiming() AntiTamperSignal {
	start := time.Now()
	var acc uint64
	for i := uint64(0); i < TimingLoopIterations; i++ {
		acc ^= i * TimingLoopXORConstant
	}
	elapsedUs := time.Since(start).Microseconds()
	suspicious := elapsedUs > TimingSuspiciousThresholdUs
	confidence := ConfidenceTimingBaseline
	if suspicious {
		confidence = ConfidenceTimingSuspicious
	}

	return makeSignal(
		ProbeTiming,
		suspicious,
		confidence,
		"loop_us="+strconv.FormatInt(elapsedUs, 10)+";acc="+strconv.FormatUint(acc, 10),
	)
}

func detectFridaPtrace() AntiTamperSignal {
	for _, marker := range fridaEnvMarkers {
		if _, ok := os.LookupEnv(marker); ok {
			return makeSignal(ProbeFridaPtrace, true, ConfidenceFridaEnvMarkerDetected, "env marker present: "+marker)
		}
	}

	if runtime.GOOS == "linux" {
		if tracer, ok := readLinuxTracerPID(); ok && tracer > 0 {
			return makeSignal(ProbeFridaPtrace, true, ConfidenceFridaPtraceTracerDetected, "ptrace tracer pid: "+strconv.Itoa(tracer))
		}
	}

	if runtime.GOOS == "windows" {
		out, err := exec.Command("tasklist").Output()
		if err == nil {
			tasks := strings.ToLower(string(out))
			for _, marker := range fridaTaskMarkers {
				if strings.Contains(tasks, marker) {
					return makeSignal(ProbeFridaPtrace, true, ConfidenceFridaTasklistDetected, "tasklist marker: "+marker)
				}
			}
		}
	}

	return makeSignal(ProbeFridaPtrace, false, ConfidenceNone, "no frida/ptrace heuristic triggered")
}

// sleepDriftNap is one of the sleep_drift probe's naps: the shortest sleep
// worth asking the scheduler for.
//
//mutant:limit duration
const sleepDriftNap = time.Millisecond

// detectSleepDrift times SleepDriftNaps naps on the OS clock and reports how far
// their total ran from what they asked for. It was called rdtsc_drift, and that
// named a check it does not make: an RDTSC check reads the processor's
// timestamp counter, and this compares the OS clock with its own sleeps
// (M26-TMP-014).
func detectSleepDrift() AntiTamperSignal {
	start := time.Now()
	for i := 0; i < SleepDriftNaps; i++ {
		time.Sleep(sleepDriftNap)
	}
	elapsedMs := int(time.Since(start).Milliseconds())
	drift := elapsedMs - SleepDriftNaps*int(sleepDriftNap/time.Millisecond)
	if drift < 0 {
		drift = -drift
	}

	suspicious := drift > SleepDriftThresholdMs
	confidence := ConfidenceNone
	if suspicious {
		confidence = ConfidenceSleepDriftSuspicious
	}

	return makeSignal(
		ProbeSleepDrift,
		suspicious,
		confidence,
		"sleep_ms="+strconv.Itoa(elapsedMs)+";drift_ms="+strconv.Itoa(drift),
	)
}

func readLinuxTracerPID() (int, bool) {
	if runtime.GOOS != "linux" {
		return 0, false
	}

	status, err := os.ReadFile(LinuxProcSelfStatusPath)
	if err != nil {
		return 0, false
	}

	for _, line := range strings.Split(string(status), "\n") {
		if !strings.HasPrefix(line, "TracerPid:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return 0, false
		}
		pid, parseErr := strconv.Atoi(fields[1])
		if parseErr != nil {
			return 0, false
		}
		return pid, true
	}

	return 0, false
}
