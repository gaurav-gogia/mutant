package server

import (
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	lsp "github.com/tliron/glsp/protocol_3_16"

	"mutant/sema"
)

const (
	// scanMaxFiles bounds a workspace crawl so a huge or pathological tree can
	// never stall startup or exhaust memory.
	scanMaxFiles = 5000
	// scanMaxFileBytes skips oversized files (unlikely to be real Mutant source).
	scanMaxFileBytes = 1 << 20 // 1 MiB
)

// pathToURI converts an absolute filesystem path to a file:// URI. It is the
// canonical form used for disk-indexed documents so go-to-definition into an
// unopened file yields a URI the client can open.
//
// It returns "" for a path that no URI uriToPath accepts reads back to, and a
// caller must take "" as having no URI to offer. Every URI it does give parses
// and reads back through uriToPath to a path naming the same file, because the
// client hands the URI back in its requests: one that uriToPath refuses, or
// reads as another path, is a document the server cannot match to the file it
// came from (M26-LSP-031). The limit is this server's, not the URI scheme's.
// VS Code reads file://alpha%40SSL/DavWWWRoot/x.mut back as the WebDAV share
// \\alpha@SSL\DavWWWRoot\x.mut, and file://%3F/GLOBALROOT/... back as
// \\?\GLOBALROOT\..., but url.Parse rejects an escaped "@" or "?" in an
// authority, and VS Code's own spelling of the share,
// file://alpha@ssl/DavWWWRoot/x.mut, carries a user part, which uriToPath
// refuses. Both paths get "".
func pathToURI(p string) lsp.DocumentUri {
	slashed := filepath.ToSlash(p)
	if runtime.GOOS == "windows" {
		ordinary, ok := withoutLongPathPrefix(slashed)
		if !ok {
			return ""
		}
		slashed = ordinary
	}
	var u url.URL
	if rest, unc := strings.CutPrefix(slashed, "//"); unc {
		// A UNC path names its host in the authority, as VS Code's URI.file
		// spells it: \\host\share\x.mut is file://host/share/x.mut. Left in the
		// path it was file:////host/share/x.mut, which VS Code's URI.parse
		// refuses -- "If a URI does not contain an authority component, then
		// the path cannot begin with two slash characters" (M26-LSP-031). So
		// two leading slashes with no host after them get no URI here.
		host, tail, found := strings.Cut(rest, "/")
		if !found || host == "" {
			return ""
		}
		u = url.URL{Scheme: "file", Host: host, Path: "/" + tail}
	} else {
		path := slashed
		if !strings.HasPrefix(path, "/") {
			// Windows drive path (O:/project/x.mut) -> /O:/project/x.mut.
			path = "/" + path
		}
		u = url.URL{Scheme: "file", Path: path}
	}
	uri := lsp.DocumentUri(u.String())
	if back, ok := uriToPath(uri); !ok || back != filepath.FromSlash(slashed) {
		return ""
	}
	return uri
}

// withoutLongPathPrefix respells a Windows path written with the \\?\ long-path
// prefix, or in the \\.\UNC\ device form, as the ordinary path of the same
// file: \\?\C:\x is C:\x, and \\?\UNC\host\share\x and \\.\UNC\host\share\x
// are \\host\share\x. No URI uriToPath reads spells those prefixes -- VS Code
// writes the authority "?" as file://%3F/..., an escape url.Parse rejects --
// while the ordinary spelling is one every client can open.
//
// \\?\ also switches Windows' path normalisation off, and \\.\ normalises
// against \\.\ rather than the share, so the respelling names the same file
// only when normalisation would leave every segment alone. An empty, "." or
// ".." segment, a name ending in a dot or a space, and a device name such as
// CON all name something else once the prefix is gone; they report false, and
// so does every other \\?\ path (\\?\GLOBALROOT\..., \\?\Volume{...}\...),
// which has no ordinary spelling at all.
func withoutLongPathPrefix(slashed string) (string, bool) {
	for _, prefix := range []string{"//?/UNC/", "//./UNC/"} {
		if len(slashed) > len(prefix) && strings.EqualFold(slashed[:len(prefix)], prefix) {
			rest := slashed[len(prefix):]
			return "//" + rest, plainSegments(rest)
		}
	}
	if rest, long := strings.CutPrefix(slashed, "//?/"); long {
		if len(rest) < 4 || !isASCIILetter(rest[0]) || rest[1] != ':' || rest[2] != '/' {
			return "", false
		}
		return rest, plainSegments(rest[3:])
	}
	return slashed, true
}

// plainSegments reports whether every segment of a slashed path is a name
// Windows' path normalisation leaves exactly as it is.
func plainSegments(slashed string) bool {
	for _, segment := range strings.Split(slashed, "/") {
		if segment == "" || segment == "." || strings.HasSuffix(segment, ".") ||
			strings.HasSuffix(segment, " ") || !filepath.IsLocal(segment) {
			return false
		}
	}
	return true
}

func isASCIILetter(b byte) bool {
	return 'a' <= b && b <= 'z' || 'A' <= b && b <= 'Z'
}

// uriToPath converts a file:// URI back to a filesystem path. ok is false for
// non-file URIs, and for an authority no path can name.
//
// The authority is part of the file's name. VS Code spells \\host\share\x.mut as
// file://host/share/x.mut, and dropping the host left \share\x.mut: one string
// for every host serving that share path, and one that filepath.Abs then
// resolved onto the server's current drive (M26-LSP-031). It is put back the way
// VS Code's own fsPath does, as //host/share/x.mut -- the UNC path on Windows.
//
// localhost is RFC 8089's name for this machine, and it stays one wherever it
// cannot be a share: before a drive letter, since \\localhost\C:\ is not a
// share name, and off Windows, which has no UNC paths at all.
//
// The authority "." before a drive letter is the device namespace: VS Code
// spells \\.\o:\x, which is o:\x under another name, as file://./o%3A/x, and
// its fsPath reads that back as \\.\o:\x. Refusing it cost a workspace opened
// that way its module keys, its links and its cross-file definitions, and left
// the scanned copy of every open document in the index beside the editor's
// (M26-LSP-031).
func uriToPath(uri lsp.DocumentUri) (string, bool) {
	u, err := url.Parse(string(uri))
	if err != nil || u.Scheme != "file" {
		return "", false
	}
	p := u.Path // already percent-decoded
	drive := len(p) >= 3 && p[0] == '/' && p[2] == ':'
	host := u.Host
	if strings.EqualFold(host, "localhost") && (drive || runtime.GOOS != "windows") {
		host = ""
	}
	if host == "." && u.User == nil && aDrivePathInTheDeviceNamespace(p) {
		return filepath.FromSlash("//." + p), true
	}
	if host != "" {
		// A user part is refused although VS Code writes one for a share: it
		// spells the WebDAV share \\alpha@SSL\DavWWWRoot\x.mut as
		// file://alpha@ssl/DavWWWRoot/x.mut, cutting the authority at the "@"
		// and lowercasing what follows. A generic URI reader takes that shape
		// as the user alpha on a host named ssl, so neither the UNC path nor
		// the machine can be rebuilt from it with certainty -- and dropping
		// the authority, as this did before M26-LSP-031, read it as a file on
		// the server's own drive. A port or an IPv6 literal names no share,
		// and nor does a host with no path after it or a drive letter on
		// another machine. "." is the device namespace, read above only
		// before a drive letter; "?" would make \\?\, and url.Parse cannot
		// deliver it anyway, since it rejects the escape in file://%3F/...
		if u.User != nil || strings.ContainsAny(host, ":[]") || host == "." || host == "?" || drive || len(p) <= 1 {
			return "", false
		}
		return filepath.FromSlash("//" + host + p), true
	}
	// Strip the leading slash before a Windows drive: /O:/x -> O:/x.
	if drive {
		p = p[1:]
	}
	return filepath.FromSlash(p), true
}

// aDrivePathInTheDeviceNamespace reports whether p, the path of a file URI
// whose authority is ".", names something on a drive: /X:/ and what follows,
// on Windows, the one system with a device namespace. \\.\X: alone is the
// volume rather than its root directory, so the slash after the colon is
// required. A "." or ".." segment is refused because the namespace's root is
// \\.\ and not the drive: Windows resolves \\.\C:\x\..\..\PhysicalDrive0 to
// \\.\PhysicalDrive0, a disk, where the same segments under C:\ stop at C:\.
//
// Segments end at a backslash as well as a slash, because Windows reads both
// as a separator and p arrives percent-decoded: split on "/" alone,
// file://./c%3A/x%5C..%5C..%5CPhysicalDrive0 had no ".." segment, and
// file://./z%3A/a%5C..%5C..%5CC:/x read a file on C: through a URI naming z:.
// Only an exact "." or ".." moves: Windows reads ".. " and "..." as names, and
// trims them to nothing only at the end of the path, which stays on the drive.
func aDrivePathInTheDeviceNamespace(p string) bool {
	if runtime.GOOS != "windows" || len(p) < 4 || p[0] != '/' || !isASCIILetter(p[1]) || p[2] != ':' || p[3] != '/' {
		return false
	}
	separator := func(r rune) bool { return r == '/' || r == '\\' }
	for _, segment := range strings.FieldsFunc(p[4:], separator) {
		if segment == "." || segment == ".." {
			return false
		}
	}
	return true
}

// canonicalPath returns a comparison key for a path that is stable across the
// URI-encoding differences between clients. It is used to detect that a
// disk-indexed file and a later-opened editor document are the same file even
// when their URIs differ.
//
// The comparison rule is sema.CanonicalKey, which module resolution also uses
// to decide that two import paths name one file. It has to be the same rule:
// the key this returns is what a document is filed under, and what an import
// from another file will look it up by. Making the path absolute stays here
// because that is where the two callers differ -- the loader resolved its path
// itself, while an editor hands us URIs and we must degrade rather than refuse.
func canonicalPath(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		abs = p
	}
	return sema.CanonicalKey(abs)
}

// sameFileKey is canonicalPath with one more fold, which sema.CanonicalKey
// lacks: a drive path in the device namespace, \\.\c:\x, keys as the drive
// path c:\x, the same file to Windows. It answers whether an editor document
// is a file the scan read, and whether the scan has already collected a file.
//
// uriToPath reads VS Code's file://./c%3A/x as \\.\c:\x, as its fsPath does,
// and one file reaches the editor under both spellings -- from the explorer of
// a folder opened as \\.\c:\..., and as c:\... from a terminal link or File >
// Open. Compared by canonicalPath, the editor's copy never evicted the copy the
// scan read from disk, and workspace/symbol went on offering a name the editor
// had renamed (M26-LSP-031).
//
// The fold belongs in sema.CanonicalKey, where an import would resolve across
// the two spellings too; until it is there, sema files such a file as two
// modules, and forgetSplitModule removes the disk one. It is not made in
// canonicalPath, which has to stay sema's rule, and which also dedupes the
// roots a document link may land in: linkCandidate relates a literal to a root
// by its spelling, so \\.\c:\x and c:\x are both kept there.
func sameFileKey(p string) string {
	return withoutTheDeviceNamespace(canonicalPath(p))
}

// withoutTheDeviceNamespace folds a key for a drive path in the device
// namespace, \\.\c:\x, to the key for c:\x, on Windows. It folds only the
// spelling uriToPath reads. \\.\c: with nothing after it is the volume, not
// the drive's root directory; and a \\?\ or \\.\UNC\ path is one whose editor
// URI uriToPath refuses, so reading it as its plain path would let the scan
// index a file under a key no open document can evict (see indexFileFromDisk).
func withoutTheDeviceNamespace(key string) string {
	if runtime.GOOS != "windows" || len(key) < 7 || key[:4] != `\\.\` ||
		!isASCIILetter(key[4]) || key[5] != ':' || key[6] != '\\' {
		return key
	}
	return key[4:]
}

// scannedKeysFor returns every key s.scanned can hold the file at path under.
// The map is keyed by canonicalPath, as didChangeWatchedFiles deletes from it,
// so a drive path and its device-namespace spelling are two keys there, and
// the scan files the file under whichever one its root was written in.
func scannedKeysFor(path string) []string {
	key := canonicalPath(path)
	if runtime.GOOS != "windows" {
		return []string{key}
	}
	if plain := withoutTheDeviceNamespace(key); plain != key {
		return []string{key, plain}
	}
	if len(key) >= 3 && isASCIILetter(key[0]) && key[1] == ':' && key[2] == '\\' {
		return []string{key, `\\.\` + key}
	}
	return []string{key}
}

// anOpenDocumentIsTheFile reports whether any open document is the file at
// path, whatever URI spelling the client used to open it.
//
// It walks the open set rather than consulting an index kept beside it. The set
// holds the files an examiner has open -- tens at the most -- while a scan
// visits every .mut under the roots, so the walk is the cheaper thing to get
// wrong and the index is the more expensive thing to let drift. A canonical
// path is derived here, from the one URI the client gave, every time it is
// needed; there is no second copy of it to fall out of step. It is compared by
// sameFileKey, so a document open as c:\x is the file the scan reaches as
// \\.\c:\x (M26-LSP-031).
func (s *Server) anOpenDocumentIsTheFile(path string) bool {
	key := sameFileKey(path)
	for _, uri := range s.documents.URIs() {
		open, ok := uriToPath(uri)
		if !ok {
			continue
		}
		if sameFileKey(open) == key {
			return true
		}
	}
	return false
}

// skipScanDir reports whether a directory should be pruned from the crawl.
func skipScanDir(name string) bool {
	switch name {
	case ".git", "node_modules", "vendor":
		return true
	}
	// Any other dot-directory.
	return strings.HasPrefix(name, ".") && name != "." && name != ".."
}

// collectMutFiles walks roots and returns absolute paths of `*.mut` files,
// bounded by scanMaxFiles. Directories such as .git/node_modules/vendor are
// pruned.
func collectMutFiles(roots []string) []string {
	var files []string
	seen := make(map[string]struct{})

	for _, root := range roots {
		if root == "" {
			continue
		}
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil // ignore unreadable entries, keep walking
			}
			if len(files) >= scanMaxFiles {
				return filepath.SkipAll
			}
			if d.IsDir() {
				if skipScanDir(d.Name()) {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(strings.ToLower(d.Name()), ".mut") {
				return nil
			}
			// One file under two roots, \\.\c:\x and c:\x, is collected once:
			// indexed under both spellings, it would be offered twice.
			key := sameFileKey(path)
			if _, dup := seen[key]; dup {
				return nil
			}
			seen[key] = struct{}{}
			files = append(files, path)
			return nil
		})
	}
	return files
}

// indexFileFromDisk reads, analyzes, and indexes a single file under its
// canonical URI, unless an editor already has it open (open snapshots win) or
// pathToURI gives it no URI that reads back to it. It records the
// canonical-path -> URI mapping so a later didOpen can evict the stale disk
// entry. Returns false if the file was skipped.
func (s *Server) indexFileFromDisk(path string) bool {
	uri := pathToURI(path)
	// A file is indexed under a URI that reads back to this same file, or not
	// at all. The editor's copy replaces the disk copy only when the URI the
	// editor opens it under reads back to the same canonical path, and a path
	// pathToURI cannot spell ("") or can spell only as another name for the
	// file (\\?\C:\x as file:///C:/x) is one whose editor spelling uriToPath
	// refuses: VS Code writes those file://%3F/c%3A/x and
	// file://alpha@ssl/DavWWWRoot/x. Indexed anyway, the disk copy of an open
	// document stayed beside the editor's for good, and workspace/symbol kept
	// offering a name the editor had renamed (M26-LSP-031).
	if back, ok := uriToPath(uri); !ok || canonicalPath(back) != canonicalPath(path) {
		return false
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() || info.Size() > scanMaxFileBytes {
		return false
	}
	// Whether an editor holds this file is a question about the file, not about
	// the spelling of its URI. This asked s.documents for pathToURI(path),
	// which is file:///C:/..., while VS Code opens documents as
	// file:///c%3A/... -- so on Windows the check missed every open document.
	// The scan then indexed the file a second time under the other spelling,
	// and called sema.PutFile, which is keyed by canonical path, with the copy
	// from disk: the editor's unsaved declarations were replaced by what had
	// been saved, and the importing file was told they did not exist
	// (M26-LSP-005).
	if s.anOpenDocumentIsTheFile(path) {
		return false // an editor copy is authoritative
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	// The disk scan is what makes an import to an unopened file resolve at
	// all, so a scanned file gets the same treatment an open one does: a
	// module key and the shared workspace.
	snapshot := s.analyzeDoc(uri, string(data))

	s.mu.Lock()
	if s.scanned == nil {
		s.scanned = make(map[string]lsp.DocumentUri)
	}
	s.scanned[canonicalPath(path)] = uri
	s.mu.Unlock()

	s.safeSymbolUpdate(uri, snapshot)
	s.safeSemaUpdate(uri, snapshot)
	return true
}

// scanWorkspace indexes every `*.mut` under the configured roots into the symbol
// index (without keeping them as open documents), so cross-file navigation works
// across files the editor has not opened. It runs in the background.
func (s *Server) scanWorkspace() {
	defer func() {
		if recovered := recover(); recovered != nil {
			logRecoveredPanic("workspace.scan", "", recovered)
		}
	}()

	s.mu.RLock()
	roots := append([]string(nil), s.roots...)
	s.mu.RUnlock()
	if len(roots) == 0 {
		return
	}

	for _, path := range collectMutFiles(roots) {
		s.indexFileFromDisk(path)
	}
}

// evictScannedEntryFor removes a stale disk-indexed symbol entry when the editor
// opens the same underlying file under a different URI, preventing duplicate
// (and therefore ambiguous) cross-file symbols.
func (s *Server) evictScannedEntryFor(openedURI lsp.DocumentUri) {
	path, ok := uriToPath(openedURI)
	if !ok {
		return
	}

	// Every spelling the scan can have filed the file under, so that lib.mut
	// opened as c:\... evicts the copy read through a root opened as \\.\c:\...
	// (M26-LSP-031).
	var stale []lsp.DocumentUri
	s.mu.Lock()
	for _, key := range scannedKeysFor(path) {
		if staleURI, tracked := s.scanned[key]; tracked {
			delete(s.scanned, key)
			stale = append(stale, staleURI)
		}
	}
	s.mu.Unlock()

	for _, staleURI := range stale {
		if staleURI != openedURI {
			s.safeSymbolDelete(staleURI)
			s.forgetSplitModule(staleURI, path)
		}
	}
}

// forgetSplitModule removes the disk copy filed under staleURI from the shared
// workspace when the workspace keeps it as another module than the document the
// editor is opening at path. Opening the editor's copy replaces the disk copy
// in sema only when the two share a module key, and a pair sameFileKey reads
// as one file can still have two keys there -- \\.\c:\x and c:\x among them,
// since sema.CanonicalKey does not fold the device namespace (see
// sameFileKey). With lib.mut open as c:\... and main.mut as \\.\c:\...,
// main.mut's import of "lib.mut" went on resolving to the copy read from disk:
// a declaration renamed in the editor was still found there, and its new name
// was reported missing (M26-LSP-031). Removed, that import is unresolved, which
// renders as nothing; resolving it to the editor's copy needs the fold in
// sema.CanonicalKey.
func (s *Server) forgetSplitModule(staleURI lsp.DocumentUri, path string) {
	defer func() {
		if recovered := recover(); recovered != nil {
			logRecoveredPanic("sema.RemoveFile", staleURI, recovered)
		}
	}()
	if s.sema == nil {
		return
	}
	// PutFile keys the document being opened by sema.CanonicalKey of the path
	// uriToPath gives, so that is the key the disk copy has to share.
	if key, filed := s.sema.KeyForURI(string(staleURI)); filed && key != sema.CanonicalKey(path) {
		s.sema.RemoveFile(string(staleURI))
	}
}
