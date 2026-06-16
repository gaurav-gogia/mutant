package errrs

import (
	"math/rand"
	"strings"
	"testing"
)

func TestMessageCyclerNoRepeatWithinCycle(t *testing.T) {
	cycler := newMessageCycler(
		[]string{"a", "b", "c", "d"},
		rand.New(rand.NewSource(7)),
	)

	seen := map[string]bool{}
	for range 4 {
		msg := cycler.next()
		if seen[msg] {
			t.Fatalf("duplicate message in same cycle: %q", msg)
		}
		seen[msg] = true
	}
}

func TestMessageCyclerAvoidsImmediateBoundaryRepeat(t *testing.T) {
	cycler := newMessageCycler(
		[]string{"one", "two", "three"},
		rand.New(rand.NewSource(1)),
	)

	firstCycle := []string{cycler.next(), cycler.next(), cycler.next()}
	next := cycler.next()
	if firstCycle[len(firstCycle)-1] == next {
		t.Fatalf("expected no immediate repeat across cycle boundary, got %q", next)
	}
}

func TestPrintMachineErrorIncludesFriendlyHeaderAndDetails(t *testing.T) {
	machineComfortCycler = newMessageCycler(
		[]string{"Friendly VM hug message."},
		rand.New(rand.NewSource(99)),
	)

	var b strings.Builder
	PrintMachineError(&b, "boom")
	out := b.String()

	if !strings.Contains(out, "Friendly VM hug message.") {
		t.Fatalf("expected friendly message in output, got: %q", out)
	}
	if !strings.Contains(out, "vm error:") {
		t.Fatalf("expected vm error label in output, got: %q", out)
	}
	if !strings.Contains(out, "boom") {
		t.Fatalf("expected original vm error details in output, got: %q", out)
	}
}

func TestPrintParseErrorsIncludesFriendlyHeaderAndDetails(t *testing.T) {
	parserComfortCycler = newMessageCycler(
		[]string{"Friendly parser pep talk."},
		rand.New(rand.NewSource(100)),
	)

	var b strings.Builder
	PrintParseErrors(&b, []string{"unexpected token"})
	out := b.String()

	if !strings.Contains(out, "Friendly parser pep talk.") {
		t.Fatalf("expected parser friendly message in output, got: %q", out)
	}
	if !strings.Contains(out, "parser errors:") {
		t.Fatalf("expected parser error label in output, got: %q", out)
	}
	if !strings.Contains(out, "unexpected token") {
		t.Fatalf("expected original parser error details in output, got: %q", out)
	}
}

func TestPrintCompilerErrorIncludesFriendlyHeaderAndDetails(t *testing.T) {
	compilerComfortCycler = newMessageCycler(
		[]string{"Friendly compiler cheer."},
		rand.New(rand.NewSource(101)),
	)

	var b strings.Builder
	PrintCompilerError(&b, "cannot compile node")
	out := b.String()

	if !strings.Contains(out, "Friendly compiler cheer.") {
		t.Fatalf("expected compiler friendly message in output, got: %q", out)
	}
	if !strings.Contains(out, "compiler error:") {
		t.Fatalf("expected compiler error label in output, got: %q", out)
	}
	if !strings.Contains(out, "cannot compile node") {
		t.Fatalf("expected original compiler error details in output, got: %q", out)
	}
}
