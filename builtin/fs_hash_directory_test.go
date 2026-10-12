package builtin

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"mutant/object"
)

// fsHashDirectoryWant is the whole refusal fs_hash gives for a directory,
// spelled out here rather than read back from the builtin, so a change to its
// words is a change these tests see.
func fsHashDirectoryWant(path string) string {
	return "fs_hash: " + path + " is a directory, and a directory has no digest; " +
		"hash each file in it instead (fs_walk lists them)"
}

// fsHashExpectDirectoryRefusal calls fs_hash and requires the directory
// refusal, word for word, with no result beside it.
func fsHashExpectDirectoryRefusal(t *testing.T, path string, args ...object.Object) {
	t.Helper()
	result, errObj := unwrapPair(t, FsHash(append([]object.Object{stringObj(path)}, args...)...))
	if errObj == nil {
		t.Fatalf("fs_hash(%q) gave no error, and a result of %s", path, result.Inspect())
	}
	if result.Type() != object.NULL_OBJ {
		t.Errorf("fs_hash(%q) refused, and still gave a result: %s", path, result.Inspect())
	}
	if want := fsHashDirectoryWant(path); errObj.Message != want {
		t.Errorf("fs_hash(%q) refused in other words: got %q, want %q", path, errObj.Message, want)
	}
}

// fs_hash refuses a directory in its own words, the same on every host. It
// used to read the directory as a file and pass the read's error on, so the
// words were the host's -- "read <dir>: Incorrect function." on Windows and
// "read <dir>: is a directory" on Linux -- and a script could not match one
// message on both, nor could a golden recorded on one host pass on the other
// (M26-BLT-002). fs_integrity_baseline stopped handing fs_hash directories
// (M26-EX-003), so no example holds this any more.
func TestFsHashRefusesDirectoryInItsOwnWords(t *testing.T) {
	dir := t.TempDir()
	t.Run("a directory", func(t *testing.T) {
		fsHashExpectDirectoryRefusal(t, dir)
	})
	t.Run("a directory with an algorithm", func(t *testing.T) {
		fsHashExpectDirectoryRefusal(t, dir, stringObj("md5"))
	})
	t.Run("a directory spelled with a trailing separator", func(t *testing.T) {
		fsHashExpectDirectoryRefusal(t, dir+string(os.PathSeparator))
	})
}

// A directory its user may not list cannot even be opened, so reading it
// failed at the open, in the host's words for that: "open <dir>: permission
// denied". It is refused in the same words as any other directory.
func TestFsHashRefusesUnlistableDirectoryInItsOwnWords(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("os.Chmod on Windows only sets the read-only attribute, which does not stop a directory being listed")
	}
	dir := filepath.Join(t.TempDir(), "unlistable")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatalf("mkdir fixture: %v", err)
	}
	if err := os.Chmod(dir, 0); err != nil {
		t.Fatalf("chmod fixture: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	fsHashExpectDirectoryRefusal(t, dir)
}

// The refusal is for a directory and nothing else. A file still hashes; so
// does a named stream a directory carries on NTFS, which is content of its
// own and which os.Stat reports as the stream, not as the directory; and a
// path that is not there is not called a directory.
func TestFsHashDirectoryRefusalLeavesFilesAlone(t *testing.T) {
	dir := t.TempDir()
	content := []byte("fs_hash directory refusal control")
	sum := sha256.Sum256(content)
	wantDigest := hex.EncodeToString(sum[:])

	expectDigest := func(t *testing.T, path string) {
		t.Helper()
		result, errObj := unwrapPair(t, FsHash(stringObj(path), stringObj("sha256")))
		if errObj != nil {
			t.Fatalf("fs_hash(%q): %s", path, errObj.Inspect())
		}
		payload, ok := result.(*object.Hash)
		if !ok {
			t.Fatalf("fs_hash(%q) payload type: %T", path, result)
		}
		if got := fsfxMustHashString(t, payload, "hash"); got != wantDigest {
			t.Errorf("fs_hash(%q) hash = %s, want %s", path, got, wantDigest)
		}
	}

	t.Run("a file", func(t *testing.T) {
		file := filepath.Join(dir, "evidence.bin")
		if err := os.WriteFile(file, content, 0o644); err != nil {
			t.Fatalf("write fixture: %v", err)
		}
		expectDigest(t, file)
	})

	t.Run("a named stream on a directory", func(t *testing.T) {
		if runtime.GOOS != "windows" {
			t.Skip("named streams are an NTFS feature")
		}
		stream := dir + ":evidence"
		if err := os.WriteFile(stream, content, 0o644); err != nil {
			t.Skipf("the temporary directory's volume holds no named stream on a directory: %v", err)
		}
		expectDigest(t, stream)
	})

	t.Run("a path that is not there", func(t *testing.T) {
		missing := filepath.Join(dir, "absent.bin")
		result, errObj := unwrapPair(t, FsHash(stringObj(missing)))
		if errObj == nil {
			t.Fatalf("fs_hash(%q) gave no error, and a result of %s", missing, result.Inspect())
		}
		if strings.Contains(errObj.Message, "is a directory") {
			t.Errorf("fs_hash(%q) called a missing path a directory: %s", missing, errObj.Message)
		}
	})
}
