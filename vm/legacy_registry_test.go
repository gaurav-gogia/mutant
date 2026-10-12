package vm

import (
	"testing"

	"mutant/builtin"
	"mutant/code"
	"mutant/compiler"
	"mutant/object"
)

// TestAV23ProgramIndexesV23sRegistry: when the runner has read from the
// container that a versionless program was compiled by v2.3.x, the VM must
// index v2.3.x's registry -- help at 398 -- and not v2.4.x's, where 398 is pmap
// (M26-BLT-014).
func TestAV23ProgramIndexesV23sRegistry(t *testing.T) {
	bc := downgradeToOrdinalEra(t, compileFresh(t, `len([]);`))
	bc.SetLegacyRegistry(compiler.LegacyRegistryV23)

	machine := New(bc)
	if machine.builtinsErr != nil {
		t.Fatalf("resolving a v2.3.x program: %v", machine.builtinsErr)
	}
	if len(machine.builtins) != builtin.LegacyOrdinalCount() {
		t.Fatalf("a v2.3.x program resolved %d builtins; the v2.3.x table keeps all %d of v2.4.x's slots",
			len(machine.builtins), builtin.LegacyOrdinalCount())
	}
	if machine.builtins[398] != builtin.GetBuiltinByName(builtin.BuiltinNameHelp) {
		t.Fatal("ordinal 398 of a v2.3.x program is not help")
	}
}

// TestAV23ProgramStillReachesV24sBuiltinsPast398: no v2.3.x artifact holds an
// ordinal above 398, but artifacts from the development builds 821bece and
// 31182f3 do -- they had v2.4.0's registry and no OpcodeMap, so they read as
// v2.3.x. Their call to help, at 408, must still run help, as it did before the
// registries were told apart, rather than be refused as out of range
// (M26-BLT-014, repair round 1).
func TestAV23ProgramStillReachesV24sBuiltinsPast398(t *testing.T) {
	bc := downgradeToOrdinalEra(t, compileFresh(t, `help("len");`))
	if code.Opcode(bc.Instructions[0]) != code.OpGetBuiltin {
		t.Fatalf("the program does not begin with OpGetBuiltin: %v", bc.Instructions)
	}
	copy(bc.Instructions, code.Make(code.OpGetBuiltin, 408))
	bc.SetLegacyRegistry(compiler.LegacyRegistryV23)

	machine, err := runSealed(t, bc)
	if err != nil {
		t.Fatalf("a v2.3.x-read program's operand 408 (help in 821bece's registry) did not run: %v", err)
	}
	if _, ok := machine.LastPoppedStackElement().(*object.String); !ok {
		t.Fatalf("operand 408 = %v, want help's STRING", machine.LastPoppedStackElement())
	}
}
