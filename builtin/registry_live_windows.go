//go:build windows

package builtin

import (
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"golang.org/x/sys/windows/registry"

	"mutant/object"
)

// liveRegistryBackend reads the live Windows registry via x/sys/windows/registry
// (pure-Go, no cgo). The backend is opened at a root+base key; enum/get paths are
// relative to that base.
type liveRegistryBackend struct {
	root registry.Key
	base string
}

func openLiveRegistry(spec string) (registryBackend, error) {
	root, base, err := parseLiveRegistrySpec(spec)
	if err != nil {
		return nil, err
	}
	// Validate the key opens now so reg_open fails fast on a bad path.
	k, err := registry.OpenKey(root, base, registry.QUERY_VALUE|registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		return nil, fmt.Errorf("cannot open registry key %q: %w", spec, err)
	}
	k.Close()
	return &liveRegistryBackend{root: root, base: base}, nil
}

var liveRegistryRoots = map[string]registry.Key{
	"HKLM": registry.LOCAL_MACHINE, "HKEY_LOCAL_MACHINE": registry.LOCAL_MACHINE,
	"HKCU": registry.CURRENT_USER, "HKEY_CURRENT_USER": registry.CURRENT_USER,
	"HKCR": registry.CLASSES_ROOT, "HKEY_CLASSES_ROOT": registry.CLASSES_ROOT,
	"HKU": registry.USERS, "HKEY_USERS": registry.USERS,
	"HKCC": registry.CURRENT_CONFIG, "HKEY_CURRENT_CONFIG": registry.CURRENT_CONFIG,
}

func parseLiveRegistrySpec(spec string) (registry.Key, string, error) {
	spec = strings.Trim(strings.ReplaceAll(spec, "/", `\`), `\`)
	parts := strings.SplitN(spec, `\`, 2)
	root, ok := liveRegistryRoots[strings.ToUpper(parts[0])]
	if !ok {
		return 0, "", fmt.Errorf("unknown registry root %q (use HKLM/HKCU/HKCR/HKU/HKCC)", parts[0])
	}
	base := ""
	if len(parts) == 2 {
		base = parts[1]
	}
	return root, base, nil
}

func (b *liveRegistryBackend) sourceType() string { return "live" }

func (b *liveRegistryBackend) subPath(path string) string {
	sub := strings.Trim(strings.ReplaceAll(path, "/", `\`), `\`)
	switch {
	case b.base == "":
		return sub
	case sub == "":
		return b.base
	default:
		return b.base + `\` + sub
	}
}

func (b *liveRegistryBackend) enumKeys(path string) ([]string, error) {
	k, err := registry.OpenKey(b.root, b.subPath(path), registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		return nil, err
	}
	defer k.Close()
	names, err := k.ReadSubKeyNames(-1)
	if err != nil {
		return nil, err
	}
	sort.Strings(names)
	return names, nil
}

func (b *liveRegistryBackend) enumValues(path string) ([]regEntry, error) {
	k, err := registry.OpenKey(b.root, b.subPath(path), registry.QUERY_VALUE)
	if err != nil {
		return nil, err
	}
	defer k.Close()
	names, err := k.ReadValueNames(-1)
	if err != nil {
		return nil, err
	}
	sort.Strings(names)
	entries := make([]regEntry, 0, len(names))
	for _, name := range names {
		// A name the OS has just listed can still fail to read: on a live system
		// a value can be deleted or retyped between the enumeration and the
		// read. That fails the call rather than dropping the value, because a
		// list presented as complete while silently short is M26-NET-004 one
		// level up.
		entry, err := readLiveRegistryValue(k, name)
		if err != nil {
			return nil, fmt.Errorf("reading value %q: %w", name, err)
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func (b *liveRegistryBackend) getValue(path, name string) (regEntry, error) {
	k, err := registry.OpenKey(b.root, b.subPath(path), registry.QUERY_VALUE)
	if err != nil {
		return regEntry{}, err
	}
	defer k.Close()
	entry, err := readLiveRegistryValue(k, name)
	if err == registry.ErrNotExist {
		return regEntry{}, errRegValueNotFound(name)
	}
	if err != nil {
		return regEntry{}, err
	}
	return entry, nil
}

// readLiveRegistryValue reads one value, or says why it could not.
//
// It used to return a bare regEntry and map every GetValue error -- including
// ERROR_FILE_NOT_FOUND for a value that is simply not there -- to an entry of
// type REG_NONE with empty data. getValue then handed that back with a nil
// error, so the live backend never said "not found" while the JSON and hive
// backends did, and absence was indistinguishable from a real present REG_NONE
// value, which does exist (M26-NET-004).
//
// The typed getters' errors were dropped too, with `s, _, _ :=`. Each getter is
// called only for a type it accepts -- GetStringValue takes SZ and EXPAND_SZ,
// GetIntegerValue DWORD and QWORD -- so an error from one of them is not a type
// mistake: it is the value being deleted or retyped between the call that read
// its type and the call that reads its data. That is a real failure on a live
// system and it now travels.
//
// A nil buffer is what makes the first call cheap: RegQueryValueEx with no
// output buffer reports the type and the size it would need. ErrShortBuffer is
// still tolerated, as before, rather than relied on not to happen. Only the
// default branch uses that size, to read the bytes of a type it does not
// decode; the typed getters size their own buffers.
func readLiveRegistryValue(k registry.Key, name string) (regEntry, error) {
	displayName := name
	if displayName == "" {
		displayName = "(default)"
	}
	size, valType, err := k.GetValue(name, nil)
	if err != nil && err != registry.ErrShortBuffer {
		return regEntry{}, err
	}
	switch valType {
	case registry.SZ, registry.EXPAND_SZ:
		s, _, err := k.GetStringValue(name)
		if err != nil {
			return regEntry{}, err
		}
		typ := "REG_SZ"
		if valType == registry.EXPAND_SZ {
			typ = "REG_EXPAND_SZ"
		}
		return regEntry{name: displayName, typ: typ, data: stringObj(s)}, nil
	case registry.DWORD:
		n, _, err := k.GetIntegerValue(name)
		if err != nil {
			return regEntry{}, err
		}
		return regEntry{name: displayName, typ: "REG_DWORD", data: intObj(int64(n))}, nil
	case registry.QWORD:
		n, _, err := k.GetIntegerValue(name)
		if err != nil {
			return regEntry{}, err
		}
		return regEntry{name: displayName, typ: "REG_QWORD", data: intObj(int64(n))}, nil
	case registry.MULTI_SZ:
		ss, _, err := k.GetStringsValue(name)
		if err != nil {
			return regEntry{}, err
		}
		elems := make([]object.Object, len(ss))
		for i, s := range ss {
			elems[i] = stringObj(s)
		}
		return regEntry{name: displayName, typ: "REG_MULTI_SZ", data: &object.Array{Elements: elems}}, nil
	case registry.BINARY:
		// GetBinaryValue allocates a fresh buffer per call, so raw is owned already
		// and needs no clone the way the regf backend's window into the hive does.
		buf, _, err := k.GetBinaryValue(name)
		if err != nil {
			return regEntry{}, err
		}
		return regEntry{name: displayName, typ: "REG_BINARY", data: stringObj(hex.EncodeToString(buf)), raw: buf}, nil
	default:
		// Every other type -- REG_NONE, REG_DWORD_BIG_ENDIAN, REG_LINK, the three
		// resource descriptor types, and numbers no Windows header names -- comes
		// back as its stored bytes: hex in data, and the same bytes in raw, which
		// regEntryHash reports as data_bytes. That is what reg_get_value and
		// reg_enum_values document for a type the reader does not recognise, and
		// what the regf backend does with each type it does not decode itself.
		//
		// This branch used to answer data "" with no bytes, so a 140-byte
		// REG_RESOURCE_LIST read as an empty value (M26-NET-006).
		raw, err := readLiveRegistryRaw(k, name, valType, size)
		if err != nil {
			return regEntry{}, err
		}
		return regEntry{name: displayName, typ: fmt.Sprintf("REG_TYPE(%d)", valType), data: stringObj(hex.EncodeToString(raw)), raw: raw}, nil
	}
}

// readLiveRegistryRaw reads the stored bytes of a value whose type the caller
// has already read as valType, and whose length that same read reported as
// size.
//
// The value can change between the two reads. Deleted, it comes back as
// registry.ErrNotExist, which getValue reports as not found, as it does when a
// typed getter meets the same race. Retyped, it is refused, because its bytes
// would be reported under a type that is no longer its own. Grown past size, it
// no longer fits and nothing is read, so that is refused too, naming both
// lengths. A value that shrank still fits, and RegQueryValueEx reports its new
// length.
//
// The buffer is allocated here and handed on as raw, so, as with
// GetBinaryValue's, nothing else holds it. An empty value yields an empty
// slice and not nil, so data_bytes is present and empty, as it is for a
// zero-length REG_BINARY on this backend.
func readLiveRegistryRaw(k registry.Key, name string, valType uint32, size int) ([]byte, error) {
	buf := make([]byte, size)
	n, typ, err := k.GetValue(name, buf)
	// With an empty buffer RegQueryValueEx only reports the length, without an
	// error, so a value that was empty and no longer is shows up as n > len(buf).
	if err == registry.ErrShortBuffer || (err == nil && n > len(buf)) {
		return nil, fmt.Errorf("value grew from %d to %d bytes while it was being read", size, n)
	}
	if err != nil {
		return nil, err
	}
	if typ != valType {
		return nil, fmt.Errorf("value changed type from %d to %d while it was being read", valType, typ)
	}
	return buf[:n], nil
}

func (b *liveRegistryBackend) deletedKeys() []string     { return nil }
func (b *liveRegistryBackend) timeline() []object.Object { return nil }
func (b *liveRegistryBackend) close()                    {}
