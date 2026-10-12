package security

import "testing"

// answeringAttempts is how many times a test starts a shell it needs an answer
// from before a timeout counts against what the test is about.
//
// Every run is bound by the product's own three-second timeout, which is the
// right limit for a script and the wrong one for a test sharing a saturated
// host. Under the release gate, `bash -c 'echo hi'` (Git Bash, whose startup
// emulates fork) answered in 0.2 s on its own and timed out at 3 s beside the
// rest of the suite, three tests at once. A run that timed out says nothing about
// argv, quoting or line joining, so it is run again; a run that answered is
// judged exactly as it came back, and three timeouts in a row still fail.
const answeringAttempts = 3

// untilAnswered runs a command that is expected to answer, again only when it
// timed out. It is not for a test whose subject is the timeout: the bounds tests
// call ExecuteCommand directly, and so does anything expecting a refusal.
func untilAnswered(t *testing.T, run func() CommandResult) CommandResult {
	t.Helper()
	var result CommandResult
	for attempt := 1; ; attempt++ {
		result = run()
		if !result.TimedOut || attempt == answeringAttempts {
			return result
		}
		t.Logf("attempt %d of %d timed out; running it again", attempt, answeringAttempts)
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
