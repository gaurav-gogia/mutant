package builtin

import (
	"errors"
	"strings"
	"testing"
	"time"

	"mutant/object"
)

// A zero timeout is the polling idiom, so polling a task that has already
// finished must collect it on every poll, whether the task produced a value or
// failed. Deciding by which select case fired let a zero-length timer, which is
// due at once, win about half of those polls and call the finished task still
// running; a thousand rounds leave no room to pass that coin toss by luck.
func TestTaskWaitZeroTimeoutCollectsAFinishedTaskEveryTime(t *testing.T) {
	const rounds = 1000
	stillRunning, first := 0, ""
	for i := 0; i < rounds; i++ {
		handle := registerTestTask(t)
		failed := i%2 == 1
		if failed {
			CompleteTask(handle, nil, errors.New("boom"))
		} else {
			CompleteTask(handle, &object.Integer{Value: int64(i)}, nil)
		}

		result, err := unwrapPair(t, TaskWait(&object.Integer{Value: handle}, &object.Integer{Value: 0}))
		if err != nil && strings.Contains(err.Message, "still running") {
			stillRunning++
			if first == "" {
				first = err.Message
			}
			// That poll kept the handle; collect it so the registry stays clean.
			unwrapPair(t, TaskWait(&object.Integer{Value: handle}))
			continue
		}
		if failed {
			if err == nil {
				t.Fatalf("round %d: task_wait(h, 0) on a failed task returned %s and no error", i, result.Inspect())
			}
			if err.Message != "task failed: boom" {
				t.Fatalf("round %d: error = %q, want the task's own failure", i, err.Message)
			}
		} else {
			if err != nil {
				t.Fatalf("round %d: task_wait(h, 0) on a finished task failed: %s", i, err.Message)
			}
			if got, ok := result.(*object.Integer); !ok || got.Value != int64(i) {
				t.Fatalf("round %d: task_wait(h, 0) returned %s, want %d", i, result.Inspect(), i)
			}
		}
		// Collecting releases the handle, the same as a wait without a timeout.
		if _, gone := unwrapPair(t, TaskDone(&object.Integer{Value: handle})); gone == nil {
			t.Fatalf("round %d: task_wait(h, 0) collected the task but did not release its handle", i)
		}
	}
	if stillRunning > 0 {
		t.Fatalf("task_wait(h, 0) reported a finished task as still running in %d of %d polls; first error: %q", stillRunning, rounds, first)
	}
}

// A poll does not wait: a task that is still running is reported as such at
// once, and the poll keeps the handle so the task can be collected later -- by
// another poll, once it has finished.
func TestTaskWaitZeroTimeoutReportsARunningTaskAndKeepsItsHandle(t *testing.T) {
	handle := registerTestTask(t)
	defer CompleteTask(handle, &object.Null{}, nil)

	_, err := unwrapPair(t, TaskWait(&object.Integer{Value: handle}, &object.Integer{Value: 0}))
	if err == nil {
		t.Fatal("task_wait(h, 0) reported success for a task that has not finished")
	}
	if err.Message != "task_wait: task is still running after 0ms" {
		t.Fatalf("error = %q, want it to say the task is still running", err.Message)
	}

	CompleteTask(handle, &object.String{Value: "later"}, nil)

	result, err := unwrapPair(t, TaskWait(&object.Integer{Value: handle}, &object.Integer{Value: 0}))
	if err != nil {
		t.Fatalf("polling the task once it had finished failed: %s", err.Message)
	}
	if got, ok := result.(*object.String); !ok || got.Value != "later" {
		t.Fatalf("task_wait(h, 0) returned %s, want \"later\"", result.Inspect())
	}
}

// A timed wait hands the task over as soon as it finishes, not when the timer
// runs out: the timeout here is a minute, far longer than the task takes.
func TestTaskWaitTimedWaitCollectsATaskThatFinishesDuringIt(t *testing.T) {
	handle := registerTestTask(t)
	defer CompleteTask(handle, &object.Null{}, nil)

	go func() {
		time.Sleep(5 * time.Millisecond)
		CompleteTask(handle, &object.String{Value: "during"}, nil)
	}()

	start := time.Now()
	result, err := unwrapPair(t, TaskWait(&object.Integer{Value: handle}, &object.Integer{Value: 60000}))
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("task_wait failed for a task that finished during the wait: %s", err.Message)
	}
	if got, ok := result.(*object.String); !ok || got.Value != "during" {
		t.Fatalf("task_wait returned %s, want \"during\"", result.Inspect())
	}
	if elapsed > 30*time.Second {
		t.Fatalf("task_wait returned after %s, so it sat out its timer instead of collecting the task", elapsed)
	}
}
