package builtin

import (
	"bytes"
	"debug/dwarf"
	"debug/elf"
	"debug/macho"
	"debug/pe"
	"os"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	saferpe "github.com/saferwall/pe"

	"mutant/object"
)

func BinPEParse(args ...object.Object) object.Object {
	if len(args) != 1 {
		return resultAndError(nil, newError("wrong number of arguments. got=%d, want=1", len(args)))
	}

	pathObj, ok := args[0].(*object.String)
	if !ok {
		return resultAndError(nil, newError("argument 1 to `bin_pe_parse` must be STRING, got %s", args[0].Type()))
	}

	f, err := saferpe.New(pathObj.Value, &saferpe.Options{Fast: true})
	if err != nil {
		return resultAndError(nil, newError("bin_pe_parse: %s", err.Error()))
	}
	defer f.Close()

	if err := f.Parse(); err != nil {
		return resultAndError(nil, newError("bin_pe_parse: %s", err.Error()))
	}

	header := f.NtHeader.FileHeader

	return resultAndError(makeHashObject(map[string]object.Object{
		"path":            stringObj(pathObj.Value),
		"format":          stringObj("pe"),
		"machine":         intObj(int64(header.Machine)),
		"num_sections":    intObj(int64(header.NumberOfSections)),
		"characteristics": intObj(int64(header.Characteristics)),
		"timestamp":       intObj(int64(header.TimeDateStamp)),
	}), nil)
}

func BinELFParse(args ...object.Object) object.Object {
	if len(args) != 1 {
		return resultAndError(nil, newError("wrong number of arguments. got=%d, want=1", len(args)))
	}

	pathObj, ok := args[0].(*object.String)
	if !ok {
		return resultAndError(nil, newError("argument 1 to `bin_elf_parse` must be STRING, got %s", args[0].Type()))
	}

	f, err := elf.Open(pathObj.Value)
	if err != nil {
		return resultAndError(nil, newError("bin_elf_parse: %s", err.Error()))
	}
	defer f.Close()

	return resultAndError(makeHashObject(map[string]object.Object{
		"path":         stringObj(pathObj.Value),
		"format":       stringObj("elf"),
		"class":        stringObj(f.Class.String()),
		"data":         stringObj(f.Data.String()),
		"machine":      stringObj(f.Machine.String()),
		"type":         stringObj(f.Type.String()),
		"entry":        intObj(int64(f.Entry)),
		"num_sections": intObj(int64(len(f.Sections))),
	}), nil)
}

// BinMachOParse parses a Mach-O binary (macOS/iOS native executables), the gap
// left by bin_pe_parse (PE) and bin_elf_parse (ELF). It handles both a thin
// single-architecture image and a fat/universal binary (multiple slices).
// Pure-Go via the stdlib debug/macho, panic-recovered for untrusted input.
func BinMachOParse(args ...object.Object) (result object.Object) {
	defer func() {
		if r := recover(); r != nil {
			result = resultAndError(nil, newError("bin_macho_parse: panic during parse: %v", r))
		}
	}()

	if len(args) != 1 {
		return resultAndError(nil, newError("wrong number of arguments. got=%d, want=1", len(args)))
	}

	pathObj, ok := args[0].(*object.String)
	if !ok {
		return resultAndError(nil, newError("argument 1 to `bin_macho_parse` must be STRING, got %s", args[0].Type()))
	}

	// Universal / fat binary: enumerate the per-architecture slices.
	if fat, err := macho.OpenFat(pathObj.Value); err == nil {
		defer fat.Close()
		arches := make([]object.Object, len(fat.Arches))
		for i := range fat.Arches {
			a := &fat.Arches[i]
			arches[i] = makeHashObject(map[string]object.Object{
				"cpu":    stringObj(a.Cpu.String()),
				"type":   stringObj(a.Type.String()),
				"offset": intObj(int64(a.Offset)),
				"size":   intObj(int64(a.Size)),
				"align":  intObj(int64(a.Align)),
			})
		}
		return resultAndError(makeHashObject(map[string]object.Object{
			"path":          stringObj(pathObj.Value),
			"format":        stringObj("macho"),
			"fat":           boolObj(true),
			"num_arches":    intObj(int64(len(fat.Arches))),
			"architectures": &object.Array{Elements: arches},
		}), nil)
	} else if err != macho.ErrNotFat {
		return resultAndError(nil, newError("bin_macho_parse: %s", err.Error()))
	}

	f, err := macho.Open(pathObj.Value)
	if err != nil {
		return resultAndError(nil, newError("bin_macho_parse: %s", err.Error()))
	}
	defer f.Close()

	libs, _ := f.ImportedLibraries()
	libObjs := make([]object.Object, len(libs))
	for i, l := range libs {
		libObjs[i] = stringObj(l)
	}

	return resultAndError(makeHashObject(map[string]object.Object{
		"path":               stringObj(pathObj.Value),
		"format":             stringObj("macho"),
		"fat":                boolObj(false),
		"magic":              intObj(int64(f.Magic)),
		"cpu":                stringObj(f.Cpu.String()),
		"type":               stringObj(f.Type.String()),
		"flags":              intObj(int64(f.Flags)),
		"num_sections":       intObj(int64(len(f.Sections))),
		"num_commands":       intObj(int64(f.Ncmd)),
		"imported_libraries": &object.Array{Elements: libObjs},
	}), nil)
}

func BinDWARFParse(args ...object.Object) object.Object {
	if len(args) != 1 {
		return resultAndError(nil, newError("wrong number of arguments. got=%d, want=1", len(args)))
	}

	pathObj, ok := args[0].(*object.String)
	if !ok {
		return resultAndError(nil, newError("argument 1 to `bin_dwarf_parse` must be STRING, got %s", args[0].Type()))
	}

	format, data, errObj := loadDWARF(pathObj.Value)
	if errObj != nil {
		return resultAndError(nil, errObj)
	}

	compileUnits := int64(0)
	if data != nil {
		reader := data.Reader()
		for {
			entry, err := reader.Next()
			if err != nil || entry == nil {
				break
			}
			if entry.Tag == dwarf.TagCompileUnit {
				compileUnits++
			}
		}
	}

	return resultAndError(makeHashObject(map[string]object.Object{
		"path":          stringObj(pathObj.Value),
		"format":        stringObj(format),
		"compile_units": intObj(compileUnits),
	}), nil)
}

func BinStrings(args ...object.Object) object.Object {
	if len(args) != 1 && len(args) != 2 {
		return resultAndError(nil, newError("wrong number of arguments. got=%d, want=1 or 2", len(args)))
	}

	pathObj, ok := args[0].(*object.String)
	if !ok {
		return resultAndError(nil, newError("argument 1 to `bin_strings` must be STRING, got %s", args[0].Type()))
	}

	minLen := int64(4)
	if len(args) == 2 {
		minLenObj, ok := args[1].(*object.Integer)
		if !ok {
			return resultAndError(nil, newError("argument 2 to `bin_strings` must be INTEGER, got %s", args[1].Type()))
		}
		if minLenObj.Value < 1 {
			return resultAndError(nil, newError("argument 2 to `bin_strings` must be >= 1"))
		}
		minLen = minLenObj.Value
	}

	data, err := os.ReadFile(pathObj.Value)
	if err != nil {
		return resultAndError(nil, newError("bin_strings: %s", err.Error()))
	}

	values := extractPrintableStrings(data, int(minLen))
	elements := make([]object.Object, len(values))
	for i, v := range values {
		elements[i] = stringObj(v)
	}

	return resultAndError(&object.Array{Elements: elements}, nil)
}

func BinEntropy(args ...object.Object) object.Object {
	if len(args) != 1 {
		return resultAndError(nil, newError("wrong number of arguments. got=%d, want=1", len(args)))
	}

	pathObj, ok := args[0].(*object.String)
	if !ok {
		return resultAndError(nil, newError("argument 1 to `bin_entropy` must be STRING, got %s", args[0].Type()))
	}

	data, err := os.ReadFile(pathObj.Value)
	if err != nil {
		return resultAndError(nil, newError("bin_entropy: %s", err.Error()))
	}

	return resultAndError(makeHashObject(map[string]object.Object{
		"path":    stringObj(pathObj.Value),
		"bytes":   intObj(int64(len(data))),
		"entropy": &object.Float{Value: shannonEntropy(data)},
	}), nil)
}

// BinYaraScan is NOT a YARA engine (a real one requires cgo/libyara, which this
// project forbids). It performs literal multi-string matching, reporting the start
// of every non-overlapping match of each rule string as a byte offset into the
// file. Matching is case-sensitive by default; pass a third boolean argument to
// match case-insensitively (caseFoldMatcher says what that matches).
func BinYaraScan(args ...object.Object) object.Object {
	if len(args) != 2 && len(args) != 3 {
		return resultAndError(nil, newError("wrong number of arguments. got=%d, want=2 or 3", len(args)))
	}

	pathObj, ok := args[0].(*object.String)
	if !ok {
		return resultAndError(nil, newError("argument 1 to `bin_yara_scan` must be STRING, got %s", args[0].Type()))
	}
	rulesObj, ok := args[1].(*object.Array)
	if !ok {
		return resultAndError(nil, newError("argument 2 to `bin_yara_scan` must be ARRAY, got %s", args[1].Type()))
	}
	caseInsensitive := false
	if len(args) == 3 {
		ciObj, ok := args[2].(*object.Boolean)
		if !ok {
			return resultAndError(nil, newError("argument 3 to `bin_yara_scan` must be BOOLEAN, got %s", args[2].Type()))
		}
		caseInsensitive = ciObj.Value
	}

	data, err := os.ReadFile(pathObj.Value)
	if err != nil {
		return resultAndError(nil, newError("bin_yara_scan: %s", err.Error()))
	}
	// Case-insensitive matching does not lower the whole file with
	// strings.ToLower. That writes the three bytes of U+FFFD for every byte that
	// is not valid UTF-8, and a lowered rune can be longer or shorter than the
	// original, so an offset found in the lowered text is not an offset into the
	// file. caseFoldMatcher reports offsets into the file itself.
	var haystack string
	var folder *caseFoldMatcher
	if caseInsensitive {
		folder = &caseFoldMatcher{data: data}
	} else {
		haystack = string(data)
	}

	hits := make([]object.Object, 0)
	totalHits := 0
	for i, ruleObj := range rulesObj.Elements {
		ruleStr, ok := ruleObj.(*object.String)
		if !ok {
			return resultAndError(nil, newError("argument 2 to `bin_yara_scan` must contain STRING rules. element %d got %s", i, ruleObj.Type()))
		}
		pattern := ruleStr.Value
		if pattern == "" {
			continue
		}
		var offsets []int
		if caseInsensitive {
			offsets = folder.offsets(pattern)
		} else {
			offsets = allSubstringOffsets(haystack, pattern)
		}
		if len(offsets) == 0 {
			continue
		}
		totalHits += len(offsets)
		offObjs := make([]object.Object, len(offsets))
		for j, off := range offsets {
			offObjs[j] = intObj(int64(off))
		}
		hits = append(hits, makeHashObject(map[string]object.Object{
			"rule":    stringObj(ruleStr.Value),
			"count":   intObj(int64(len(offsets))),
			"offsets": &object.Array{Elements: offObjs},
		}))
	}

	return resultAndError(makeHashObject(map[string]object.Object{
		"path":             stringObj(pathObj.Value),
		"engine":           stringObj("literal-substring"),
		"case_insensitive": boolObj(caseInsensitive),
		"total_rules":      intObj(int64(len(rulesObj.Elements))),
		"matched":          intObj(int64(len(hits))),
		"total_hits":       intObj(int64(totalHits)),
		"hits":             &object.Array{Elements: hits},
	}), nil)
}

// allSubstringOffsets returns every non-overlapping start offset of needle in
// haystack.
func allSubstringOffsets(haystack, needle string) []int {
	if needle == "" {
		return nil
	}
	out := make([]int, 0)
	for start := 0; start <= len(haystack)-len(needle); {
		idx := strings.Index(haystack[start:], needle)
		if idx < 0 {
			break
		}
		out = append(out, start+idx)
		start += idx + len(needle)
	}
	return out
}

// caseFoldMatcher finds the case-insensitive matches of bin_yara_scan rules in
// one file.
//
// What matches: a rune of the rule matches a valid UTF-8 sequence in the file
// whose rune lowers, by unicode.ToLower (the mapping strings.ToLower applies), to
// the rune the rule's own rune lowers to. A byte of the rule that is not part of a
// valid UTF-8 sequence matches only that same byte, wherever it is in the file,
// so a rule matches at every offset where its own bytes are. A byte of the file
// that is not part of a valid sequence is never matched by a rune, not even by
// U+FFFD. A match can span a different number of bytes from the rule: the
// three-byte KELVIN SIGN (U+212A) lowers to, and so matches, "k".
//
// Every offset reported is a byte offset into the file. Matches do not overlap:
// the search for the next one starts where the last one ended, as in
// allSubstringOffsets.
type caseFoldMatcher struct {
	data    []byte
	lowered *foldedHaystack // built by the first rule that is valid UTF-8
}

// offsets returns the file offset of every non-overlapping match of rule, which
// must not be empty. A rule that is valid UTF-8 -- every text rule -- is found
// with bytes.Index in a lowered copy of the file, built once and shared by every
// such rule. A rule holding any other byte is walked against the file itself,
// because that byte has to be compared with the file's own byte, which the
// lowered copy does not always hold (a lowered rune is not the bytes it came
// from).
func (m *caseFoldMatcher) offsets(rule string) []int {
	if !utf8.ValidString(rule) {
		return caseFoldWalk(m.data, rule)
	}
	if m.lowered == nil {
		m.lowered = foldHaystack(m.data)
	}
	return m.lowered.offsets(strings.ToLower(rule))
}

// foldedHaystack is a file lowered for case-insensitive matching of rules that
// are valid UTF-8. Each valid UTF-8 sequence is replaced by the encoding of its
// rune lowered by unicode.ToLower, and each byte that is not part of one is kept
// as it is, where strings.ToLower would write the three bytes of U+FFFD.
//
// That makes a match in the lowered copy a match of whole lowered runes. A valid
// rule's first byte is never a continuation byte, so a match starts where a
// lowered rune or a kept byte starts. It never covers a kept byte: a lowered rune
// never starts with a continuation byte, so in the copy a kept byte is followed by
// continuation bytes only when the file had them right after it too, and a kept
// byte that began a valid sequence of the rule would have begun the same sequence
// in the file and been decoded with it, not kept. U+FFFD in a rule therefore only
// matches a U+FFFD that is in the file.
//
// What the copy does not keep is the offsets: a lowered rune can be longer or
// shorter than the original (U+212A, three bytes, lowers to the one-byte "k";
// U+023A, two bytes, lowers to the three-byte U+2C65). So the copy carries marks,
// pairs of positions known to line up, and a position is mapped back to the file
// by walking forward, a rune or a byte at a time, from the nearest point at or
// before it where the two line up. Marks are spaced by the file, not by how many
// runes change length, so what they cost stays a small fraction of the file
// however it was written.
type foldedHaystack struct {
	data    []byte
	lowered []byte
	// marks[k] is where the first rune or byte to start at or after file
	// offset k*foldMarkStride starts in both.
	marks []foldMark
}

// foldMark is a place where the file and its lowered copy line up: the rune or
// byte that starts at offset file starts at offset lowered of the copy.
type foldMark struct {
	file    int
	lowered int
}

// foldMarkStride is how many bytes of the file lie between two marks of a
// foldedHaystack: about the most that mapping one match back to the file walks.
// The marks cost two ints per stride of the file.
const foldMarkStride = 1024

func foldHaystack(data []byte) *foldedHaystack {
	h := &foldedHaystack{data: data, lowered: make([]byte, 0, len(data))}
	for i := 0; i < len(data); {
		if i >= len(h.marks)*foldMarkStride {
			h.marks = append(h.marks, foldMark{file: i, lowered: len(h.lowered)})
		}
		lower, width, valid := foldUnit(data, i)
		if valid {
			h.lowered = utf8.AppendRune(h.lowered, lower)
		} else {
			h.lowered = append(h.lowered, data[i])
		}
		i += width
	}
	return h
}

// foldUnit reads the rune or byte of data that starts at offset i: a valid UTF-8
// sequence, which it returns lowered by unicode.ToLower with valid set, or one byte
// that is not part of a valid sequence, which is kept as it is (valid unset). A
// U+FFFD written in the file is a valid sequence like any other, lowered to
// itself. width is how many bytes of data it took up.
func foldUnit(data []byte, i int) (lower rune, width int, valid bool) {
	if c := data[i]; c < utf8.RuneSelf {
		if 'A' <= c && c <= 'Z' {
			c += 'a' - 'A'
		}
		return rune(c), 1, true
	}
	r, width := utf8.DecodeRune(data[i:])
	if r == utf8.RuneError && width == 1 {
		return 0, 1, false
	}
	return unicode.ToLower(r), width, true
}

// offsets returns the file offset of every non-overlapping match of rule, a
// non-empty valid UTF-8 string already lowered by strings.ToLower.
func (h *foldedHaystack) offsets(rule string) []int {
	needle := []byte(rule)
	out := make([]int, 0)
	var at foldMark // where the last match was mapped; the start lines up too
	for start := 0; start <= len(h.lowered)-len(needle); {
		idx := bytes.Index(h.lowered[start:], needle)
		if idx < 0 {
			break
		}
		at = h.walkTo(at, start+idx)
		out = append(out, at.file)
		start += idx + len(needle)
	}
	return out
}

// walkTo maps pos, a position of the lowered copy at which a lowered rune or a
// kept byte starts, back to the file: it walks forward from from, a point at or
// before pos where the two line up, or from the last mark at or before pos when
// that is nearer.
func (h *foldedHaystack) walkTo(from foldMark, pos int) foldMark {
	k := sort.Search(len(h.marks), func(k int) bool { return h.marks[k].lowered > pos }) - 1
	if k >= 0 && h.marks[k].lowered > from.lowered {
		from = h.marks[k]
	}
	for from.lowered < pos {
		lower, width, valid := foldUnit(h.data, from.file)
		from.file += width
		if valid {
			from.lowered += utf8.RuneLen(lower)
		} else {
			from.lowered++
		}
	}
	return from
}

// caseFoldWalk returns the file offset of every non-overlapping
// caseFoldMatcher match of rule, a non-empty string that is not valid UTF-8,
// trying each offset of data in turn.
func caseFoldWalk(data []byte, rule string) []int {
	first, firstWidth := utf8.DecodeRuneInString(rule)
	firstIsByte := first == utf8.RuneError && firstWidth == 1
	out := make([]int, 0)
	for at := 0; at < len(data); {
		if firstIsByte {
			// The rule starts with a byte that matches only itself, so no
			// match starts anywhere else.
			idx := bytes.IndexByte(data[at:], rule[0])
			if idx < 0 {
				break
			}
			at += idx
		}
		if end, ok := caseFoldMatchAt(data, at, rule); ok {
			out = append(out, at)
			at = end
			continue
		}
		at++
	}
	return out
}

// caseFoldMatchAt reports whether rule matches data at offset at, and the offset
// in data at which the match ends.
func caseFoldMatchAt(data []byte, at int, rule string) (int, bool) {
	pos := at
	for k := 0; k < len(rule); {
		if pos >= len(data) {
			return 0, false
		}
		r, width := utf8.DecodeRuneInString(rule[k:])
		if r == utf8.RuneError && width == 1 {
			if data[pos] != rule[k] {
				return 0, false
			}
			pos++
			k++
			continue
		}
		lower, dataWidth, valid := foldUnit(data, pos)
		if !valid || lower != unicode.ToLower(r) {
			return 0, false
		}
		pos += dataWidth
		k += width
	}
	return pos, true
}

func BinImports(args ...object.Object) object.Object {
	if len(args) != 1 {
		return resultAndError(nil, newError("wrong number of arguments. got=%d, want=1", len(args)))
	}

	pathObj, ok := args[0].(*object.String)
	if !ok {
		return resultAndError(nil, newError("argument 1 to `bin_imports` must be STRING, got %s", args[0].Type()))
	}

	format, imports, errObj := loadImports(pathObj.Value)
	if errObj != nil {
		return resultAndError(nil, errObj)
	}

	sort.Strings(imports)
	imports = uniqueStrings(imports)
	elements := make([]object.Object, len(imports))
	for i, v := range imports {
		elements[i] = stringObj(v)
	}

	return resultAndError(makeHashObject(map[string]object.Object{
		"path":    stringObj(pathObj.Value),
		"format":  stringObj(format),
		"imports": &object.Array{Elements: elements},
	}), nil)
}

func BinSections(args ...object.Object) object.Object {
	if len(args) != 1 {
		return resultAndError(nil, newError("wrong number of arguments. got=%d, want=1", len(args)))
	}

	pathObj, ok := args[0].(*object.String)
	if !ok {
		return resultAndError(nil, newError("argument 1 to `bin_sections` must be STRING, got %s", args[0].Type()))
	}

	format, sections, errObj := loadSections(pathObj.Value)
	if errObj != nil {
		return resultAndError(nil, errObj)
	}

	return resultAndError(makeHashObject(map[string]object.Object{
		"path":     stringObj(pathObj.Value),
		"format":   stringObj(format),
		"sections": &object.Array{Elements: sections},
	}), nil)
}

func loadDWARF(path string) (string, *dwarf.Data, *object.Error) {
	if peFile, err := pe.Open(path); err == nil {
		defer peFile.Close()
		data, derr := peFile.DWARF()
		if derr != nil {
			return "pe", nil, newError("bin_dwarf_parse: %s", derr.Error())
		}
		return "pe", data, nil
	}
	if elfFile, err := elf.Open(path); err == nil {
		defer elfFile.Close()
		data, derr := elfFile.DWARF()
		if derr != nil {
			return "elf", nil, newError("bin_dwarf_parse: %s", derr.Error())
		}
		return "elf", data, nil
	}
	return "", nil, newError("bin_dwarf_parse: unsupported or invalid binary format")
}

func loadImports(path string) (string, []string, *object.Error) {
	if peFile, err := saferpe.New(path, &saferpe.Options{Fast: false}); err == nil {
		defer peFile.Close()
		if parseErr := peFile.Parse(); parseErr == nil {
			all := make([]string, 0)
			for _, imp := range peFile.Imports {
				if imp.Name != "" {
					all = append(all, imp.Name)
				}
				for _, fn := range imp.Functions {
					if fn.Name != "" {
						all = append(all, fn.Name)
					}
				}
			}
			return "pe", all, nil
		}
	}

	if peFile, err := pe.Open(path); err == nil {
		defer peFile.Close()
		imports, ierr := peFile.ImportedSymbols()
		if ierr != nil {
			return "pe", nil, newError("bin_imports: %s", ierr.Error())
		}
		return "pe", imports, nil
	}
	if elfFile, err := elf.Open(path); err == nil {
		defer elfFile.Close()
		libs, lerr := elfFile.ImportedLibraries()
		if lerr != nil {
			return "elf", nil, newError("bin_imports: %s", lerr.Error())
		}
		syms, serr := elfFile.ImportedSymbols()
		if serr != nil {
			return "elf", nil, newError("bin_imports: %s", serr.Error())
		}
		all := make([]string, 0, len(libs)+len(syms))
		all = append(all, libs...)
		for _, sym := range syms {
			all = append(all, sym.Name)
		}
		return "elf", all, nil
	}
	return "", nil, newError("bin_imports: unsupported or invalid binary format")
}

func loadSections(path string) (string, []object.Object, *object.Error) {
	if peFile, err := saferpe.New(path, &saferpe.Options{Fast: true}); err == nil {
		defer peFile.Close()
		if parseErr := peFile.Parse(); parseErr == nil {
			sections := make([]object.Object, 0, len(peFile.Sections))
			for _, sec := range peFile.Sections {
				name := strings.TrimRight(string(sec.Header.Name[:]), "\x00")
				name = strings.TrimSpace(name)
				if name == "" {
					name = string(bytes.Trim(sec.Header.Name[:], "\x00"))
				}
				sections = append(sections, makeHashObject(map[string]object.Object{
					"name": stringObj(name),
					"size": intObj(int64(sec.Header.SizeOfRawData)),
					"addr": intObj(int64(sec.Header.VirtualAddress)),
				}))
			}
			return "pe", sections, nil
		}
	}

	if peFile, err := pe.Open(path); err == nil {
		defer peFile.Close()
		sections := make([]object.Object, 0, len(peFile.Sections))
		for _, sec := range peFile.Sections {
			sections = append(sections, makeHashObject(map[string]object.Object{
				"name": stringObj(strings.TrimSpace(sec.Name)),
				"size": intObj(int64(sec.Size)),
				"addr": intObj(int64(sec.VirtualAddress)),
			}))
		}
		return "pe", sections, nil
	}
	if elfFile, err := elf.Open(path); err == nil {
		defer elfFile.Close()
		sections := make([]object.Object, 0, len(elfFile.Sections))
		for _, sec := range elfFile.Sections {
			sections = append(sections, makeHashObject(map[string]object.Object{
				"name": stringObj(sec.Name),
				"size": intObj(int64(sec.Size)),
				"addr": intObj(int64(sec.Addr)),
			}))
		}
		return "elf", sections, nil
	}
	return "", nil, newError("bin_sections: unsupported or invalid binary format")
}

func uniqueStrings(values []string) []string {
	if len(values) == 0 {
		return values
	}
	out := make([]string, 0, len(values))
	last := ""
	for i, v := range values {
		if i == 0 || v != last {
			out = append(out, v)
			last = v
		}
	}
	return out
}
