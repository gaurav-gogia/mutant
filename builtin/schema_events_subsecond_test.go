package builtin

import (
	"testing"
	"time"

	"mutant/object"
)

// M26-DAT-020: events_from built every event from normalizeTimestamp's whole
// seconds and took a fraction only from a companion ns field, so any timestamp
// that carries its fraction in the value itself -- an fs_report ISO string, a
// unix_ms, FILETIME or WebKit integer -- reached ts_ms, iso and the emitters
// truncated to the second.

// subsecondEventOf normalizes a single-entry artifact and returns its one event.
func subsecondEventOf(t *testing.T, artifact, kind object.Object) *object.Hash {
	t.Helper()
	events := eventsOf(t, artifact, kind)
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
	return events[0]
}

func subsecondMapping(format string) object.Object {
	return makeHashObject(map[string]object.Object{
		"times": &object.Array{Elements: []object.Object{makeHashObject(map[string]object.Object{
			"field": stringObj("when"), "format": stringObj(format),
		})}},
	})
}

func subsecondRow(value object.Object) object.Object {
	return &object.Array{Elements: []object.Object{makeHashObject(map[string]object.Object{"when": value})}}
}

func TestEventsFromSubsecondFsReportKeepsTheISOFraction(t *testing.T) {
	// The value is what the *_report family itself writes: formatTime's
	// RFC3339Nano rendering of the filesystem's time.
	created := time.Date(2024, 1, 2, 3, 4, 5, 678901234, time.UTC)
	row := makeHashObject(map[string]object.Object{
		"path": stringObj("/a"), "name": stringObj("a"), "filesystem": stringObj("ext4"),
		"created": stringObj(formatTime(created)),
	})
	artifact := makeHashObject(map[string]object.Object{"files": &object.Array{Elements: []object.Object{row}}})
	event := subsecondEventOf(t, artifact, stringObj("fs_report"))

	if got := hInt(t, event, "ts"); got != 1704164645 {
		t.Errorf("ts = %d, want 1704164645", got)
	}
	if got := hInt(t, event, "ts_ms"); got != 1704164645678 {
		t.Errorf("ts_ms = %d, want 1704164645678", got)
	}
	if got := hStr(t, event, "iso"); got != "2024-01-02T03:04:05.678901234Z" {
		t.Errorf("iso = %q, want 2024-01-02T03:04:05.678901234Z", got)
	}

	if got := docInt(t, emitted(t, OcsfEvent, event), "time"); got != 1704164645678 {
		t.Errorf("OCSF time = %d, want 1704164645678", got)
	}
	if got := docInt(t, emitted(t, TimesketchEvent, event), "timestamp"); got != 1704164645678000 {
		t.Errorf("Timesketch timestamp = %d, want 1704164645678000", got)
	}
	if got := docStr(t, emitted(t, EcsEvent, event), "@timestamp"); got != "2024-01-02T03:04:05.678901234Z" {
		t.Errorf("ECS @timestamp = %q, want 2024-01-02T03:04:05.678901234Z", got)
	}
}

func TestEventsFromSubsecondNumericFormatsKeepTheirFraction(t *testing.T) {
	for _, tc := range []struct {
		format string
		value  int64
		tsMs   int64
		iso    string
	}{
		{"unix_ms", 1704164645678, 1704164645678, "2024-01-02T03:04:05.678Z"},
		{"unix_us", 1704164645678901, 1704164645678, "2024-01-02T03:04:05.678901Z"},
		{"unix_ns", 1704164645678901234, 1704164645678, "2024-01-02T03:04:05.678901234Z"},
		// 2024-01-02T03:04:05.6789012Z in 100 ns ticks since 1601.
		{"filetime", (1704164645+filetimeEpochDeltaSec)*10_000_000 + 6789012, 1704164645678, "2024-01-02T03:04:05.6789012Z"},
		{"webkit", (1704164645+filetimeEpochDeltaSec)*1_000_000 + 678901, 1704164645678, "2024-01-02T03:04:05.678901Z"},
		// auto reads this magnitude as milliseconds.
		{"auto", 1704164645678, 1704164645678, "2024-01-02T03:04:05.678Z"},
		// Whole seconds have no fraction to keep.
		{"unix", 1704164645, 1704164645000, "2024-01-02T03:04:05Z"},
	} {
		event := subsecondEventOf(t, subsecondRow(intObj(tc.value)), subsecondMapping(tc.format))
		if got := hInt(t, event, "ts"); got != 1704164645 {
			t.Errorf("%s: ts = %d, want 1704164645", tc.format, got)
		}
		if got := hInt(t, event, "ts_ms"); got != tc.tsMs {
			t.Errorf("%s: ts_ms = %d, want %d", tc.format, got, tc.tsMs)
		}
		if got := hStr(t, event, "iso"); got != tc.iso {
			t.Errorf("%s: iso = %q, want %q", tc.format, got, tc.iso)
		}
	}
}

func TestEventsFromSubsecondArrayTimestampsKeepTheirFraction(t *testing.T) {
	// prefetch's run times are an array of ISO strings.
	artifact := makeHashObject(map[string]object.Object{
		"executable": stringObj("X.EXE"), "run_count": intObj(1),
		"run_times": &object.Array{Elements: []object.Object{stringObj("2024-01-02T03:04:05.5Z")}},
	})
	event := subsecondEventOf(t, artifact, stringObj("prefetch"))
	if got := hInt(t, event, "ts_ms"); got != 1704164645500 {
		t.Errorf("ts_ms = %d, want 1704164645500", got)
	}
	if got := hStr(t, event, "iso"); got != "2024-01-02T03:04:05.5Z" {
		t.Errorf("iso = %q, want 2024-01-02T03:04:05.5Z", got)
	}
}

func TestEventsFromSubsecondCompanionFieldStillDecides(t *testing.T) {
	mapping := makeHashObject(map[string]object.Object{
		"times": &object.Array{Elements: []object.Object{makeHashObject(map[string]object.Object{
			"field": stringObj("when"), "format": stringObj("unix_ms"), "ns_field": stringObj("when_ns"),
		})}},
	})

	// A companion the entry carries is the artifact's own word on the fraction.
	carried := &object.Array{Elements: []object.Object{makeHashObject(map[string]object.Object{
		"when": intObj(1704164645678), "when_ns": intObj(123456789),
	})}}
	if got := hStr(t, subsecondEventOf(t, carried, mapping), "iso"); got != "2024-01-02T03:04:05.123456789Z" {
		t.Errorf("with companion: iso = %q, want 2024-01-02T03:04:05.123456789Z", got)
	}

	// A companion the spec names but the entry lacks leaves the value's own
	// fraction in place rather than zeroing it.
	missing := subsecondRow(intObj(1704164645678))
	if got := hStr(t, subsecondEventOf(t, missing, mapping), "iso"); got != "2024-01-02T03:04:05.678Z" {
		t.Errorf("without companion: iso = %q, want 2024-01-02T03:04:05.678Z", got)
	}
}
