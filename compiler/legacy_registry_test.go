package compiler

import (
	"bytes"
	"encoding/gob"
	"reflect"
	"strings"
	"testing"

	"mutant/ast"
	"mutant/code"
	"mutant/object"
)

// v23ByteCodeStream and v24ByteCodeStream gob-encode a program through ByteCode
// as v2.3.0 and v2.4.0 declared it. gob names a struct type by its Go name, so
// each is declared as ByteCode inside the function that encodes it.
func v23ByteCodeStream(t *testing.T) []byte {
	t.Helper()
	type ByteCode struct {
		Instructions code.Instructions
		Constants    []object.Object
		StructDefs   map[string][]*ast.Identifier
		EnumDefs     map[string][]string
		LuaPatches   map[string]*object.LuaPatch
	}
	return gobStream(t, &ByteCode{Instructions: code.Make(code.OpGetBuiltin, 398)})
}

func v24ByteCodeStream(t *testing.T, opcodeMap []byte) []byte {
	t.Helper()
	type ByteCode struct {
		Instructions code.Instructions
		Constants    []object.Object
		StructDefs   map[string][]*ast.Identifier
		EnumDefs     map[string][]string
		LuaPatches   map[string]*object.LuaPatch
		OpcodeMap    []byte
	}
	return gobStream(t, &ByteCode{Instructions: code.Make(code.OpGetBuiltin, 398), OpcodeMap: opcodeMap})
}

func gobStream(t *testing.T, value any) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(value); err != nil {
		t.Fatalf("gob: %v", err)
	}
	return buf.Bytes()
}

// firstGobMessageEnd is where the first message of a gob stream ends: its byte
// count, read the way gobLeadingStruct reads it, plus the bytes that count
// took.
func firstGobMessageEnd(t *testing.T, stream []byte) int {
	t.Helper()
	r := gobReader{buf: stream}
	size := r.readUint()
	if r.err != nil {
		t.Fatalf("reading the first message's length: %v", r.err)
	}
	return r.off + int(size)
}

func TestLegacyRegistryOfReadsTheReleaseFromTheDeclaredType(t *testing.T) {
	identity := make([]byte, 256)
	for i := range identity {
		identity[i] = byte(i)
	}
	for _, c := range []struct {
		name   string
		stream []byte
		want   LegacyRegistry
	}{
		{"v2.3.0's ByteCode", v23ByteCodeStream(t), LegacyRegistryV23},
		// The case a decoded value cannot answer: a v2.4.0 program built at
		// mutation level 0 has no opcode map, and its type declares one anyway.
		{"v2.4.0's ByteCode without an opcode map", v24ByteCodeStream(t, nil), LegacyRegistryV24},
		{"v2.4.0's ByteCode with an opcode map", v24ByteCodeStream(t, identity), LegacyRegistryV24},
		{"this release's ByteCode", gobStream(t, &ByteCode{Instructions: code.Make(code.OpPop)}), LegacyRegistryV24},
	} {
		got, err := LegacyRegistryOf(c.stream)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if got != c.want {
			t.Errorf("%s: LegacyRegistryOf = %d, want %d", c.name, got, c.want)
		}
	}
}

// TestGobLeadingStructReadsEveryField pins the reader to gob's own encoder
// rather than to a hand-assembled byte string that could share the reader's
// mistakes: over this release's ByteCode, it must name every field gob sends --
// the exported ones -- in declaration order, and none it does not.
func TestGobLeadingStructReadsEveryField(t *testing.T) {
	name, fields, err := gobLeadingStruct(gobStream(t, &ByteCode{Version: BytecodeVersion}))
	if err != nil {
		t.Fatal(err)
	}
	if name != "ByteCode" {
		t.Fatalf("name = %q, want ByteCode", name)
	}
	var want []string
	typ := reflect.TypeOf(ByteCode{})
	for i := 0; i < typ.NumField(); i++ {
		if typ.Field(i).IsExported() {
			want = append(want, typ.Field(i).Name)
		}
	}
	if strings.Join(fields, " ") != strings.Join(want, " ") {
		t.Fatalf("fields = %v\nwant     %v", fields, want)
	}
}

// TestLegacyRegistryOfRefusesWhatItCannotRead: a stream that does not open with
// ByteCode's type says nothing about which registry its ordinals index, so it
// is an error and never a default.
func TestLegacyRegistryOfRefusesWhatItCannotRead(t *testing.T) {
	type Program struct {
		Instructions code.Instructions
	}
	for _, c := range []struct {
		name   string
		stream []byte
		want   string
	}{
		{"an empty stream", nil, "ends inside"},
		{"a value with no type definition ahead of it", gobStream(t, 7), "does not begin with a type definition"},
		{"a map type", gobStream(t, map[string]int{"a": 1}), "not a struct"},
		{"a struct that is not ByteCode", gobStream(t, &Program{Instructions: code.Make(code.OpPop)}), `"Program"`},
	} {
		_, err := LegacyRegistryOf(c.stream)
		if err == nil {
			t.Errorf("%s: LegacyRegistryOf answered", c.name)
			continue
		}
		if !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: error %q does not say %q", c.name, err, c.want)
		}
	}

	// Every cut short of the first message's end is a truncation, never an
	// answer and never a panic.
	stream := v23ByteCodeStream(t)
	end := firstGobMessageEnd(t, stream)
	for n := 0; n < end; n++ {
		if registry, err := LegacyRegistryOf(stream[:n]); err == nil {
			t.Fatalf("the first %d of %d bytes of the leading message answered %d", n, end, registry)
		}
	}
	if _, err := LegacyRegistryOf(stream[:end]); err != nil {
		t.Fatalf("the leading message alone, with nothing after it, should be enough: %v", err)
	}
}

// TestGobLeadingStructSurvivesCorruptBytes overwrites each byte of the leading
// message with the values most likely to derail a length or a field number,
// and requires an answer or an error -- never a panic, never a hang.
func TestGobLeadingStructSurvivesCorruptBytes(t *testing.T) {
	stream := v24ByteCodeStream(t, nil)
	end := firstGobMessageEnd(t, stream)
	for i := 0; i < end; i++ {
		for _, b := range []byte{0x00, 0x01, 0x7f, 0x80, 0xf8, 0xff} {
			corrupt := append([]byte(nil), stream[:end]...)
			corrupt[i] = b
			_, _, _ = gobLeadingStruct(corrupt)
		}
	}
}
