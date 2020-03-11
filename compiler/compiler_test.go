package compiler

import (
	"fmt"
	"mutant/ast"
	"mutant/code"
	"mutant/lexer"
	"mutant/object"
	"mutant/parser"
	"testing"
)

type compilerTestCase struct {
	input                string
	expectedConstants    []interface{}
	expectedInstructions []code.Instructions
}

func TestIntegerArithmatic(t *testing.T) {
	tests := []compilerTestCase{
		{
			input:             "1 + 2",
			expectedConstants: []interface{}{1, 2},
			expectedInstructions: []code.Instructions{
				code.Make(code.OpConstant, 0),
				code.Make(code.OpConstant, 1),
				code.Make(code.OpAdd),
				code.Make(code.OpPop),
			},
		},
		{
			input:             "1; 2",
			expectedConstants: []interface{}{1, 2},
			expectedInstructions: []code.Instructions{
				code.Make(code.OpConstant, 0),
				code.Make(code.OpPop),
				code.Make(code.OpConstant, 1),
				code.Make(code.OpPop),
			},
		},
		{
			input:             "1 - 2",
			expectedConstants: []interface{}{1, 2},
			expectedInstructions: []code.Instructions{
				code.Make(code.OpConstant, 0),
				code.Make(code.OpConstant, 1),
				code.Make(code.OpSub),
				code.Make(code.OpPop),
			},
		},
		{
			input:             "1 * 2",
			expectedConstants: []interface{}{1, 2},
			expectedInstructions: []code.Instructions{
				code.Make(code.OpConstant, 0),
				code.Make(code.OpConstant, 1),
				code.Make(code.OpMul),
				code.Make(code.OpPop),
			},
		},
		{
			input:             "2 / 1",
			expectedConstants: []interface{}{2, 1},
			expectedInstructions: []code.Instructions{
				code.Make(code.OpConstant, 0),
				code.Make(code.OpConstant, 1),
				code.Make(code.OpDiv),
				code.Make(code.OpPop),
			},
		},
	}

	runCompilerTests(t, tests)
}

func TestBooleanExpressions(t *testing.T) {
	tests := []compilerTestCase{
		{
			input:             "true",
			expectedConstants: []interface{}{},
			expectedInstructions: []code.Instructions{
				code.Make(code.OpTrue),
				code.Make(code.OpPop),
			},
		},
		{
			input:             "false",
			expectedConstants: []interface{}{},
			expectedInstructions: []code.Instructions{
				code.Make(code.OpFalse),
				code.Make(code.OpPop),
			},
		},
	}
	runCompilerTests(t, tests)
}

func runCompilerTests(t *testing.T, tests []compilerTestCase) {
	t.Helper()
	for _, tt := range tests {
		program := parse(tt.input)

		compiler := New()
		if err := compiler.Compile(program); err != nil {
			t.Fatalf("compiler error: %s", err)
		}

		bytecode := compiler.ByteCode()
		if err := testInstructions(tt.expectedInstructions, bytecode.Instructions); err != nil {
			t.Fatalf("testInstructions failed: %s", err)
		}

		if err := testConstants(t, tt.expectedConstants, bytecode.Constants); err != nil {
			t.Fatalf("testConstants failed: %s", err)
		}
	}
}

func parse(input string) ast.Node {
	l := lexer.New(input)
	p := parser.New(l)
	return p.ParseProgram()
}

func testInstructions(expected []code.Instructions, actual code.Instructions) error {
	concatted := flattenInstructions(expected)
	if len(actual) != len(concatted) {
		return fmt.Errorf("\nwrong instructions length.\nwant = %q,\ngot = %q", concatted, actual)
	}

	for i, ins := range concatted {
		if actual[i] != ins {
			return fmt.Errorf("\nwrong instruction at %d, \ngot  = %q,\nwant = %q", i, concatted, actual)
		}
	}

	return nil
}

func flattenInstructions(s []code.Instructions) code.Instructions {
	var out code.Instructions
	for _, ins := range s {
		out = append(out, ins...)
	}
	return out
}

func testConstants(t *testing.T, expected []interface{}, actual []object.Object) error {
	if len(expected) != len(actual) {
		return fmt.Errorf("wrong number of constants. got = %d, want = %d", len(actual), len(expected))
	}
	for i, constant := range expected {
		switch cons := constant.(type) {
		case int:
			if err := testIntegerObject(int64(cons), actual[i]); err != nil {
				return fmt.Errorf("constant %d - testIntegerObject failed - %s", i, err)
			}
		}
	}
	return nil
}

func testIntegerObject(expected int64, actual object.Object) error {
	result, ok := actual.(*object.Integer)
	if !ok {
		return fmt.Errorf("object is not integer. got = %T, (%+v)", actual, actual)
	}

	if result.Value != expected {
		return fmt.Errorf("object has wrong value. got = %d, want = %d", result.Value, expected)
	}

	return nil
}
