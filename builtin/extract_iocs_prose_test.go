package builtin

import (
	"slices"
	"testing"

	"mutant/object"
)

// extract_iocs scans free text, so before matching it undoes only the defang
// markers a defanger writes: [.], (dot), [at], hxxp and the like. It used to
// read the bare words " at " and " dot " as "@" and "." too, which made
// ordinary prose into indicators that were never in it.

type extractIOCsProseCase struct {
	text    string
	ipv4    []string
	urls    []string
	emails  []string
	domains []string
}

func checkExtractIOCsProseCases(t *testing.T, cases []extractIOCsProseCase) {
	t.Helper()
	for _, tc := range cases {
		res, ok := ExtractIOCs(stringObj(tc.text)).(*object.Hash)
		if !ok {
			t.Fatalf("extract_iocs(%q) did not return a HASH", tc.text)
		}
		for _, field := range []struct {
			key  string
			want []string
		}{
			{"ipv4", tc.ipv4},
			{"urls", tc.urls},
			{"emails", tc.emails},
			{"domains", tc.domains},
		} {
			got := mustHashStringArray(t, res, field.key)
			if !slices.Equal(got, field.want) {
				t.Errorf("extract_iocs(%q)[%q] = %q, want %q", tc.text, field.key, got, field.want)
			}
		}
	}
}

func TestExtractIOCsProseAtAndDotStayWords(t *testing.T) {
	checkExtractIOCsProseCases(t, []extractIOCsProseCase{
		{
			// Was emails [in@example.com] and domains [example.com, red.on].
			text:    "The operator logged in at example.com and clicked the red dot on the map.",
			domains: []string{"example.com"},
		},
		{
			// Was emails [beacons@evil.com].
			text:    "The implant beacons at evil.com every hour.",
			domains: []string{"evil.com"},
		},
		{
			// Was urls [http://example.com/login@09:00]: the URL that is in
			// the text was lost, and one that is not was reported.
			text:    "Download from http://example.com/login at 09:00 and the red dot marks it.",
			urls:    []string{"http://example.com/login"},
			domains: []string{"example.com"},
		},
	})
}

func TestExtractIOCsProseFixStillUndoesDefangMarkers(t *testing.T) {
	checkExtractIOCsProseCases(t, []extractIOCsProseCase{
		{
			text:    "Contact admin[at]evil[.]example[.]com via hxxp://bad(dot)example[dot]net/x",
			urls:    []string{"http://bad.example.net/x"},
			emails:  []string{"admin@evil.example.com"},
			domains: []string{"bad.example.net", "evil.example.com"},
		},
		{
			text:    "ops(at)corp[.]example and root[@]host{.}example[.]org, see hxxps[://]c2[.]example[/]gate",
			urls:    []string{"https://c2.example/gate"},
			emails:  []string{"ops@corp.example", "root@host.example.org"},
			domains: []string{"c2.example", "corp.example", "host.example.org"},
		},
		{
			text: "beacon hXXp://203.0.113(.)7[:]8080/c",
			ipv4: []string{"203.0.113.7"},
			urls: []string{"http://203.0.113.7:8080/c"},
		},
	})
}

// refang is handed one indicator rather than prose, so it still reads the
// bare words; giving extract_iocs its own replacer must not change refang.
func TestRefangStillReadsBareAtAndDotInOneIndicator(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"admin at evil dot com", "admin@evil.com"},
		{"hxxps://bad(dot)site[dot]net", "https://bad.site.net"},
		{"ops(at)corp[.]example[/]x", "ops@corp.example/x"},
	} {
		if got := strResult(t, Refang(stringObj(tc.in))); got != tc.want {
			t.Errorf("refang(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
