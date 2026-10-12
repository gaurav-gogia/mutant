package security

func probeOne(name string) AntiTamperSignal {
	switch name {
	case ProbeTiming:
		return detectTiming()
	case ProbeFridaPtrace:
		return detectFridaPtrace()
	case ProbeSleepDrift:
		return detectSleepDrift()
	case ProbeIATGOT:
		return detectIATGOT()
	case ProbeSyscallTable:
		return detectSyscallTable()
	case ProbeTrampoline:
		return detectTrampoline()
	case ProbeProcessInjection:
		return detectProcessInjection()
	case ProbeModuleIntegrity:
		return detectModuleIntegrity()
	case ProbeMemoryPageAnomaly:
		return detectMemoryPageAnomaly()
	case "":
		return unmeasuredSignal("", AntiTamperDetailUnknownProbe)
	default:
		return unmeasuredSignal(name, AntiTamperDetailUnknownProbe)
	}
}
