package compiler

import (
	"crypto/rand"
	"encoding/binary"
	"math/big"
	"mutant/code"
	"mutant/object"
)

// PolymorphicEngine generates functionally equivalent but structurally different bytecode
type PolymorphicEngine struct {
	mutationLevel int   // 0-10: intensity of mutations
	randomSeed    int64 // seed for reproducible builds
}

// MutationConfig controls which mutations are applied
type MutationConfig struct {
	InsertNOPs          bool
	ReorderInstructions bool
	MutateOpcodes       bool
	InsertDeadCode      bool
	RandomizeConstants  bool
	Level               int // 0-10
}

// NewPolymorphicEngine creates a new polymorphic engine
func NewPolymorphicEngine(level int, seed int64) *PolymorphicEngine {
	if level < 0 {
		level = 0
	}
	if level > 10 {
		level = 10
	}

	return &PolymorphicEngine{
		mutationLevel: level,
		randomSeed:    seed,
	}
}

// Mutate applies polymorphic transformations to bytecode
func (pe *PolymorphicEngine) Mutate(bytecode *ByteCode) *ByteCode {
	if pe.mutationLevel == 0 {
		return bytecode // No mutations
	}

	config := pe.getConfig()

	// Apply mutations in stages
	if config.InsertNOPs {
		bytecode.Instructions = pe.insertNOPs(bytecode.Instructions)
	}

	if config.MutateOpcodes {
		bytecode = pe.mutateOpcodes(bytecode)
	}

	if config.RandomizeConstants {
		bytecode = pe.randomizeConstantPool(bytecode)
	}

	return bytecode
}

// getConfig returns mutation configuration based on level
func (pe *PolymorphicEngine) getConfig() MutationConfig {
	return MutationConfig{
		InsertNOPs:          pe.mutationLevel >= 3,
		ReorderInstructions: pe.mutationLevel >= 5,
		MutateOpcodes:       pe.mutationLevel >= 7,
		InsertDeadCode:      pe.mutationLevel >= 8,
		RandomizeConstants:  pe.mutationLevel >= 4,
		Level:               pe.mutationLevel,
	}
}

// insertNOPs inserts no-operation instructions
func (pe *PolymorphicEngine) insertNOPs(instructions code.Instructions) code.Instructions {
	if len(instructions) == 0 {
		return instructions
	}

	// Calculate NOP insertion rate based on level
	// Level 3: ~5%, Level 10: ~15%
	insertionRate := float64(pe.mutationLevel) * 1.5 / 100.0

	result := make(code.Instructions, 0, int(float64(len(instructions))*(1+insertionRate)))

	for i := 0; i < len(instructions); i++ {
		result = append(result, instructions[i])

		// Randomly insert NOP after this instruction
		if pe.shouldInsertNOP(insertionRate) {
			nop := pe.generateNOP()
			result = append(result, nop...)
		}
	}

	return result
}

// shouldInsertNOP determines if a NOP should be inserted
func (pe *PolymorphicEngine) shouldInsertNOP(rate float64) bool {
	max := big.NewInt(100)
	n, _ := rand.Int(rand.Reader, max)
	return float64(n.Int64()) < rate*100
}

// generateNOP creates a no-operation instruction sequence
func (pe *PolymorphicEngine) generateNOP() code.Instructions {
	// Generate different types of NOPs randomly
	nopType := pe.randomInt(4)

	switch nopType {
	case 0:
		// Push null then pop
		return append(code.Make(code.OpNull), code.Make(code.OpPop)...)
	case 1:
		// Push true then pop
		return append(code.Make(code.OpTrue), code.Make(code.OpPop)...)
	case 2:
		// Push false then pop
		return append(code.Make(code.OpFalse), code.Make(code.OpPop)...)
	default:
		// Just OpPop (safe if stack has something)
		return code.Make(code.OpPop)
	}
}

// mutateOpcodes remaps opcodes to different values
func (pe *PolymorphicEngine) mutateOpcodes(bytecode *ByteCode) *ByteCode {
	// Create a random opcode mapping
	mapping := pe.generateOpcodeMapping()

	// Apply mapping to instructions
	newInstructions := make(code.Instructions, len(bytecode.Instructions))
	copy(newInstructions, bytecode.Instructions)

	for i := 0; i < len(newInstructions); i++ {
		// Check if this is an opcode position
		if mapped, ok := mapping[code.Opcode(newInstructions[i])]; ok {
			newInstructions[i] = byte(mapped)
		}
	}

	// Apply mapping to compiled functions in constants
	for i, constant := range bytecode.Constants {
		if fn, ok := constant.(*object.CompiledFunction); ok {
			newFnInsts := make(code.Instructions, len(fn.Instructions))
			copy(newFnInsts, fn.Instructions)

			for j := 0; j < len(newFnInsts); j++ {
				if mapped, ok := mapping[code.Opcode(newFnInsts[j])]; ok {
					newFnInsts[j] = byte(mapped)
				}
			}

			fn.Instructions = newFnInsts
			bytecode.Constants[i] = fn
		}
	}

	bytecode.Instructions = newInstructions
	return bytecode
}

// generateOpcodeMapping creates a random but valid opcode remapping
func (pe *PolymorphicEngine) generateOpcodeMapping() map[code.Opcode]code.Opcode {
	// For now, return identity mapping (no change)
	// In production, this would create a shuffled mapping
	// Note: This requires storing the mapping in bytecode metadata
	// for the VM to reverse it
	return make(map[code.Opcode]code.Opcode)
}

// randomizeConstantPool shuffles constant pool indices
func (pe *PolymorphicEngine) randomizeConstantPool(bytecode *ByteCode) *ByteCode {
	if len(bytecode.Constants) <= 1 {
		return bytecode
	}

	// Create a shuffled index mapping
	mapping := pe.generateShuffleMapping(len(bytecode.Constants))

	// Reorder constants
	newConstants := make([]object.Object, len(bytecode.Constants))
	for oldIdx, newIdx := range mapping {
		newConstants[newIdx] = bytecode.Constants[oldIdx]
	}

	// Update all OpConstant instructions
	bytecode.Instructions = pe.updateConstantReferences(bytecode.Instructions, mapping)

	// Update references in compiled functions
	for i, constant := range newConstants {
		if fn, ok := constant.(*object.CompiledFunction); ok {
			fn.Instructions = pe.updateConstantReferences(fn.Instructions, mapping)
			newConstants[i] = fn
		}
	}

	bytecode.Constants = newConstants
	return bytecode
}

// generateShuffleMapping creates a random permutation
func (pe *PolymorphicEngine) generateShuffleMapping(size int) []int {
	mapping := make([]int, size)
	for i := range mapping {
		mapping[i] = i
	}

	// Fisher-Yates shuffle
	for i := size - 1; i > 0; i-- {
		j := pe.randomInt(i + 1)
		mapping[i], mapping[j] = mapping[j], mapping[i]
	}

	return mapping
}

// updateConstantReferences updates OpConstant operands
func (pe *PolymorphicEngine) updateConstantReferences(instructions code.Instructions, mapping []int) code.Instructions {
	result := make(code.Instructions, len(instructions))
	copy(result, instructions)

	for i := 0; i < len(result); i++ {
		if code.Opcode(result[i]) == code.OpConstant {
			// Read old index
			oldIdx := binary.BigEndian.Uint16(result[i+1 : i+3])

			// Map to new index
			if int(oldIdx) < len(mapping) {
				newIdx := uint16(mapping[oldIdx])
				binary.BigEndian.PutUint16(result[i+1:i+3], newIdx)
			}

			i += 2 // Skip operand bytes
		}
	}

	return result
}

// randomInt generates a random integer in range [0, max)
func (pe *PolymorphicEngine) randomInt(max int) int {
	if max <= 0 {
		return 0
	}

	n, _ := rand.Int(rand.Reader, big.NewInt(int64(max)))
	return int(n.Int64())
}

// AddPolymorphicMarker adds metadata to bytecode indicating polymorphic level
func (pe *PolymorphicEngine) AddPolymorphicMarker(instructions code.Instructions) code.Instructions {
	// Add a marker byte at the end indicating mutation level
	// Format: [original_instructions][0xFF][level]
	marker := []byte{0xFF, byte(pe.mutationLevel)}
	return append(instructions, marker...)
}

// DetectPolymorphicLevel reads the polymorphic level from bytecode
func DetectPolymorphicLevel(instructions code.Instructions) int {
	if len(instructions) < 2 {
		return 0
	}

	// Check for marker
	if instructions[len(instructions)-2] == 0xFF {
		return int(instructions[len(instructions)-1])
	}

	return 0
}
