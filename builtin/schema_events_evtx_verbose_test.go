package builtin

import (
	"testing"

	"mutant/object"
)

// Windows event Levels run 1 Critical, 2 Error, 3 Warning, 4 Information,
// 5 Verbose. Verbose is the least severe of them, so no emitter may rank a
// Level 5 record above a Level 4 one.

func evtxVerboseLevelEvent(t *testing.T, level int64) *object.Hash {
	t.Helper()
	// Hand-built: an .evtx needs a chunked BinXML file, and what is checked here
	// is the level-to-severity mapping.
	record := makeHashObject(map[string]object.Object{
		"timestamp": intObj(1700000000), "event_id": intObj(7036), "level": intObj(level),
		"provider": stringObj("Service Control Manager"),
		"computer": stringObj("WS01"), "channel": stringObj("System"),
	})
	events := eventsOf(t, makeHashObject(map[string]object.Object{
		"records": &object.Array{Elements: []object.Object{record}},
	}), stringObj("evtx"))
	if len(events) != 1 {
		t.Fatalf("Level %d: expected 1 event, got %d", level, len(events))
	}
	return events[0]
}

func TestEvtxVerboseLevelNeverOutranksInformation(t *testing.T) {
	information := evtxVerboseLevelEvent(t, 4)
	verbose := evtxVerboseLevelEvent(t, 5)

	if got := hStr(t, verbose, "severity"); got != "informational" {
		t.Errorf("Level 5 (Verbose) severity = %q, want \"informational\"", got)
	}

	// OCSF severity_id counts up with severity.
	infoID := docInt(t, emitted(t, OcsfEvent, information), "severity_id")
	verboseID := docInt(t, emitted(t, OcsfEvent, verbose), "severity_id")
	if verboseID > infoID {
		t.Errorf("OCSF severity_id: Level 5 (Verbose) = %d ranks above Level 4 (Information) = %d", verboseID, infoID)
	}

	// ECS event.severity runs the syslog way: a larger number is less severe.
	infoCode := docInt(t, emitted(t, EcsEvent, information), "event.severity")
	verboseCode := docInt(t, emitted(t, EcsEvent, verbose), "event.severity")
	if verboseCode < infoCode {
		t.Errorf("ECS event.severity: Level 5 (Verbose) = %d ranks above Level 4 (Information) = %d", verboseCode, infoCode)
	}
}

func TestEvtxVerboseLevelSeverityFollowsTheLevelOrder(t *testing.T) {
	// Each Level from 1 to 5 is no more severe than the one before it, under
	// both emitters' numbers.
	prevOCSF, prevECS := int64(-1), int64(-1)
	for level := int64(1); level <= 5; level++ {
		event := evtxVerboseLevelEvent(t, level)
		ocsfID := docInt(t, emitted(t, OcsfEvent, event), "severity_id")
		ecsCode := docInt(t, emitted(t, EcsEvent, event), "event.severity")
		if level > 1 {
			if ocsfID > prevOCSF {
				t.Errorf("OCSF severity_id rises from %d to %d at Level %d", prevOCSF, ocsfID, level)
			}
			if ecsCode < prevECS {
				t.Errorf("ECS event.severity falls from %d to %d at Level %d", prevECS, ecsCode, level)
			}
		}
		prevOCSF, prevECS = ocsfID, ecsCode
	}
}
