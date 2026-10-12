package compiler

// Bytecode container versions.
//
// The version travels in ByteCode.Version and decides how an OpGetBuiltin
// operand is read. It is not a compatibility flag for the instruction set: an
// opcode's meaning has never depended on it, and a change there would need a
// different mechanism.
//
// The instruction set is versioned by a different rule, which is worth stating
// because it looks like an omission. Opcode values have been append-only since
// v2.2.0: a value emitted by v2.2.0 or later never changes meaning, and new ones
// go on the end. Operand widths have not changed since v2.3.0. So bytecode from
// v2.3.0 on keeps its opcodes' meanings and widths on a newer runtime, and the
// only failing direction is new bytecode on an old runtime -- where the old
// runtime meets an opcode it does not know and stops, naming it. Bumping this
// constant would not help that runtime, which does not know the new version
// either; the guarantee is the append-only rule plus a refusal, not a number.
//
// That refusal is recent. Until the boxed-cell opcodes went in, the dispatch
// switch had no default arm: an unknown opcode matched nothing, the instruction
// pointer advanced by one, and the operand bytes were executed as opcodes. See
// VM.execLoop's default case.
//
// Neither the append-only rule nor the fixed widths held earlier, and no check
// recognises bytecode that predates them. v2.2.0 inserted OpMultiValue, OpDup
// and OpDestructure ahead of OpGetBuiltin, moving OpGetBuiltin from 27 to 30
// and every opcode after it up by three, so bytecode from v2.1.x and earlier
// does not keep its opcodes' meanings. v2.3.0 widened OpGetBuiltin's operand
// from one byte to two, so this runtime reads two operand bytes where v2.2.x
// wrote one: a v2.2.x program's first builtin call takes the next instruction's
// first byte into its operand, binds whichever builtin the two bytes number (or
// stops when they number none), and leaves the instructions after it read out
// of step. v2.2.x wrote the same ByteCode type as v2.3.x, and no container
// before v2.5.0 records which instruction set it was compiled for.
const (
	// BytecodeVersionOrdinalBuiltins is everything compiled up to and including
	// v2.4.0, when OpGetBuiltin carried an ordinal into the global builtin
	// registry. Such bytecode has no Version field at all, so it decodes to 0
	// and normalises to this. Its operands resolve through the frozen tables in
	// builtin/legacy_ordinals.go -- v2.4.x's, or v2.3.x's, which held help where
	// v2.4.x holds pmap. LegacyRegistryOf reads which from the container.
	BytecodeVersionOrdinalBuiltins = 1

	// BytecodeVersionNamedBuiltins is bytecode that carries ByteCode.BuiltinNames
	// and whose OpGetBuiltin operand indexes that table.
	BytecodeVersionNamedBuiltins = 2

	// BytecodeVersion is what this build emits.
	BytecodeVersion = BytecodeVersionNamedBuiltins
)

// NormalizeVersion reports the container version of decoded bytecode, mapping
// the absent field of a pre-versioning artifact onto the version it implies.
func NormalizeVersion(version int) int {
	if version <= 0 {
		return BytecodeVersionOrdinalBuiltins
	}
	return version
}
