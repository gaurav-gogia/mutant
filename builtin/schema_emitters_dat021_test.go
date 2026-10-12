package builtin

import (
	"testing"

	"mutant/object"
)

// OCSF 1001 File System Activity defines activity 4 as Delete. A deletion
// filed under 99 Other reads to a consumer as "something the schema has no
// name for", so a detection keyed on type_uid 100104 would never see it.
func TestOcsfDat021FileDeletionIsActivityDelete(t *testing.T) {
	document := timelineReport().toHash("handle-1")
	payload, errObj := unwrapPair(t, EventsFrom(document, stringObj("fs_report")))
	if errObj != nil {
		t.Fatalf("events_from returned error: %s", errObj.Inspect())
	}

	var deletion *object.Hash
	for _, element := range payload.(*object.Array).Elements {
		event := element.(*object.Hash)
		if mustHashStringValue(t, event, "ts_desc") == "Deletion Time" {
			deletion = event
		}
	}
	if deletion == nil {
		t.Fatal("the report produced no Deletion Time event")
	}

	event := emitted(t, OcsfEvent, deletion)
	if got := docInt(t, event, "class_uid"); got != 1001 {
		t.Fatalf("class_uid = %d, want 1001", got)
	}
	if got := docInt(t, event, "activity_id"); got != 4 {
		t.Errorf("activity_id = %d, want 4 Delete", got)
	}
	if got := docStr(t, event, "activity_name"); got != "Delete" {
		t.Errorf("activity_name = %q, want \"Delete\"", got)
	}
	if got := docInt(t, event, "type_uid"); got != 100104 {
		t.Errorf("type_uid = %d, want 100104", got)
	}
	if got := docStr(t, event, "type_name"); got != "File System Activity: Delete" {
		t.Errorf("type_name = %q", got)
	}
}

// When the timestamp description names no activity, the delete action
// decides: an envelope event that says it is a deletion is still a deletion.
// Outside class 1001 the action keeps its 99 Other reading.
func TestOcsfDat021DeleteActionWithoutDeletionTime(t *testing.T) {
	file := ocsfClasses["file"]
	if id, name := ocsfActivity(file, "delete", "Recorded Time"); id != 4 || name != "Delete" {
		t.Errorf("file + delete action -> (%d, %q), want (4, \"Delete\")", id, name)
	}
	if id, name := ocsfActivity(file, "delete", ""); id != 4 || name != "Delete" {
		t.Errorf("file + delete action, no description -> (%d, %q), want (4, \"Delete\")", id, name)
	}
	if id, name := ocsfActivity(file, "", "Deletion Time ($FILE_NAME)"); id != 4 || name != "Delete" {
		t.Errorf("file + Deletion Time ($FILE_NAME) -> (%d, %q), want (4, \"Delete\")", id, name)
	}
	if id, name := ocsfActivity(ocsfClasses["web"], "delete", ""); id != 99 || name != "delete" {
		t.Errorf("web + delete action -> (%d, %q), want (99, \"delete\")", id, name)
	}
}

// The description is read before the action, so a timestamp that names an
// activity keeps it even when the event's action is delete. This pins what
// ocsfActivity's doc comment and docs/INTERCHANGE_SCHEMAS.md say: the action
// decides only when the description names no activity.
func TestOcsfDat021TimestampActivityBeatsDeleteAction(t *testing.T) {
	file := ocsfClasses["file"]
	cases := []struct {
		description string
		id          int64
		name        string
	}{
		{"Creation Time", 1, "Create"},
		{"Content Modification Time", 3, "Update"},
		{"Last Access Time", 2, "Read"},
		{"Metadata Modification Time", 6, "Set Attributes"},
		{"Written Time", 6, "Set Attributes"},
		{"Deletion Time", 4, "Delete"},
	}
	for _, c := range cases {
		if id, name := ocsfActivity(file, "delete", c.description); id != c.id || name != c.name {
			t.Errorf("file + delete action + %q -> (%d, %q), want (%d, %q)", c.description, id, name, c.id, c.name)
		}
	}
}
