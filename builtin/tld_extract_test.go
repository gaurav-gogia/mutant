package builtin

import (
	"testing"

	"mutant/object"
)

// tld_extract answers with a hash for every name, and an etld1 only where the
// name has one. It returned an empty string for a cookie's host (".example.com",
// which names the same site as "example.com"), for a fully qualified name with
// its trailing dot, for a bare public suffix, for localhost and for an IP
// address, so the caller's ["etld1"] failed on a string.
func TestTldExtractAnswersEveryNameWithAHash(t *testing.T) {
	for _, c := range []struct{ in, domain, etld1, suffix string }{
		{"sub.example.co.uk", "sub.example.co.uk", "example.co.uk", "co.uk"},
		{".adserve-metrics.example", "adserve-metrics.example", "adserve-metrics.example", "example"},
		{".news.bbc.co.uk", "news.bbc.co.uk", "bbc.co.uk", "co.uk"},
		{"Example.COM.", "example.com", "example.com", "com"},
		{"co.uk", "co.uk", "", "co.uk"},
		{"localhost", "localhost", "", "localhost"},
		{"192.168.1.1", "192.168.1.1", "", ""},
		{"::1", "::1", "", ""},
		{"", "", "", ""},
	} {
		h, ok := TLDExtract(stringObj(c.in)).(*object.Hash)
		if !ok {
			t.Errorf("tld_extract(%q) is not a hash", c.in)
			continue
		}
		for _, f := range []struct{ key, want string }{
			{"domain", c.domain}, {"etld1", c.etld1}, {"suffix", c.suffix},
		} {
			if got := hStr(t, h, f.key); got != f.want {
				t.Errorf("tld_extract(%q)[%q] = %q, want %q", c.in, f.key, got, f.want)
			}
		}
	}
}
