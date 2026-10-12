//go:build linux
// +build linux

package security

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// M26-TMP-013. One unreadable cgroup file used to end the Linux detection:
// detectSandboxLinux returned an empty detection with the read error, throwing
// away every indicator already found and skipping every check after the cgroup
// loop. Where /proc is mounted hidepid=2, /proc/1 is invisible to an
// unprivileged process, so that is an ordinary hardened host, not a fault.
//
// The cgroup files are the seam: a readable one planted with a Kubernetes
// marker, and one that does not exist. Whichever order they come in, the
// answer must be the one the readable file alone gives -- the same confidence
// and the same indicators, from the checks before the loop, the loop and the
// checks after it -- and the error beside it must name the file it could not
// read. Comparing against that answer, rather than a list of expected
// indicators, keeps the test independent of what this host itself looks like.
func TestLinuxSandboxDetectionSurvivesAnUnreadableCgroupFile(t *testing.T) {
	dir := t.TempDir()
	marked := filepath.Join(dir, "cgroup")
	if err := os.WriteFile(marked, []byte("12:pids:/kubepods/besteffort/pod1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(dir, "unreadable-cgroup")
	alsoMissing := filepath.Join(dir, "unreadable-cgroup-2")

	saved := linuxCgroupPaths
	t.Cleanup(func() { linuxCgroupPaths = saved })

	linuxCgroupPaths = []string{marked}
	want, err := detectSandboxLinux()
	if err != nil {
		t.Fatalf("detection with every cgroup file readable returned an error: %v", err)
	}
	if !slices.Contains(want.Indicators, linuxIndicatorKubernetesCgroup) || want.Confidence < linuxConfidenceKubernetesCgrp {
		t.Fatalf("the planted cgroup marker did not register even when readable: confidence %d, indicators %v",
			want.Confidence, want.Indicators)
	}

	for _, tc := range []struct {
		name  string
		paths []string
	}{
		{"an indicator found before the unreadable file is kept", []string{marked, missing}},
		{"the files after the unreadable one are still read", []string{missing, marked}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			linuxCgroupPaths = tc.paths
			got, err := detectSandboxLinux()
			if got.Type == "" || got.Confidence != want.Confidence || !slices.Equal(got.Indicators, want.Indicators) {
				t.Errorf("an unreadable cgroup file changed the answer:\n got type %q, confidence %d, indicators %v\n"+
					"want confidence %d, indicators %v (type %q)",
					got.Type, got.Confidence, got.Indicators, want.Confidence, want.Indicators, want.Type)
			}
			if err == nil || !strings.Contains(err.Error(), missing) {
				t.Errorf("got error %v; want one naming the unread file %s", err, missing)
			}
		})
	}

	// With no cgroup file readable the answer is the one the other checks give
	// with no cgroup files at all, and the error names both.
	linuxCgroupPaths = nil
	wantNone, err := detectSandboxLinux()
	if err != nil {
		t.Fatalf("detection with no cgroup files to read returned an error: %v", err)
	}
	linuxCgroupPaths = []string{missing, alsoMissing}
	got, err := detectSandboxLinux()
	if got.Confidence != wantNone.Confidence || !slices.Equal(got.Indicators, wantNone.Indicators) {
		t.Errorf("two unreadable cgroup files changed the other checks' answer:\n got confidence %d, indicators %v\n"+
			"want confidence %d, indicators %v", got.Confidence, got.Indicators, wantNone.Confidence, wantNone.Indicators)
	}
	if err == nil || !strings.Contains(err.Error(), missing) || !strings.Contains(err.Error(), alsoMissing) {
		t.Errorf("got error %v; want one naming both unread files", err)
	}
}
