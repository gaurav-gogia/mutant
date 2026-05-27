package security

func shouldTriggerDebuggerByWeight(highConfidence bool, weakHits, weakThreshold int) bool {
	if highConfidence {
		return true
	}

	if weakThreshold <= 0 {
		weakThreshold = 1
	}

	return weakHits >= weakThreshold
}
