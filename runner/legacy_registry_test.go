package runner

// M26-BLT-014. A program compiled before v2.5.0 calls each builtin by its
// ordinal in the registry of the release that compiled it, and v2.3.x and
// v2.4.x disagree about one: help was appended to the registry last, so it was
// 398 in v2.3.x and 408 in v2.4.x, whose ten new builtins put pmap at 398.
// Neither release wrote ByteCode.Version, and the runtime resolved every
// versionless artifact through v2.4.0's table -- so a v2.3.0 artifact's
// help("len") ran pmap("len") and came back as pmap's arity error.
//
// These artifacts have the shape each release's had: the program goes
// through the ordinary compiler, its builtin operands are rewritten to that
// release's ordinals, and it is gob-encoded through a struct type named
// ByteCode holding exactly the fields that release's ByteCode had -- which is
// what tells the two apart, since gob declares a value's type ahead of it.
// Then it is sealed as generator.encode seals a program and handed to decode,
// the function every .mu passes through.

import (
	"bytes"
	"encoding/gob"
	"strings"
	"testing"

	"mutant/ast"
	"mutant/code"
	"mutant/compiler"
	"mutant/global"
	"mutant/lexer"
	"mutant/mutil"
	"mutant/object"
	"mutant/parser"
	"mutant/security"
	"mutant/serialize"
	"mutant/vm"

	"github.com/klauspost/compress/zstd"
)

const legacyArtifactPassword = "a password for this test's own legacy artifacts"

// legacyProgram compiles src and rewrites its one builtin operand to the
// registry ordinal a pre-v2.5 release gave that builtin, then encrypts it as the
// generator does. The program must call exactly one builtin.
func legacyProgram(t *testing.T, src string, ordinal int) *compiler.ByteCode {
	t.Helper()
	p := parser.New(lexer.New(src))
	program := p.ParseProgram()
	if errs := p.Errors(); len(errs) > 0 {
		t.Fatalf("parsing %q: %v", src, errs)
	}
	comp := compiler.New()
	// As the generator compiles: the runner's VM refuses a program without the
	// injected security checks.
	comp.EnableSecurityOpcodeInjection()
	comp.SetSecurityCheckSeed(1)
	if err := comp.Compile(program); err != nil {
		t.Fatalf("compiling %q: %v", src, err)
	}
	bc := comp.ByteCode()
	if len(bc.BuiltinNames) != 1 {
		t.Fatalf("%q references the builtins %v; these tests rewrite exactly one", src, bc.BuiltinNames)
	}

	rewritten := 0
	ins := bc.Instructions
	for ip := 0; ip < len(ins); {
		def, err := code.Lookup(ins[ip])
		if err != nil {
			t.Fatalf("undecodable opcode %d at %d: %v", ins[ip], ip, err)
		}
		_, width := code.ReadOperands(def, ins[ip+1:])
		if code.Opcode(ins[ip]) == code.OpGetBuiltin {
			copy(ins[ip:], code.Make(code.OpGetBuiltin, ordinal))
			rewritten++
		}
		ip += 1 + width
	}
	if rewritten == 0 {
		t.Fatalf("%q has no OpGetBuiltin in its main stream", src)
	}
	return mutil.EncryptByteCode(bc, legacyArtifactPassword)
}

// asV23ByteCode is bc as v2.3.0 declared ByteCode: no OpcodeMap, no Version.
func asV23ByteCode(bc *compiler.ByteCode) any {
	type ByteCode struct {
		Instructions code.Instructions
		Constants    []object.Object
		StructDefs   map[string][]*ast.Identifier
		EnumDefs     map[string][]string
		LuaPatches   map[string]*object.LuaPatch
	}
	return &ByteCode{bc.Instructions, bc.Constants, bc.StructDefs, bc.EnumDefs, bc.LuaPatches}
}

// asV24ByteCode is bc as v2.4.0 declared ByteCode. OpcodeMap is left nil, as a
// program built at mutation level 0 left it: what marks the release is that
// the type declares the field, not anything in it.
func asV24ByteCode(bc *compiler.ByteCode) any {
	type ByteCode struct {
		Instructions code.Instructions
		Constants    []object.Object
		StructDefs   map[string][]*ast.Identifier
		EnumDefs     map[string][]string
		LuaPatches   map[string]*object.LuaPatch
		OpcodeMap    []byte
	}
	return &ByteCode{bc.Instructions, bc.Constants, bc.StructDefs, bc.EnumDefs, bc.LuaPatches, nil}
}

// sealLegacyArtifact gob-encodes shape and wraps it as generator.encode wraps
// a program: zstd, the XOR layer, AES under the password, an Ed25519 signature.
func sealLegacyArtifact(t *testing.T, shape any) []byte {
	t.Helper()
	serialize.RegisterGobTypes()
	var stream bytes.Buffer
	if err := gob.NewEncoder(&stream).Encode(shape); err != nil {
		t.Fatalf("gob: %v", err)
	}
	var compressed bytes.Buffer
	zw, err := zstd.NewWriter(&compressed)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := zw.Write(stream.Bytes()); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	xored, err := security.SecureXOREncrypt(compressed.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	encrypted, err := security.AESEncrypt(xored, legacyArtifactPassword)
	if err != nil {
		t.Fatal(err)
	}
	keys, err := security.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	signed, err := security.SignCode(encrypted, keys.PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	return signed
}

// runLegacyArtifact decodes an artifact as the runner does and returns the
// value its program ends on.
func runLegacyArtifact(t *testing.T, artifact []byte) object.Object {
	t.Helper()
	bytecode, err := decode(artifact, legacyArtifactPassword)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	machine := vm.NewWithPasswordAndGlobalStoreMode(bytecode, legacyArtifactPassword,
		make([]object.Object, global.GlobalSize), false)
	defer machine.CleanupSensitiveData(true)
	if err := machine.Run(); err != nil {
		t.Fatalf("run: %v", err)
	}
	return machine.LastPoppedStackElement()
}

// TestAV23ArtifactsHelpCallRunsHelp is the row's repro: v2.3.0 compiled
// help("len") to ordinal 398, and it must still be help's answer.
func TestAV23ArtifactsHelpCallRunsHelp(t *testing.T) {
	got := runLegacyArtifact(t, sealLegacyArtifact(t, asV23ByteCode(legacyProgram(t, `help("len");`, 398))))
	text, ok := got.(*object.String)
	if !ok {
		t.Fatalf("a v2.3.0 artifact's help(\"len\") = %s %s, want help's STRING; ordinal 398 was "+
			"help in v2.3.0's registry", got.Type(), got.Inspect())
	}
	if !strings.Contains(text.Value, "len") {
		t.Fatalf("a v2.3.0 artifact's help(\"len\") = %q, which is not help for len", text.Value)
	}
}

// TestAV24ArtifactsPmapCallStillRunsPmap is the other half: the same ordinal
// in a v2.4.0 artifact is pmap, and stays pmap in one built with no opcode map,
// because what marks v2.4.0 is the field its ByteCode type declares.
func TestAV24ArtifactsPmapCallStillRunsPmap(t *testing.T) {
	got := runLegacyArtifact(t, sealLegacyArtifact(t,
		asV24ByteCode(legacyProgram(t, `pmap([1, 2, 3], fn(x) { x * 10 });`, 398))))
	if got.Inspect() != "[10, 20, 30]" {
		t.Fatalf("a v2.4.0 artifact's pmap call = %s %s, want [10, 20, 30]; ordinal 398 is pmap "+
			"in v2.4.0's registry", got.Type(), got.Inspect())
	}
}

// TestAV24ArtifactsHelpCallRunsHelp keeps v2.4.0's own help, at 408, where it was.
func TestAV24ArtifactsHelpCallRunsHelp(t *testing.T) {
	got := runLegacyArtifact(t, sealLegacyArtifact(t, asV24ByteCode(legacyProgram(t, `help("len");`, 408))))
	if text, ok := got.(*object.String); !ok || !strings.Contains(text.Value, "len") {
		t.Fatalf("a v2.4.0 artifact's help(\"len\") = %s %s, want help's STRING", got.Type(), got.Inspect())
	}
}

// TestADevBuildArtifactsHelpCallAt408StillRunsHelp covers the development
// builds 821bece and 31182f3: they had v2.4.0's registry, help at 408, but
// v2.3.0's ByteCode fields, so their artifacts read as v2.3.x. An ordinal above
// 398 never came from v2.3.x, and the v2.3.x table keeps v2.4.x's entries
// there, so this call runs help as it did before the registries were told
// apart, rather than being refused as out of range (repair round 1).
func TestADevBuildArtifactsHelpCallAt408StillRunsHelp(t *testing.T) {
	got := runLegacyArtifact(t, sealLegacyArtifact(t, asV23ByteCode(legacyProgram(t, `help("len");`, 408))))
	if text, ok := got.(*object.String); !ok || !strings.Contains(text.Value, "len") {
		t.Fatalf("a dev build's help(\"len\") at 408 = %s %s, want help's STRING", got.Type(), got.Inspect())
	}
}

// TestALegacyArtifactWhoseTypeCannotBeReadIsRefused covers the refusal: a
// versionless stream whose leading type is not ByteCode does not say which
// registry it was compiled against, and guessing is how one ordinal came to
// mean two builtins.
func TestALegacyArtifactWhoseTypeCannotBeReadIsRefused(t *testing.T) {
	type Program struct {
		Instructions code.Instructions
		Constants    []object.Object
	}
	bc := legacyProgram(t, `help("len");`, 398)
	_, err := decode(sealLegacyArtifact(t, &Program{bc.Instructions, bc.Constants}), legacyArtifactPassword)
	if err == nil {
		t.Fatal("a versionless program whose container does not declare ByteCode was decoded; " +
			"which registry its ordinals index is unknown")
	}
	for _, want := range []string{"Program", "recompile"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("the refusal should name what it found and what to do; got: %v", err)
		}
	}
}
