package builtin

import (
	"bufio"
	"net/mail"
	"slices"
	"strings"
	"testing"

	"mutant/object"
)

// emailURLsLongLineMessage returns a message whose Content-Type header is
// contentType and whose body is body, as written: with a multipart type, body
// carries the parts and their boundary lines itself.
func emailURLsLongLineMessage(contentType, body string) string {
	return "From: a@example.com\r\n" +
		"To: b@example.com\r\n" +
		"Subject: long line\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: " + contentType + "\r\n" +
		"\r\n" +
		body
}

// emailURLsLongLineGot runs email_urls on raw and returns the url field of every
// element, in the order email_urls returned them.
func emailURLsLongLineGot(t *testing.T, raw string) []string {
	t.Helper()
	result, errObj := unwrapPair(t, EmailURLs(stringObj(raw)))
	if errObj != nil {
		t.Fatalf("email_urls: %s", errObj.Inspect())
	}
	arr, ok := result.(*object.Array)
	if !ok {
		t.Fatalf("email_urls returned %T, want an ARRAY", result)
	}
	got := make([]string, 0, len(arr.Elements))
	for _, e := range arr.Elements {
		got = append(got, efMustHashString(t, efMustHash(t, e), "url"))
	}
	return got
}

// TestEmailURLsKeepsEveryURLPastALineLongerThan64KiB is M26-ART-014's
// regression test. The body was read through a bufio.Scanner with its default
// 64 KiB buffer limit and Err() was never checked, so the first line of 64 KiB
// or more (counting a CR before the line feed) -- an inline data: image,
// minified HTML -- ended the scan: email_urls returned no error and none of the
// URLs on that line or after it.
func TestEmailURLsKeepsEveryURLPastALineLongerThan64KiB(t *testing.T) {
	// 70,000 bytes, longer than bufio.MaxScanTokenSize (64 KiB).
	pad := strings.Repeat("A", 70000)
	image := `<img src="data:image/png;base64,` + pad + `">`

	check := func(t *testing.T, raw string, want ...string) {
		t.Helper()
		if got := emailURLsLongLineGot(t, raw); !slices.Equal(got, want) {
			t.Fatalf("email_urls = %q, want %q", got, want)
		}
	}

	t.Run("the long line comes first", func(t *testing.T) {
		body := image + "<a href=https://first.example/x>first</a>\r\n" +
			"<a href=https://second.example/y>second</a>\r\n"
		check(t, emailURLsLongLineMessage("text/html", body),
			"https://first.example/x", "https://second.example/y")
	})

	t.Run("URLs before, on and after the long line", func(t *testing.T) {
		body := "<a href=https://before.example/a>before</a>\r\n" +
			image + "<a href=https://onlong.example/b>on</a>\r\n" +
			"<a href=https://after.example/c>after</a>\r\n"
		check(t, emailURLsLongLineMessage("text/html", body),
			"https://after.example/c", "https://before.example/a", "https://onlong.example/b")
	})

	// The text and HTML bodies are matched as one string, text first, so a long
	// line in the text/plain part used to hide every link in the text/html part.
	t.Run("a long text line and the links of the HTML part", func(t *testing.T) {
		body := "--AltLong\r\n" +
			"Content-Type: text/plain; charset=UTF-8\r\n" +
			"\r\n" +
			"Open https://text.example/t\r\n" +
			pad + "\r\n" +
			"--AltLong\r\n" +
			"Content-Type: text/html; charset=UTF-8\r\n" +
			"\r\n" +
			"<a href=\"https://html.example/z\">open</a>\r\n" +
			"--AltLong--\r\n"
		check(t, emailURLsLongLineMessage(`multipart/alternative; boundary="AltLong"`, body),
			"https://html.example/z", "https://text.example/t")
	})
}

// TestEmailURLsWholeBodyMatchFindsWhatEachLineFinds checks that matching the
// whole body changes no answer for mail without long lines: a URL match cannot
// cross a line break, so the URLs of a body are the URLs of its lines taken one
// at a time, whichever line endings the mail uses.
func TestEmailURLsWholeBodyMatchFindsWhatEachLineFinds(t *testing.T) {
	cases := []struct {
		body string
		want []string
	}{
		{"", []string{}},
		{"\r\n\r\n\n", []string{}},
		{"see https://a.example/x, and http://b.example/y.\r\nthen (https://c.example/z)\r\n",
			[]string{"http://b.example/y", "https://a.example/x", "https://c.example/z"}},
		{"lf only https://d.example/1\nand https://d.example/2\n",
			[]string{"https://d.example/1", "https://d.example/2"}},
		{"tab\thttps://e.example/3\tafter\fhttps://e.example/4\f",
			[]string{"https://e.example/3", "https://e.example/4"}},
		{"lone carriage returns\rhttps://f.example/5\rhttps://f.example/6\r",
			[]string{"https://f.example/5", "https://f.example/6"}},
		{"no final newline https://g.example/7",
			[]string{"https://g.example/7"}},
		{"<a href=\"https://h.example/8\">x</a><a href=https://h.example/9>y</a>\r\n",
			[]string{"https://h.example/8", "https://h.example/9"}},
	}
	for _, tc := range cases {
		got := collectURLs(mail.Header{}, tc.body)
		if !slices.Equal(got, tc.want) {
			t.Errorf("collectURLs(%q) = %q, want %q", tc.body, got, tc.want)
		}

		perLine := map[string]bool{}
		scanner := bufio.NewScanner(strings.NewReader(tc.body))
		for scanner.Scan() {
			for _, u := range collectURLs(mail.Header{}, scanner.Text()) {
				perLine[u] = true
			}
		}
		if err := scanner.Err(); err != nil {
			t.Fatalf("splitting %q into lines: %v", tc.body, err)
		}
		if len(perLine) != len(got) {
			t.Errorf("%q: line by line found %d URLs, the whole body %q", tc.body, len(perLine), got)
			continue
		}
		for _, u := range got {
			if !perLine[u] {
				t.Errorf("%q: the whole body found %q, no single line did", tc.body, u)
			}
		}
	}
}
