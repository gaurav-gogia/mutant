package vm

import (
	"fmt"
	"mutant/ast"
	"mutant/compiler"
	"mutant/lexer"
	"mutant/object"
	"mutant/parser"
	"testing"
)

type vmTestCase struct {
	input    string
	expected interface{}
}

func runVmTests(t *testing.T, tests []vmTestCase) {
	t.Helper()
	for _, tt := range tests {
		program := parse(tt.input)
		comp := compiler.New()
		if err := comp.Compile(program); err != nil {
			t.Fatalf("compiler error: %s", err)
		}

		vm := New(comp.ByteCode())
		if err := vm.Run(); err != nil {
			t.Fatalf("vm error: %s", err)
		}

		stackElem := vm.StackTop()
		testExpectedObject(t, tt.expected, stackElem)
	}
}

func parse(input string) *ast.Program {
	l := lexer.New(input)
	p := parser.New(l)

	return p.ParseProgram()
}

func testIntegerObject(expected int64, actual object.Object) error {
	result, ok := actual.(*object.Integer)

	if !ok {
		return fmt.Errorf("object is not Integer. got=%T (%+v)", actual, actual)
	}

	if result.Value != expected {
		return fmt.Errorf("object has wrong value. got=%d, want=%d", result.Value, expected)
	}

	return nil
}

func testExpectedObject(t *testing.T, expected interface{}, actual object.Object) {
	t.Helper()
	switch expected := expected.(type) {
	case int:
		if err := testIntegerObject(int64(expected), actual); err != nil {
			t.Errorf("testIntegerObject failed: %s", err)
		}
	}
}

func TestIntegerArithmatic(t *testing.T) {
	tests := []vmTestCase{
		{"1", 1},
		{"2", 2},
		{"1 + 2", 3}, // TODO: FIX THIS
	}
	runVmTests(t, tests)
}
