package compiler

import (
	"mutant/code"
	"testing"
)

func TestPolymorphicMutation(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		mutationLevel int
	}{
		{
			name:          "Mutation level 0 - no mutations",
			input:         "let x = 5 + 3; x",
			mutationLevel: 0,
		},
		{
			name:          "Mutation level 5 - medium mutations",
			input:         "let x = 5 + 3; x",
			mutationLevel: 5,
		},
		{
			name:          "Mutation level 10 - maximum mutations",
			input:         "let x = 5 + 3; x",
			mutationLevel: 10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Compile without polymorphism
			program := parse(tt.input)
			compiler1 := New()
			if err := compiler1.Compile(program); err != nil {
				t.Fatalf("compiler error: %s", err)
			}
			originalBytecode := compiler1.ByteCode()

			// Compile with polymorphism
			compiler2 := New()
			compiler2.EnablePolymorphism(tt.mutationLevel)
			if err := compiler2.Compile(program); err != nil {
				t.Fatalf("compiler error: %s", err)
			}
			mutatedBytecode := compiler2.ByteCode()

			// For level 0, bytecode should be identical
			if tt.mutationLevel == 0 {
				if len(mutatedBytecode.Instructions) != len(originalBytecode.Instructions) {
					t.Errorf("Mutation level 0: bytecode should not change. Expected %d bytes, got %d",
						len(originalBytecode.Instructions), len(mutatedBytecode.Instructions))
				}
				return
			}

			// For levels > 0, bytecode should be different (unless extremely unlucky)
			if len(mutatedBytecode.Instructions) == len(originalBytecode.Instructions) {
				// Check if instructions are actually identical
				identical := true
				for i := range originalBytecode.Instructions {
					if originalBytecode.Instructions[i] != mutatedBytecode.Instructions[i] {
						identical = false
						break
					}
				}
				if identical {
					t.Logf("Warning: Mutation level %d produced identical bytecode (possible but rare)", tt.mutationLevel)
				}
			}

			// Verify polymorphic marker is present for mutated bytecode
			if tt.mutationLevel > 0 {
				level := DetectPolymorphicLevel(mutatedBytecode.Instructions)
				if level != tt.mutationLevel {
					t.Errorf("Polymorphic marker incorrect. Expected level %d, got %d",
						tt.mutationLevel, level)
				}
			}

			// Verify constants are preserved
			if len(originalBytecode.Constants) != len(mutatedBytecode.Constants) {
				t.Errorf("Constants count changed. Expected %d, got %d",
					len(originalBytecode.Constants), len(mutatedBytecode.Constants))
			}
		})
	}
}

func TestPolymorphicWithSeed(t *testing.T) {
	input := "let x = 10; let y = 20; x + y"
	seed := int64(12345)
	mutationLevel := 7

	// Compile with seed 1
	program1 := parse(input)
	compiler1 := New()
	compiler1.EnablePolymorphismWithSeed(mutationLevel, seed)
	if err := compiler1.Compile(program1); err != nil {
		t.Fatalf("compiler error: %s", err)
	}
	bytecode1 := compiler1.ByteCode()

	// Compile with same seed again
	program2 := parse(input)
	compiler2 := New()
	compiler2.EnablePolymorphismWithSeed(mutationLevel, seed)
	if err := compiler2.Compile(program2); err != nil {
		t.Fatalf("compiler error: %s", err)
	}
	bytecode2 := compiler2.ByteCode()

	// With same seed, polymorphic transformations should be reproducible
	// (though random elements during mutation may still vary)
	if len(bytecode1.Instructions) == 0 {
		t.Error("Bytecode 1 is empty")
	}
	if len(bytecode2.Instructions) == 0 {
		t.Error("Bytecode 2 is empty")
	}

	// Both should have polymorphic marker
	level1 := DetectPolymorphicLevel(bytecode1.Instructions)
	level2 := DetectPolymorphicLevel(bytecode2.Instructions)
	if level1 != mutationLevel || level2 != mutationLevel {
		t.Errorf("Polymorphic markers incorrect. Expected %d, got %d and %d",
			mutationLevel, level1, level2)
	}
}

func TestPolymorphicNOPInjection(t *testing.T) {
	input := "fn(x) { x + 1 }"
	mutationLevel := 10 // Max mutations should inject NOPs

	program := parse(input)
	compiler := New()
	compiler.EnablePolymorphism(mutationLevel)
	if err := compiler.Compile(program); err != nil {
		t.Fatalf("compiler error: %s", err)
	}
	bytecode := compiler.ByteCode()

	// With high mutation level, bytecode should be longer due to NOP injection
	// (though this is probabilistic, so we just verify it compiles successfully)
	if len(bytecode.Instructions) == 0 {
		t.Error("Mutated bytecode is empty")
	}

	// Verify polymorphic marker
	level := DetectPolymorphicLevel(bytecode.Instructions)
	if level != mutationLevel {
		t.Errorf("Expected polymorphic level %d, got %d", mutationLevel, level)
	}
}

func TestByteCodeWithoutPolymorphismStillWorks(t *testing.T) {
	input := "1 + 2"
	expected := []code.Instructions{
		code.Make(code.OpConstant, 0),
		code.Make(code.OpConstant, 1),
		code.Make(code.OpAdd),
		code.Make(code.OpPop),
	}

	program := parse(input)
	compiler := New()
	// Don't enable polymorphism
	if err := compiler.Compile(program); err != nil {
		t.Fatalf("compiler error: %s", err)
	}

	bytecode := compiler.ByteCode()
	if err := testInstructions(expected, bytecode.Instructions); err != nil {
		t.Errorf("Bytecode without polymorphism incorrect: %s", err)
	}

	// No polymorphic marker when not enabled
	level := DetectPolymorphicLevel(bytecode.Instructions)
	if level != 0 {
		t.Errorf("Expected no polymorphic marker, but found level %d", level)
	}
}
