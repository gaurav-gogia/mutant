package server

import (
	"net/url"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	lsp "github.com/tliron/glsp/protocol_3_16"
)

// A file URI's authority is part of the file's name. VS Code spells a document
// on a network share \\alpha\share\lib.mut as file://alpha/share/lib.mut, and
// uriToPath dropped the host: two hosts serving one share path became one file
// to every call site, and a host-relative path that filepath.Abs then resolved
// onto the server's own drive (M26-LSP-031). Every input here is a string, so
// no share has to exist.

func TestAFileURIKeepsItsAuthority(t *testing.T) {
	alpha, okA := uriToPath("file://alpha/share/lib.mut")
	beta, okB := uriToPath("file://beta/share/lib.mut")
	if !okA || !okB {
		t.Fatalf("a file URI with a host was refused: ok=%v,%v", okA, okB)
	}
	if canonicalPath(alpha) == canonicalPath(beta) {
		t.Errorf("two hosts serving one share path are one file: %q", canonicalPath(alpha))
	}

	want := "//alpha/share/lib.mut" // VS Code's fsPath for this URI
	if runtime.GOOS == "windows" {
		want = `\\alpha\share\lib.mut` // the UNC path
	}
	if alpha != want {
		t.Errorf("file://alpha/share/lib.mut -> %q, want %q", alpha, want)
	}
	if runtime.GOOS == "windows" {
		if c := canonicalPath(alpha); len(c) >= 2 && c[1] == ':' {
			t.Errorf("canonicalPath put a file on host alpha onto a local drive: %q", c)
		}
	}
}

// TestALocalFileURIMapsAsBefore is the guard: the authority is not a host in
// these spellings, and what they map to must not move.
func TestALocalFileURIMapsAsBefore(t *testing.T) {
	type want struct{ windows, other string }
	for uri, w := range map[lsp.DocumentUri]want{
		"file:///C:/x/a.mut":          {`C:\x\a.mut`, "C:/x/a.mut"},
		"file:///c%3A/x/a.mut":        {`c:\x\a.mut`, "c:/x/a.mut"},
		"file:///home/u/a.mut":        {`\home\u\a.mut`, "/home/u/a.mut"},
		"file://localhost/C:/x/a.mut": {`C:\x\a.mut`, "C:/x/a.mut"},
	} {
		got, ok := uriToPath(uri)
		expected := w.other
		if runtime.GOOS == "windows" {
			expected = w.windows
		}
		if !ok || got != expected {
			t.Errorf("uriToPath(%s) = %q ok=%v, want %q", uri, got, ok, expected)
		}
	}
}

// TestAnAuthorityThatNamesNoShareIsRefused: a user, a port, an IPv6 literal, a
// bare host, a drive on another machine, and every device-namespace name that
// is not a path on a drive are refused rather than spliced into a path. The
// user part is how VS Code spells the WebDAV share \\alpha@SSL\DavWWWRoot, which
// no reading of the URI rebuilds with certainty. file://./c%3A is \\.\c:, the
// volume rather than its root directory, and a ".." under "." climbs out of the
// drive, because the device namespace's root is \\.\ itself. file://%3F/... is
// VS Code's spelling of \\?\, and url.Parse rejects the escape.
func TestAnAuthorityThatNamesNoShareIsRefused(t *testing.T) {
	for _, uri := range []lsp.DocumentUri{
		"file://user@alpha/share/x.mut",
		"file://alpha:445/share/x.mut",
		"file://[::1]/share/x.mut",
		"file://alpha",
		"file://alpha/",
		"file://alpha/C:/x.mut",
		"file://./PhysicalDrive0",
		"file://alpha@ssl/DavWWWRoot/x.mut",
		"file://./c%3A",
		"file://./c%3A/x/../../PhysicalDrive0",
		"file://./UNC/alpha/share/x.mut",
		"file://%3F/c%3A/x/a.mut",
	} {
		if got, ok := uriToPath(uri); ok {
			t.Errorf("uriToPath(%s) = %q, want a refusal", uri, got)
		}
	}
}

// TestAUNCPathBecomesAnAuthorityURI: pathToURI spelled \\alpha\share\lib.mut as
// file:////alpha/share/lib.mut, which VS Code's URI.parse refuses ("If a URI
// does not contain an authority component, then the path cannot begin with two
// slash characters"). It is reachable once uriToPath keeps the host, because the
// workspace roots, and so the scan and the links, then land on the share.
func TestAUNCPathBecomesAnAuthorityURI(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("UNC is a Windows spelling")
	}
	unc := `\\alpha\share\lib.mut`
	uri := pathToURI(unc)
	if uri != "file://alpha/share/lib.mut" {
		t.Errorf("pathToURI(%q) = %q, want file://alpha/share/lib.mut", unc, uri)
	}
	back, ok := uriToPath(uri)
	if !ok || canonicalPath(back) != canonicalPath(unc) {
		t.Errorf("round trip %q -> %q -> %q (ok=%v)", unc, uri, back, ok)
	}
}

// TestDocumentsOnTwoHostsAreTwoModules: the module key is what sema files a
// document's declarations under and resolves its imports against.
func TestDocumentsOnTwoHostsAreTwoModules(t *testing.T) {
	s := New(false)
	a := s.analyzeDoc("file://alpha/share/main.mut", "let a = 1;\n")
	b := s.analyzeDoc("file://beta/share/main.mut", "let b = 2;\n")
	if a.ModuleKey == "" || a.ModuleKey == b.ModuleKey {
		t.Errorf("documents on two hosts share the module key %q", a.ModuleKey)
	}
}

// TestADevicePathOnADriveReadsBackAsThatPath: VS Code spells \\.\o:\x, the
// device-namespace name of o:\x, as file://./o%3A/x, and its fsPath reads that
// back as \\.\o:\x. Refused, it cost a workspace opened that way its module
// keys, its links and its cross-file definitions (M26-LSP-031). Off Windows
// there is no device namespace, and the URI stays refused.
func TestADevicePathOnADriveReadsBackAsThatPath(t *testing.T) {
	const uri lsp.DocumentUri = "file://./o%3A/x"
	got, ok := uriToPath(uri)
	if runtime.GOOS != "windows" {
		if ok {
			t.Errorf("uriToPath(%s) = %q off Windows, want a refusal", uri, got)
		}
		return
	}
	if want := filepath.FromSlash("//./o:/x"); !ok || got != want {
		t.Errorf("uriToPath(%s) = %q ok=%v, want %q", uri, got, ok, want)
	}
}

// TestADevicePathStaysOnItsDrive: the device namespace's root is \\.\ rather
// than the drive, so a ".." under file://./c%3A/ climbs off the drive, to
// \\.\PhysicalDrive0, a disk, or to another drive than the URI names. Windows
// reads a backslash as a separator as well as a slash, and the path arrives
// percent-decoded, so a %5C ends a segment just as "/" does: with segments
// split on "/" alone, file://./z%3A/a%5C..%5C..%5CC:/... read a file on C:
// (M26-LSP-031). The first URI is the control. Every row below it is refused
// because of its segments, not because "." is refused as an authority, and
// off Windows, which has no device namespace, all of them are refused.
func TestADevicePathStaysOnItsDrive(t *testing.T) {
	const control lsp.DocumentUri = "file://./c%3A/x/a%5Cb.mut"
	got, ok := uriToPath(control)
	if runtime.GOOS != "windows" {
		if ok {
			t.Errorf("uriToPath(%s) = %q off Windows, want a refusal", control, got)
		}
	} else if want := filepath.FromSlash("//./c:/x/a/b.mut"); !ok || got != want {
		t.Errorf("uriToPath(%s) = %q ok=%v, want %q", control, got, ok, want)
	}
	for _, uri := range []lsp.DocumentUri{
		"file://./c%3A/x/../../PhysicalDrive0",
		"file://./c%3A/x%5C..%5C..%5CPhysicalDrive0",
		"file://./c%3A/..%5CPhysicalDrive0",
		"file://./c%3A/x/..%5C..%5CPhysicalDrive0",
		"file://./c%3A/x%5C.%5C..%5C..%5CPhysicalDrive0",
		"file://./c%3A/x%5C%5C..%5C..%5CPhysicalDrive0",
		"file://./z%3A/a%5C..%5C..%5CC:/x/a.mut",
	} {
		if got, ok := uriToPath(uri); ok {
			t.Errorf("uriToPath(%s) = %q, want a refusal", uri, got)
		}
	}
}

// TestPathToURIGivesOnlyURIsThatReadBack: what pathToURI gives is handed to
// the client and comes back in its requests, so a URI that does not parse, or
// that uriToPath reads as another file or not at all, is a document the server
// cannot match to its file (M26-LSP-031). Each row is a path, the URI wanted
// for it ("" where no URI that uriToPath accepts reads back to the path: VS
// Code reads file://alpha%40SSL/... and file://%3F/GLOBALROOT/... back as the
// WebDAV and shadow-copy paths, but url.Parse rejects an escaped "@" or "?" in
// an authority), the path that URI reads back as, and the URI VS Code's
// URI.file(path).toString() gives for the same path.
//
// The last column is the scan's rule. A file is indexed only when its URI reads
// back to the file itself, and that has to hold exactly when the editor's own
// spelling reads back to it too: otherwise the document the editor opens could
// never replace the copy the scan read from disk.
func TestPathToURIGivesOnlyURIsThatReadBack(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("drive, UNC and device-namespace paths are Windows spellings")
	}
	for _, c := range []struct {
		name, path, uri, back, vscode string
		indexed                       bool
	}{
		{"drive", "C:/x/a.mut", "file:///C:/x/a.mut", "C:/x/a.mut",
			"file:///c%3A/x/a.mut", true},
		{"UNC", "//alpha/share/x/a.mut", "file://alpha/share/x/a.mut", "//alpha/share/x/a.mut",
			"file://alpha/share/x/a.mut", true},
		{"device", "//./C:/x/a.mut", "file://./C:/x/a.mut", "//./C:/x/a.mut",
			"file://./c%3A/x/a.mut", true},
		{"long path", "//?/C:/x/a.mut", "file:///C:/x/a.mut", "C:/x/a.mut",
			"file://%3F/c%3A/x/a.mut", false},
		{"long-path UNC", "//?/UNC/alpha/share/x/a.mut", "file://alpha/share/x/a.mut", "//alpha/share/x/a.mut",
			"file://%3F/UNC/alpha/share/x/a.mut", false},
		{"device UNC", "//./UNC/alpha/share/x/a.mut", "file://alpha/share/x/a.mut", "//alpha/share/x/a.mut",
			"file://./UNC/alpha/share/x/a.mut", false},
		{"WebDAV", "//alpha@SSL/DavWWWRoot/x.mut", "", "",
			"file://alpha@ssl/DavWWWRoot/x.mut", false},
		{"WebDAV on a port", "//alpha@8080/DavWWWRoot/x.mut", "", "",
			"file://alpha@8080/DavWWWRoot/x.mut", false},
		{"shadow copy", "//?/GLOBALROOT/Device/HarddiskVolumeShadowCopy1/x.mut", "", "",
			"file://%3F/GLOBALROOT/Device/HarddiskVolumeShadowCopy1/x.mut", false},
		// The long-path prefix turns normalisation off, so without it these
		// name another file: a.mut. would be a.mut, and CON the console.
		{"long path, trailing dot", "//?/C:/x/a.mut.", "", "",
			"file://%3F/c%3A/x/a.mut.", false},
		{"long path, device name", "//?/C:/x/CON", "", "",
			"file://%3F/c%3A/x/CON", false},
		{"long path, dot dot", "//?/C:/x/../a.mut", "", "",
			"file://%3F/c%3A/x/../a.mut", false},
		{"volume", "//./C:", "", "",
			"file://./c%3A", false},
	} {
		path := filepath.FromSlash(c.path)
		uri := pathToURI(path)
		if uri != lsp.DocumentUri(c.uri) {
			t.Errorf("%s: pathToURI(%q) = %q, want %q", c.name, path, uri, c.uri)
			continue
		}
		readsBackAsItself := false
		if uri != "" {
			u, err := url.Parse(string(uri))
			if err != nil {
				t.Errorf("%s: pathToURI(%q) = %q, which does not parse: %v", c.name, path, uri, err)
				continue
			}
			// VS Code's URI validator: "If a URI does not contain an authority
			// component, then the path cannot begin with two slash characters".
			if u.Host == "" && strings.HasPrefix(u.Path, "//") {
				t.Errorf("%s: pathToURI(%q) = %q, which VS Code refuses", c.name, path, uri)
			}
			back, ok := uriToPath(uri)
			if want := filepath.FromSlash(c.back); !ok || back != want {
				t.Errorf("%s: uriToPath(%s) = %q ok=%v, want %q", c.name, uri, back, ok, want)
				continue
			}
			readsBackAsItself = canonicalPath(back) == canonicalPath(path)
		}
		editor, ok := uriToPath(lsp.DocumentUri(c.vscode))
		editorReadsBack := ok && canonicalPath(editor) == canonicalPath(path)
		if readsBackAsItself != c.indexed || editorReadsBack != c.indexed {
			t.Errorf("%s: %q reads back as itself through pathToURI: %v, through VS Code's %s: %v; want both %v",
				c.name, path, readsBackAsItself, c.vscode, editorReadsBack, c.indexed)
		}
	}
}

// TestPathToURIOnAPOSIXPath is the guard off Windows: a POSIX path, and the
// //host/path spelling uriToPath gives a file URI with a host there, both read
// back unchanged.
func TestPathToURIOnAPOSIXPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX spellings")
	}
	for _, p := range []string{"/home/u/a.mut", "/tmp/a b/c%d#e.mut", "//alpha/share/x.mut"} {
		uri := pathToURI(p)
		if _, err := url.Parse(string(uri)); err != nil || uri == "" {
			t.Errorf("pathToURI(%q) = %q, err=%v", p, uri, err)
			continue
		}
		if back, ok := uriToPath(uri); !ok || back != p {
			t.Errorf("uriToPath(pathToURI(%q)) = %q ok=%v", p, back, ok)
		}
	}
}
