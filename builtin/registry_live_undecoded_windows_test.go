//go:build windows

package builtin

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"os"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"mutant/object"
)

// M26-NET-006. readLiveRegistryValue reads six value types itself -- REG_SZ,
// REG_EXPAND_SZ, REG_DWORD, REG_QWORD, REG_MULTI_SZ and REG_BINARY -- and
// answered every other type with data "" and no bytes:
//
//	default:
//		return regEntry{name: displayName, typ: fmt.Sprintf("REG_TYPE(%d)", valType), data: stringObj("")}, nil
//
// reg_get_value and reg_enum_values document the opposite: a value of a type
// the reader does not recognise carries its stored bytes, as hex in data and
// as a buffer in data_bytes. On a live machine the 140-byte REG_RESOURCE_LIST
// HKLM\HARDWARE\RESOURCEMAP\System Resources\Physical Memory\.Translated read
// as {type: REG_TYPE(8), data: ""} -- an empty value, which is a claim about
// the value and not an admission that it was not read.
//
// x/sys/windows/registry has no setter for these types (each Set*Value writes
// a type it knows), so the fixture calls RegSetValueExW itself. The key is the
// test's own, under HKCU, with this process's id in its name so that two test
// binaries running at once cannot delete each other's values, and Cleanup
// removes it, including when the test fails.

// liveRegUndecodedValue is one value of a type the live reader does not decode.
type liveRegUndecodedValue struct {
	name string
	typ  uint32
	data []byte
}

// liveRegUndecodedValues covers each kind of type the reader leaves undecoded:
// a resource list the size of the one the row was found on, and larger than the
// 64-byte buffer x/sys's own getters start from; REG_NONE with bytes and
// without; the two types the regf backend decodes and this reader does not; and
// a type number no Windows header names.
func liveRegUndecodedValues() []liveRegUndecodedValue {
	resourceList := make([]byte, 140)
	for i := range resourceList {
		resourceList[i] = byte(i*37 + 11)
	}
	return []liveRegUndecodedValue{
		{name: "ResourceList", typ: registry.RESOURCE_LIST, data: resourceList},
		{name: "NoneWithBytes", typ: registry.NONE, data: []byte{0xDE, 0xAD, 0xBE, 0xEF, 0x00, 0xFF}},
		{name: "NoneEmpty", typ: registry.NONE, data: []byte{}},
		{name: "BigEndianDword", typ: registry.DWORD_BIG_ENDIAN, data: []byte{0x00, 0x00, 0x01, 0x02}},
		{name: "LinkTyped", typ: registry.LINK, data: []byte{'\\', 0, 'R', 0, 'e', 0, 'g', 0}},
		{name: "UnnamedType", typ: 0x7F, data: []byte{0x4D, 0x5A, 0x90, 0x00, 0xFF}},
	}
}

var liveRegSetValueExW = windows.NewLazySystemDLL("advapi32.dll").NewProc("RegSetValueExW")

// setLiveRegRawValue writes data under name with the given type, which
// x/sys/windows/registry cannot do for a type it has no setter for.
func setLiveRegRawValue(t *testing.T, k registry.Key, name string, typ uint32, data []byte) {
	t.Helper()

	pname, err := windows.UTF16PtrFromString(name)
	if err != nil {
		t.Fatalf("UTF16PtrFromString(%q): %v", name, err)
	}
	var pdata *byte
	if len(data) > 0 {
		pdata = &data[0]
	}
	r, _, _ := liveRegSetValueExW.Call(uintptr(k), uintptr(unsafe.Pointer(pname)), 0,
		uintptr(typ), uintptr(unsafe.Pointer(pdata)), uintptr(len(data)))
	if r != 0 {
		t.Fatalf("RegSetValueExW(%q, type %d): %v", name, typ, windows.Errno(r))
	}
}

// liveRegUndecodedFixture creates this test's key, writes every value in
// liveRegUndecodedValues and one REG_SZ and one REG_DWORD beside them, and
// returns the spec to open the key with.
func liveRegUndecodedFixture(t *testing.T) string {
	t.Helper()

	sub := fmt.Sprintf(`Software\MutantReview-M26-NET-006-%d`, os.Getpid())
	k, _, err := registry.CreateKey(registry.CURRENT_USER, sub, registry.ALL_ACCESS)
	if err != nil {
		t.Skipf("cannot create HKCU\\%s, so the live backend cannot be tested here: %v", sub, err)
	}
	t.Cleanup(func() {
		_ = k.Close()
		_ = registry.DeleteKey(registry.CURRENT_USER, sub)
	})

	for _, v := range liveRegUndecodedValues() {
		setLiveRegRawValue(t, k, v.name, v.typ, v.data)
	}
	if err := k.SetStringValue("Text", "decoded already"); err != nil {
		t.Fatalf("SetStringValue: %v", err)
	}
	if err := k.SetDWordValue("Count", 42); err != nil {
		t.Fatalf("SetDWordValue: %v", err)
	}
	return `HKCU\` + sub
}

// TestTheLiveRegistryKeepsTheBytesOfATypeItDoesNotDecode is the row, read
// through the backend: each undecoded value comes back as its stored bytes,
// hex in data and the same bytes in raw, which regEntryHash reports as
// data_bytes. The type keeps the backend's existing spelling, REG_TYPE(n), and
// has to name the value's own type.
func TestTheLiveRegistryKeepsTheBytesOfATypeItDoesNotDecode(t *testing.T) {
	backend, err := openLiveRegistry(liveRegUndecodedFixture(t))
	if err != nil {
		t.Fatalf("openLiveRegistry: %v", err)
	}
	defer backend.close()

	for _, want := range liveRegUndecodedValues() {
		t.Run(want.name, func(t *testing.T) {
			entry, err := backend.getValue("", want.name)
			if err != nil {
				t.Fatalf("getValue: %v", err)
			}
			if wantType := fmt.Sprintf("REG_TYPE(%d)", want.typ); entry.typ != wantType {
				t.Errorf("type = %q, want %q", entry.typ, wantType)
			}
			str, ok := entry.data.(*object.String)
			if !ok {
				t.Fatalf("data is %T, want a STRING of hex", entry.data)
			}
			if wantHex := hex.EncodeToString(want.data); str.Value != wantHex {
				t.Errorf("data = %q, want the stored bytes as hex, %q", str.Value, wantHex)
			}
			if entry.raw == nil {
				t.Fatalf("no bytes, so no data_bytes: a %d-byte value read as {type: %s, data: %q}",
					len(want.data), entry.typ, str.Value)
			}
			if !bytes.Equal(entry.raw, want.data) {
				t.Errorf("raw = %x, want %x", entry.raw, want.data)
			}
		})
	}
}

// TestRegReadersOnTheLiveRegistryCarryDataBytesForUndecodedTypes reads the same
// values through the builtins, because the contract the row cites is theirs:
// reg_get_value hands back data_bytes for a live REG_RESOURCE_LIST, holding
// every stored byte, and reg_enum_values does the same for each undecoded
// value it lists. A REG_SZ and a REG_DWORD still omit the field, which is the
// guard against the over-correction of attaching bytes to every value.
func TestRegReadersOnTheLiveRegistryCarryDataBytesForUndecodedTypes(t *testing.T) {
	payload, errObj := unwrapPair(t, RegOpen(stringObj(liveRegUndecodedFixture(t))))
	if errObj != nil {
		t.Fatalf("reg_open: %s", errObj.Inspect())
	}
	handle := regMustHashString(t, regMustHash(t, payload), "handle")
	t.Cleanup(func() { RegClose(stringObj(handle)) })

	values := liveRegUndecodedValues()
	resourceList := values[0]

	payload, errObj = unwrapPair(t, RegGetValue(stringObj(handle), stringObj(""), stringObj(resourceList.name)))
	if errObj != nil {
		t.Fatalf("reg_get_value: %s", errObj.Inspect())
	}
	value := regMustHash(t, payload)
	buf := mustDataBytes(t, value)
	if len(buf.Value) != len(resourceList.data) || !bytes.Equal(buf.Value, resourceList.data) {
		t.Errorf("reg_get_value data_bytes holds %d bytes %x, want the %d stored bytes %x",
			len(buf.Value), buf.Value, len(resourceList.data), resourceList.data)
	}
	if got, want := hex.EncodeToString(buf.Value), regMustHashString(t, value, "data"); got != want {
		t.Errorf("hex(data_bytes) = %s, but data = %s", got, want)
	}

	payload, errObj = unwrapPair(t, RegEnumValues(stringObj(handle)))
	if errObj != nil {
		t.Fatalf("reg_enum_values: %s", errObj.Inspect())
	}
	listed := map[string]*object.Hash{}
	for _, element := range payload.(*object.Array).Elements {
		h := regMustHash(t, element)
		listed[regMustHashString(t, h, "name")] = h
	}

	for _, want := range values {
		h, ok := listed[want.name]
		if !ok {
			t.Errorf("value %q was written but not listed", want.name)
			continue
		}
		field, present := optionalRegField(h, "data_bytes")
		if !present {
			t.Errorf("reg_enum_values: %s carries no data_bytes: %s", want.name, h.Inspect())
			continue
		}
		got, ok := field.(*object.Bytes)
		if !ok {
			t.Errorf("reg_enum_values: %s data_bytes is %s, want BYTES", want.name, field.Type())
			continue
		}
		if !bytes.Equal(got.Value, want.data) {
			t.Errorf("reg_enum_values: %s data_bytes = %x, want %x", want.name, got.Value, want.data)
		}
	}

	for _, name := range []string{"Text", "Count"} {
		h, ok := listed[name]
		if !ok {
			t.Errorf("value %q was written but not listed", name)
			continue
		}
		if _, present := optionalRegField(h, "data_bytes"); present {
			t.Errorf("reg_enum_values: %s, a type already decoded, carries data_bytes: %s", name, h.Inspect())
		}
	}
}
