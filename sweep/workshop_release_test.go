package sweep

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"mutant/builtin"
)

// The QUILLDROP workshop targets the released version, not this branch.
//
// An attendee downloads v2.6.0 and runs the programs in examples/workshop. Once
// this branch moves on it registers builtins the release does not, and every
// one of the extras is a builtin the workshop must not call -- a single new
// builtin in a teaching example turns "run this on the release" into a runtime
// error twenty people hit at once.
//
// Nothing else in the tree checks this. The constraint lives in prose in the
// workshop README, and prose does not fail. So the list below is the check: it
// names every builtin the workshop is allowed to call, each one confirmed
// present at v2.6.0 when the workshop was written. Adding a builtin to a
// workshop program means adding it here, which is the moment to go and look at
// whether the release has it.
//
// The list is a frozen historical fact rather than a mirror of anything, so it
// cannot drift the way a hand-maintained list of file names would. It is wrong
// only if someone adds a name without checking, and adding a name is the whole
// prompt to check.
var workshopBuiltinsAtV260 = []string{
	"base64_decode", "base64_encode", "bin_pe_parse", "bodyfile_parse",
	"browser_cookies", "browser_history", "case_bundle", "case_close",
	"case_evidence", "case_manifest_verify", "case_note", "case_open",
	"cidr_hosts", "defang", "domain_extract", "each", "email_parse",
	"events_from", "extract_iocs", "filter", "fs_delete", "fs_entropy",
	"fs_exists", "fs_hash", "fs_magic", "fs_read", "fs_walk", "get", "has_key",
	"hex_decode", "hex_encode", "hive_close", "hive_key_info",
	"hive_list_keys", "hive_list_values", "hive_open", "imphash", "ip_in_cidr",
	"ip_is_private", "is_null", "jwt_decode", "keys", "len", "mactime",
	"mft_parse", "ntfs_close", "ntfs_extract_file", "ntfs_open", "putln",
	"regex_capture_groups", "regex_find_all", "regex_match", "set",
	"sigma_scan", "slice", "sort_by", "sqlite_query", "str_format",
	"str_starts_with", "str_trim_prefix", "table_close",
	"table_list_partitions", "table_open", "text_jaro_winkler",
	"text_levenshtein", "timestamp_normalize", "tld_extract",
	"tls_generate_ca", "to_string", "x509_parse", "zip_close", "zip_entries",
	"zip_open",
}

func TestTheWorkshopOnlyCallsBuiltinsTheReleaseHas(t *testing.T) {
	allowed := map[string]bool{}
	for _, name := range workshopBuiltinsAtV260 {
		allowed[name] = true
	}

	registered := map[string]bool{}
	for _, definition := range builtin.Builtins {
		registered[definition.Name] = true
	}

	workshop := filepath.Join(repositoryRoot, "examples", "workshop")
	programs := 0
	unexpected := map[string][]string{}

	for _, path := range mutantPrograms(t) {
		if !strings.HasPrefix(filepath.ToSlash(path), filepath.ToSlash(workshop)+"/") {
			continue
		}
		programs++

		source, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("%s: %s", path, err)
		}
		for name := range calledNames(string(source)) {
			// A name that is not registered at all is a user function or a
			// module call, which this test has nothing to say about.
			if !registered[name] || allowed[name] {
				continue
			}
			rel := filepath.ToSlash(strings.TrimPrefix(path, repositoryRoot+string(filepath.Separator)))
			unexpected[name] = append(unexpected[name], rel)
		}
	}

	if programs == 0 {
		t.Fatal("found no workshop programs; the walk is looking in the wrong place")
	}

	names := make([]string, 0, len(unexpected))
	for name := range unexpected {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		files := unexpected[name]
		sort.Strings(files)
		t.Errorf("the workshop calls %q, which is not on the list of builtins v2.6.0 has: %s"+
			"\n\tconfirm the release registers it (`git show v2.6.0:builtin/names.go`), then add it to"+
			"\n\tworkshopBuiltinsAtV260 -- or use a builtin the release does have",
			name, strings.Join(files, ", "))
	}
}
