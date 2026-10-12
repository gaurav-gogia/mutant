package sema

import (
	"fmt"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"
)

// ImportPath returns the path an import spelling names on this host, before it
// is joined onto any directory.
//
// A backslash is a separator on every host, as a forward slash is. A Windows
// author may well type one, ast.ImportStatement.Namespace already reads it as a
// separator, and leaving it alone elsewhere made the same import line open
// lib/util.mut on Windows and look for a file literally named `lib\util.mut`
// on Linux (M26-RUN-012). The cost is that a file whose name contains a
// backslash -- legal outside Windows -- cannot be imported by that name; no
// such file can exist on Windows, so no import that works there loses anything.
//
// module.Resolver and Workspace.resolveImportFrom both start from this, so the
// build and the editor read a spelling the same way.
func ImportPath(spelling string) string {
	return filepath.FromSlash(strings.ReplaceAll(spelling, `\`, "/"))
}

// ImportSpellingProblem returns why spelling cannot be resolved the same way on
// every host, or "" when it can.
//
// Whitespace at either end is refused first, whatever the host. The one case
// with a host behind it, a trailing space, is left to the segment rule below,
// which names that cause; any other edge whitespace is refused as a rule, with
// no cause claimed, because no host drops it.
//
// A spelling that begins with a separator but names no volume (`/lib/x.mut`,
// `\lib\x.mut`) is refused on Windows only. There it is rooted on the
// compiler's current drive -- not absolute, so joining it onto the
// importer's directory, as an ordinary relative path is, would resolve it
// somewhere Windows itself never would -- while on every other host it is an
// absolute path, which docs/MODULES.md says is used as written. Before this,
// such a line opened a file beside the importer on Windows and looked at the
// filesystem root on Linux (M26-RUN-012). lsp/internal/server/link_path.go
// refuses the same shape for the same reason.
//
// A path segment ending in a space or a dot is refused on every host. Win32
// path normalisation drops every trailing space and dot from the last segment
// and a single trailing dot from any other -- measured with os.Stat on Windows
// 11: `util.mut ` opens util.mut and `lib./util.mut` opens lib/util.mut, while
// `lib /util.mut` and `lib../util.mut` are looked up as written -- and Windows
// strips the same characters from a name when it creates a file, so a name
// ending in either cannot be made there. Other systems keep both, so the same
// import line builds on one host and is "cannot find module" on another.
// Holding every segment to the rule is the only reading that does not depend
// on where the segment sits.
//
// Each such spelling is refused rather than repaired: repairing would leave the
// namespace derived from the text as written, and either reading silently
// picks one host's meaning for the line.
//
// It answers only the portability question. Whether the spelling names a
// Mutant source file at all (the extension, an empty path) is module's rule.
func ImportSpellingProblem(spelling string) string {
	if r, _ := utf8.DecodeRuneInString(spelling); unicode.IsSpace(r) {
		return fmt.Sprintf("the path begins with whitespace (%U); an import "+
			"path must begin and end with a character that is not whitespace", r)
	}
	if r, _ := utf8.DecodeLastRuneInString(spelling); unicode.IsSpace(r) && r != ' ' {
		return fmt.Sprintf("the path ends with whitespace (%U); an import "+
			"path must begin and end with a character that is not whitespace", r)
	}
	if local := ImportPath(spelling); strings.HasPrefix(local, string(filepath.Separator)) && !filepath.IsAbs(local) {
		return "the path begins with a separator but names no drive, so " +
			"Windows reads it from the root of the compiler's current drive " +
			"and other systems from the filesystem root; write a path " +
			"relative to the importing file, or an absolute path with a drive"
	}
	for _, segment := range strings.Split(strings.ReplaceAll(spelling, `\`, "/"), "/") {
		if segment == "" || segment == "." || segment == ".." {
			continue
		}
		// A space (U+0020) and a dot are the two trailing characters Windows
		// drops from a name; no host drops any other.
		last := segment[len(segment)-1]
		if last == '.' || last == ' ' {
			what := "a dot"
			if last == ' ' {
				what = "a space"
			}
			return fmt.Sprintf("the segment %q ends in %s: Windows does not "+
				"keep a space or a dot at the end of a name -- it drops them "+
				"when it creates a file, and when they end the path it opens "+
				"-- while other systems keep them, so the path need not name "+
				"the same file on each", segment, what)
		}
	}
	return ""
}
