package security

import (
	"testing"
	"time"
)

// answeringAttempts is how many times a test starts a shell it needs an answer
// from before a timeout counts against what the test is about.
//
// Every run is bound by the product's own three-second timeout, which is the
// right limit for a script and the wrong one for a test sharing a saturated
// host. Under the release gate, `bash -c 'echo hi'` (Git Bash, whose startup
// emulates fork) answered in 0.2 s on its own and timed out at 3 s beside the
// rest of the suite, three tests at once. A run that timed out says nothing about
// argv, quoting or line joining, so it is run again; a run that answered is
// judged exactly as it came back.
//
// Three attempts back to back were not enough: at the next gate all three timed
// out inside ten seconds, in the same spike of load. So each retry waits first,
// one second and then twice as long as the wait before, and five timeouts in a
// row, spread over about half a minute, still fail.
const answeringAttempts = 5

// answeringFirstPause is how long untilAnswered waits before its first retry.
const answeringFirstPause = time.Second

// untilAnswered runs a command that is expected to answer, again only when it
// timed out. It is not for a test whose subject is the timeout: the bounds tests
// call ExecuteCommand directly, and so does anything expecting a refusal.
func untilAnswered(t *testing.T, run func() CommandResult) CommandResult {
	t.Helper()
	var result CommandResult
	pause := answeringFirstPause
	for attempt := 1; ; attempt++ {
		result = run()
		if !result.TimedOut || attempt == answeringAttempts {
			return result
		}
		t.Logf("attempt %d of %d timed out; running it again in %v", attempt, answeringAttempts, pause)
		time.Sleep(pause)
		pause *= 2
	}
}

func executeAnswering(t *testing.T, shell, command, stage string) CommandResult {
	t.Helper()
	return untilAnswered(t, func() CommandResult { return ExecuteCommand(shell, command, stage) })
}

func executeArgvAnswering(t *testing.T, argv []string, stage string) CommandResult {
	t.Helper()
	return untilAnswered(t, func() CommandResult { return ExecuteArgv(argv, stage) })
}
