package analyzer

import (
	"strings"
	"testing"
)

// traversalMessages returns the messages of every pathTraversal diagnostic in
// src. Its one message says "is given a path built from".
func traversalMessages(t *testing.T, src string) []string {
	t.Helper()

	snapshot := New().Analyze(src)
	out := make([]string, 0, 2)
	for _, d := range Diagnostics(snapshot, DefaultLintConfig()) {
		if d.Source != nil && *d.Source == "mutant-lint" && strings.Contains(d.Message, "is given a path built from") {
			out = append(out, d.Message)
		}
	}
	return out
}

func TestPathTraversalFires(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"a filename typed at the terminal",
			`let name = gets();
let body, err = fs_read(name);`,
			"`fs_read` is given a path built from `name`, which was typed at the terminal",
		},
		{
			// The classic shape: a root the program chose, and a leaf it did
			// not.
			"a request argument joined to a root",
			`let leaf, aerr = serve_arg();
let path = "/srv/files/" + leaf;
let body, err = fs_read(path);`,
			"built from `path`, which was taken from a request",
		},
		{
			"a request argument interpolated into a path",
			`let leaf, aerr = serve_arg();
let body, err = fs_read("/srv/files/${leaf}");`,
			"taken from a request",
		},
		{
			// A binding that comes before its source in the file still holds
			// the source's value, so the taint has to be a fixed point rather
			// than a single forward pass.
			"a path assembled above the source that feeds it",
			`let serve = fn(request) {
  let path = "/srv/" + leaf;
  let leaf, aerr = serve_arg();
  let body, err = fs_read(path);
  return body;
};`,
			"taken from a request",
		},
		{
			"a field of a parsed request",
			`let request, err = http_parse_request(raw);
let target = request["path"];
let body, rerr = fs_read("/srv" + target);`,
			"parsed out of an HTTP request",
		},
		{
			"a write rather than a read",
			`let name = gets();
let w, err = fs_write("/var/spool/" + name, data);`,
			"`fs_write` is given a path",
		},
		{
			// Inside an image the traversal is relative to the image root, and
			// the path argument is the second one.
			"a name reaching a file inside an image",
			`let wanted = gets();
let body, err = ntfs_read_file(handle, wanted);`,
			"`ntfs_read_file` is given a path",
		},
		{
			"a downloaded value used as a path",
			`let listing, err = http_get(feed_url);
let body, rerr = fs_read("/cache/" + listing);`,
			"fetched over HTTP",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			messages := traversalMessages(t, tc.src)
			if len(messages) != 1 {
				t.Fatalf("want exactly one report, got %d: %v", len(messages), messages)
			}
			if !strings.Contains(messages[0], tc.want) {
				t.Fatalf("message %q does not contain %q", messages[0], tc.want)
			}
		})
	}
}

func TestPathTraversalStaysQuiet(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{
			"a path the program wrote itself",
			`let body, err = fs_read("/etc/hosts");`,
		},
		{
			"a path from a value with no tracked origin",
			`let body, err = fs_read(configured_path);`,
		},
		{
			// The program looked at the value. The rule cannot read which
			// characters it looked for, so it takes the author's word.
			"a value checked for a traversal segment",
			`let leaf, aerr = serve_arg();
let bad = text_contains(leaf, "..");
if (bad == false) { let body, err = fs_read("/srv/" + leaf); };`,
		},
		{
			"a value confirmed to start with the intended root",
			`let leaf = gets();
let path = "/srv/" + leaf;
if (str_starts_with(path, "/srv/")) { let body, err = fs_read(path); };`,
		},
		{
			"a value with the segment stripped out",
			`let leaf = gets();
let safe = text_replace(leaf, "..", "");
let body, err = fs_read("/srv/" + safe);`,
		},
		{
			"a value matched against a pattern",
			`let leaf = gets();
let ok, err = regex_match("^[a-z0-9]+$", leaf);
let body, rerr = fs_read("/srv/" + leaf);`,
		},
		{
			"a value looked up in an allowed set",
			`let leaf = gets();
let allowed, err = hashset_contains(set, leaf);
let body, rerr = fs_read("/srv/" + leaf);`,
		},
		{
			"an untrusted value that never becomes a path",
			`let answer = gets();
putln("hello ", answer);`,
		},
		{
			// The taint stops at a function boundary, so a value passed to a
			// helper is a value this rule has lost sight of.
			"a value handed to a helper",
			`let read_one = fn(p) { let body, err = fs_read(p); return body; };
let leaf = gets();
let body = read_one("/srv/" + leaf);`,
		},
		{
			"a shadowed source",
			`let gets = fn() { return "fixed.txt"; };
let name = gets();
let body, err = fs_read(name);`,
		},
		{
			"a shadowed sink",
			`let fs_read = fn(p) { return p; };
let name = gets();
let body = fs_read(name);`,
		},
		{
			"a name rebound, so it has no single value",
			`let name = gets();
let name = "fixed.txt";
let body, err = fs_read(name);`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if messages := traversalMessages(t, tc.src); len(messages) != 0 {
				t.Fatalf("want silence, got %v", messages)
			}
		})
	}
}

func TestPathTraversalCanBeTurnedOff(t *testing.T) {
	src := `let name = gets();
let body, err = fs_read(name);`

	config := DefaultLintConfig()
	config.PathTraversal = LintSeverityOff

	snapshot := New().Analyze(src)
	for _, d := range Diagnostics(snapshot, config) {
		if strings.Contains(d.Message, "is given a path built from") {
			t.Fatalf("rule is off but still reported: %s", d.Message)
		}
	}
}

// One sink is one report however deep it sits. The same pairing of
// forEachStatementInScope with walkExpressions that inflated evidenceMutation
// inflated this: a read three blocks down was reported once per enclosing
// statement.
func TestPathTraversalReportsANestedCallOnce(t *testing.T) {
	src := `let name = gets();
if (true) {
  if (true) {
    if (true) {
      let body, err = fs_read("cases/" + name);
    }
  }
}`

	if messages := traversalMessages(t, src); len(messages) != 1 {
		t.Fatalf("want one report, got %d: %v", len(messages), messages)
	}
}

// The doc at the top of path_traversal.go promises that a mention of the name
// "or of any name it was derived from" counts as the check. The code looked at
// one hop, so a path built in two steps was reported with a message saying
// nothing in the file looks at the value -- four lines under the line that
// does.
func TestPathTraversalFollowsAGuardThroughEveryHop(t *testing.T) {
	twoHops := `let name = gets();
if (text_contains(name, "..")) {
  putln("rejected");
}
let file = "cases/" + name;
let full = file + ".log";
let body, err = fs_read(full);`

	if messages := traversalMessages(t, twoHops); len(messages) != 0 {
		t.Fatalf("guard two hops back, want silence, got %v", messages)
	}

	// Still a narrowing and not an amputation: the same two hops with no guard
	// anywhere report, and so does the one-hop case the old code did catch.
	unguarded := `let name = gets();
let file = "cases/" + name;
let full = file + ".log";
let body, err = fs_read(full);`

	if messages := traversalMessages(t, unguarded); len(messages) != 1 {
		t.Fatalf("two hops with no guard, want one report, got %d: %v", len(messages), messages)
	}

	oneHop := `let name = gets();
let file = "cases/" + name;
let body, err = fs_read(file);`

	if messages := traversalMessages(t, oneHop); len(messages) != 1 {
		t.Fatalf("one hop with no guard, want one report, got %d: %v", len(messages), messages)
	}
}
