package builtin

import (
	"testing"

	"mutant/object"
)

// commandAnswerAttempts is security's answeringAttempts at the builtin: a
// command builtin's run is bound by the product's three-second timeout, and
// under the release gate `bash -c 'echo hi'` timed out at that bound on a
// saturated host while answering in 0.2 s on its own. A timed-out run says
// nothing about what these tests check, so it is run again; three in a row
// still fail.
const commandAnswerAttempts = 3

// untilAnswered calls a command builtin that is expected to answer, again only
// when its result hash says timed_out. Anything else -- an error object, a
// result of another shape -- comes back on the first call for the test to judge.
func untilAnswered(t *testing.T, call func() object.Object) object.Object {
	t.Helper()
	for attempt := 1; ; attempt++ {
		answer := call()
		if attempt == commandAnswerAttempts || !answerTimedOut(answer) {
			return answer
		}
		t.Logf("attempt %d of %d timed out; running it again", attempt, commandAnswerAttempts)
	}
}

func answerTimedOut(answer object.Object) bool {
	result, errObj := unwrapPairNoFatal(answer)
	if errObj != nil {
		return false
	}
	hash, isHash := result.(*object.Hash)
	if !isHash {
		return false
	}
	timedOut, isBool := hashValueByKey(hash, "timed_out").(*object.Boolean)
	return isBool && timedOut.Value
}
