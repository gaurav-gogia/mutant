package builtin

import (
	"strings"
	"testing"

	"mutant/object"
)

// A reclassification goes one way: the new record supersedes the old. Recording
// a record as reclassifying one that already supersedes it, directly or through
// the records it supersedes in turn, would leave the ledger holding a history
// in which each record is older than the other, which no reclassification can
// make. Each such call is refused, names the records the cycle would close
// through, and writes nothing; the reclassifications that came before it, and a
// record reclassifying two, stay as they were (M26-REC-024).
func TestDiscloseReclassifiedRefusesACycleREC024(t *testing.T) {
	f := newDiscloseFixture(t)
	ledger := intObj(f.ledger)
	a, aUID := f.record, recordUIDOf(t, f.record)
	b, bUID := f.reseal(t, recordArray(
		mustHash(t, RecordClassifyRange(intObj(100), intObj(150), stringObj("restricted"))),
	))
	c, cUID := f.reseal(t, recordArray(
		mustHash(t, RecordClassifyRange(intObj(250), intObj(50), stringObj("pii"))),
	))
	session, _ := ledgerGet(f.ledger)
	supersedes := func(uid string) string {
		t.Helper()
		got, err := redactionReclassifies(session, uid)
		if err != nil {
			t.Fatal(err)
		}
		return strings.Join(got, ",")
	}

	// Directly: b reclassifies a, so a cannot reclassify b.
	mustHash(t, DiscloseReclassified(ledger, b, a))
	mustRefuse(t, "a record reclassifying the record that reclassified it", DiscloseReclassified(ledger, a, b),
		"record "+bUID+" already supersedes record "+aUID+" ("+bUID+" reclassifies "+aUID+")")
	if got := supersedes(aUID); got != "" {
		t.Fatalf("the refused reclassification was recorded anyway: %s supersedes %q", aUID, got)
	}

	// Along the line: c reclassifies b, which reclassifies a, so a cannot
	// reclassify c.
	mustHash(t, DiscloseReclassified(ledger, c, b))
	mustRefuse(t, "a record reclassifying the head of its own line", DiscloseReclassified(ledger, a, c),
		"record "+cUID+" already supersedes record "+aUID+" ("+cUID+" reclassifies "+bUID+", "+bUID+" reclassifies "+aUID+")")
	if got := supersedes(aUID); got != "" {
		t.Fatalf("the refused reclassification was recorded anyway: %s supersedes %q", aUID, got)
	}

	// The pairs recorded before are untouched, and asking them again is free.
	if got := supersedes(cUID) + "|" + supersedes(bUID); got != bUID+"|"+aUID {
		t.Fatalf("the ledger's reclassifications are %s, want %s|%s", got, bUID, aUID)
	}
	for _, again := range []*object.Hash{
		mustHash(t, DiscloseReclassified(ledger, b, a)),
		mustHash(t, DiscloseReclassified(ledger, c, b)),
	} {
		if mustHashBoolValue(t, again, "recorded") || !mustHashBoolValue(t, again, "already_recorded") {
			t.Fatalf("asking a recorded reclassification again gave %s", again.Inspect())
		}
	}
}

// A record reclassifying two is a consolidation and stays allowed, but neither
// record it consolidates may then be recorded as reclassifying it: the cycle
// runs through one branch and not along a single line, and is refused all the
// same (M26-REC-024).
func TestDiscloseReclassifiedRefusesACycleThroughAConsolidationREC024(t *testing.T) {
	f := newDiscloseFixture(t)
	ledger := intObj(f.ledger)
	a, aUID := f.record, recordUIDOf(t, f.record)
	b, bUID := f.reseal(t, recordArray(
		mustHash(t, RecordClassifyRange(intObj(100), intObj(150), stringObj("restricted"))),
	))
	d, dUID := f.reseal(t, recordArray(
		mustHash(t, RecordClassifyRange(intObj(250), intObj(50), stringObj("pii"))),
	))
	if !mustHashBoolValue(t, mustHash(t, DiscloseReclassified(ledger, d, a)), "recorded") ||
		!mustHashBoolValue(t, mustHash(t, DiscloseReclassified(ledger, d, b)), "recorded") {
		t.Fatal("a record reclassifying two was not recorded")
	}
	mustRefuse(t, "a consolidated record reclassifying its consolidation", DiscloseReclassified(ledger, b, d),
		"record "+dUID+" already supersedes record "+bUID+" ("+dUID+" reclassifies "+bUID+")")
	mustRefuse(t, "a consolidated record reclassifying its consolidation", DiscloseReclassified(ledger, a, d),
		"record "+dUID+" already supersedes record "+aUID+" ("+dUID+" reclassifies "+aUID+")")
	session, _ := ledgerGet(f.ledger)
	for _, uid := range []string{aUID, bUID} {
		if got, err := redactionReclassifies(session, uid); err != nil || len(got) != 0 {
			t.Fatalf("the refused reclassification was recorded anyway: %s supersedes %v (%v)", uid, got, err)
		}
	}
}
