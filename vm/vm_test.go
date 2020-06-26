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

func runVMTests(t *testing.T, tests []vmTestCase) {
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

		stackElem := vm.LastPoppedStackElement()
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

func testBooleanObject(expected bool, actual object.Object) error {
	result, ok := actual.(*object.Boolean)
	if !ok {
		return fmt.Errorf("object is not Boolean. got=%T (%+v)", actual, actual)
	}

	if result.Value != expected {
		return fmt.Errorf("object has wrong value. got=%t, want=%t", result.Value, expected)
	}

	return nil
}

func testStringObject(expected string, actual object.Object) error {
	result, ok := actual.(*object.String)
	if !ok {
		return fmt.Errorf("object is not String. got=%T (%+v)", actual, actual)
	}

	if result.Value != expected {
		return fmt.Errorf("object has wrong value. got=%q, want=%q", result.Value, expected)
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
	case bool:
		if err := testBooleanObject(bool(expected), actual); err != nil {
			t.Errorf("testBooleanObject failed: %s", err)
		}
	case string:
		if err := testStringObject(string(expected), actual); err != nil {
			t.Errorf("testStringObject failed: %s", err)
		}
	case []int:
		array, ok := actual.(*object.Array)
		if !ok {
			t.Errorf("object not array: %T (%+v)", actual, actual)
			return
		}
		if len(array.Elements) != len(expected) {
			t.Errorf("wrong number of elements. want=%d, got=%d", len(expected), len(array.Elements))
			return
		}
		for i, expectedElement := range expected {
			if err := testIntegerObject(int64(expectedElement), array.Elements[i]); err != nil {
				t.Errorf("testIntegerObject failed: %s", err)
			}
		}
	case map[object.HashKey]int64:
		hash, ok := actual.(*object.Hash)
		if !ok {
			t.Errorf("object is not Hash. got=%T (%+v)", actual, actual)
			return
		}
		if len(hash.Pairs) != len(expected) {
			t.Errorf("hash has wrong number of pairs, want=%d, got=%d", len(expected), len(hash.Pairs))
			return
		}
		for expectedKey, expectedValue := range expected {
			pair, ok := hash.Pairs[expectedKey]
			if !ok {
				t.Errorf("no pair for given key in the Pairs")
			}
			if err := testIntegerObject(expectedValue, pair.Value); err != nil {
				t.Errorf("testIntegerObject failed: %s", err)
			}
		}
	case *object.Null:
		if actual != Null {
			t.Errorf("object is not Null: %T (%+v)", actual, actual)
		}
	case *object.Error:
		errObj, ok := actual.(*object.Error)
		if !ok {
			t.Errorf("object is not Error: %T (%v)", actual, actual)
			return
		}
		if errObj.Message != expected.Message {
			t.Errorf("wrong error message. expected:%q, got:%q", expected.Message, errObj.Message)
		}
	}
}

func TestIntegerArithmatic(t *testing.T) {
	tests := []vmTestCase{
		{"1", 1},
		{"2", 2},
		{"1 + 2", 3},
		{"1 - 2", -1},
		{"1 * 2", 2},
		{"4 / 2", 2},
		{"50 / 2 * 2 + 10 - 5", 55},
		{"5 + 5 + 5 + 5 - 10", 10},
		{"2 * 2 * 2 * 2 * 2", 32},
		{"5 * 2 + 10", 20},
		{"5 + 2 * 10", 25},
		{"5 * (2 + 10)", 60},
		{"-5", -5},
		{"-10", -10},
		{"-50 + 100 + -50", 0},
		{"(5 + 10 * 2 + 15 / 3) * 2 + -10", 50},
	}
	runVMTests(t, tests)
}

func TestBooleanExpressions(t *testing.T) {
	tests := []vmTestCase{
		{"true", true},
		{"false", false},
		{"1 < 2", true},
		{"1 > 2", false},
		{"1 < 1", false},
		{"1 > 1", false},
		{"1 == 1", true},
		{"1 != 1", false},
		{"1 == 2", false},
		{"1 != 2", true},
		{"true == true", true},
		{"false == false", true},
		{"true == false", false},
		{"true != false", true},
		{"false != true", true},
		{"(1 < 2) == true", true},
		{"(1 < 2) == false", false},
		{"(1 > 2) == true", false},
		{"(1 > 2) == false", true},
		{"!false", true},
		{"!true", false},
		{"!5", false},
		{"!!true", true},
		{"!!false", false},
		{"!!5", true},
		{"!(if (false) { 5; })", true},
	}
	runVMTests(t, tests)
}

func TestConditionals(t *testing.T) {
	tests := []vmTestCase{
		{"if (true) { 10 }", 10},
		{"if (true) { 10 } else { 20 }", 10},
		{"if (false) { 10 } else { 20 } ", 20},
		{"if (1) { 10 }", 10},
		{"if (1 < 2) { 10 }", 10},
		{"if (1 < 2) { 10 } else { 20 }", 10},
		{"if (1 > 2) { 10 } else { 20 }", 20},
		{"if (1 > 2) { 10 }", Null},
		{"if (false) { 10 }", Null},
		{"if ((if (false) { 10 })) { 10 } else { 20 }", 20},
	}
	runVMTests(t, tests)
}

func TestGlobalLetStatements(t *testing.T) {
	tests := []vmTestCase{
		{"let one = 1; one", 1},
		{"let one = 1; let two = 2; one + two", 3},
		{"let one = 1; let two = one + one; one + two", 3},
	}
	runVMTests(t, tests)
}

func TestStringExpressions(t *testing.T) {
	tests := []vmTestCase{
		{`"monkey"`, "monkey"},
		{`"mon" + "key"`, "monkey"},
		{`"mon" + "key" + "banana"`, "monkeybanana"},
	}
	runVMTests(t, tests)
}

func TestArrayLiterals(t *testing.T) {
	tests := []vmTestCase{
		{"[]", []int{}},
		{"[1, 2, 3]", []int{1, 2, 3}},
		{"[1 + 2, 3 * 4, 5 + 6]", []int{3, 12, 11}},
	}
	runVMTests(t, tests)
}

func TestHashLiterals(t *testing.T) {
	tests := []vmTestCase{
		{"{}", map[object.HashKey]int64{}},
		{
			"{1: 2, 2: 3}",
			map[object.HashKey]int64{
				(&object.Integer{Value: 1}).HashKey(): 2,
				(&object.Integer{Value: 2}).HashKey(): 3,
			},
		},
		{
			"{1+1: 2*2, 3+3: 4*4}",
			map[object.HashKey]int64{
				(&object.Integer{Value: 1 + 1}).HashKey(): 2 * 2,
				(&object.Integer{Value: 3 + 3}).HashKey(): 4 * 4,
			},
		},
	}
	runVMTests(t, tests)
}

func TestIndexExpressions(t *testing.T) {
	tests := []vmTestCase{
		{"[1, 2, 3][1]", 2},
		{"[1, 2, 3][0 + 2]", 3},
		{"[[1, 1, 1]][0][0]", 1},
		{"[][0]", Null},
		{"[1, 2, 3][99]", Null},
		{"[1][-1]", Null},
		{"{1: 1, 2: 2}[1]", 1},
		{"{1: 1, 2: 2}[2]", 2},
		{"{1: 1}[0]", Null},
		{"{}[0]", Null},
	}
	runVMTests(t, tests)
}

func TestCallingFunctionsWithoutArgument(t *testing.T) {
	tests := []vmTestCase{
		{
			input:    "let x = fn(){ 5 + 10; }; x();",
			expected: 15,
		},
		{
			input:    "let o = fn(){1;}; let t = fn(){2;}; o() + t();",
			expected: 3,
		},
		{
			input:    "let a = fn() { 1 }; let b = fn() { a() + 1 }; let c = fn() { b() + 1 }; c();",
			expected: 3,
		},
	}
	runVMTests(t, tests)
}

func TestCallingFunctionsWithReturn(t *testing.T) {
	tests := []vmTestCase{
		{
			input:    "let x = fn(){ return 5 + 10; }; x();",
			expected: 15,
		},
		{
			input:    "let o = fn(){ return 1;}; let t = fn(){ return 2;}; o() + t();",
			expected: 3,
		},
		{
			input:    "let a = fn() { return 1; }; let b = fn() { return a() + 1; }; let c = fn() { return b() + 1; }; c();",
			expected: 3,
		},
	}
	runVMTests(t, tests)
}

func TestFunctionsWithoutReturnValue(t *testing.T) {
	tests := []vmTestCase{
		{input: ` let noReturn = fn() { }; noReturn(); `, expected: Null},
		{input: ` let noReturn = fn() { }; let noReturnTwo = fn() { noReturn(); }; noReturn(); noReturnTwo(); `, expected: Null},
	}
	runVMTests(t, tests)
}

func TestFirstClassFunctions(t *testing.T) {
	tests := []vmTestCase{
		{
			input:    "let returnsOne = fn() { 1; }; let returnsOneReturner = fn() { returnsOne; }; returnsOneReturner()(); ",
			expected: 1,
		},
	}
	runVMTests(t, tests)
}

func TestCallingFunctionsWithBindings(t *testing.T) {
	tests := []vmTestCase{
		{
			input:    "let one = fn() { let one = 1; one }; one();",
			expected: 1,
		},
		{
			input:    "let oneAndTwo = fn() { let one = 1; let two = 2; one + two; }; oneAndTwo();",
			expected: 3,
		},
		{
			input: `
					let oneAndTwo = fn() { let one = 1; let two = 2; one + two; };
					let threeAndFour = fn() { let three = 3; let four = 4; three + four; };
					oneAndTwo() + threeAndFour();
					`,
			expected: 10,
		},
		{
			input: `
						let firstFoobar = fn() { let foobar = 50; foobar; };
						let secondFoobar = fn() { let foobar = 100; foobar; };
						firstFoobar() + secondFoobar();
					`,
			expected: 150,
		},
		{
			input: `
						let globalSeed = 50; 
						let minusOne = fn() { let num = 1; return globalSeed - num; };
						let minusTwo = fn() { let num = 2; return globalSeed - num; };
						minusOne() + minusTwo();
					`,
			expected: 97,
		},
		{
			input:    "let returnsOneReturner = fn() { let returnsOne = fn() { 1; }; returnsOne; }; returnsOneReturner()();",
			expected: 1,
		},
	}
	runVMTests(t, tests)
}

func TestCallingFunctionsWithArgumentsAndBindings(t *testing.T) {
	tests := []vmTestCase{
		{input: "let identity = fn(a) { a; }; identity(4);", expected: 4},
		{input: "let sum = fn(a, b) { a + b; }; sum(1, 2); ", expected: 3},
		{input: "let sum = fn(a, b) { let c = a + b; c; }; sum(1, 2);", expected: 3},
		{input: "let sum = fn(a, b) { let c = a + b; c; }; sum(1, 2) + sum(3, 4);", expected: 10},
		{input: "let sum = fn(a, b) { let c = a + b; c; }; let outer = fn() { sum(1, 2) + sum(3, 4); }; outer();", expected: 10},
		{
			input: `
					let globalNum = 10; 
					let sum = fn(a, b) {
						let c = a + b;
						c + globalNum;
					};	
					
					let outer = fn() {
						sum(1, 2) + sum(3, 4) + globalNum;
					};

					outer() + globalNum;
				   `,
			expected: 50,
		},
	}
	runVMTests(t, tests)
}

func TestCallingFunctionsWithWrongArguments(t *testing.T) {
	tests := []vmTestCase{
		{input: "fn() { 1; }(1);", expected: "wrong number of arguments. want=0, got=1"},
		{input: "fn(a) { a; }();", expected: "wrong number of arguments. want=1, got=0"},
		{input: "fn(a, b) { a + b; }(1);", expected: "wrong number of arguments. want=2, got=1"},
	}

	for _, tt := range tests {
		program := parse(tt.input)
		comp := compiler.New()

		if err := comp.Compile(program); err != nil {
			t.Fatalf("compiler error: %s", err)
		}

		vm := New(comp.ByteCode())

		if err := vm.Run(); err == nil {
			t.Fatalf("expected VM error but resulted in none.")
		} else if err.Error() != tt.expected {
			t.Fatalf("wrong VM error: want=%q, got=%q", tt.expected, err)
		}
	}
}

func TestBuiltinFunctions(t *testing.T) {
	tests := []vmTestCase{
		{`len([1, 2, 3]);`, 3},
		{`let a = [1, 2, 3]; len(a)`, 3},
		{`len("")`, 0},
		{`len("four")`, 4},
		{`len("hello world")`, 11},
		{`len(1)`, &object.Error{Message: "argument to `len` not supported, got INTEGER"}},
		{`len("one", "two")`, &object.Error{Message: "wrong number of arguments. got=2, want=1"}},
		{`len([])`, 0},
		{`puts("hello", "world!")`, Null},
		{`first([1, 2, 3])`, 1},
		{`first([])`, Null},
		{`first(1)`, &object.Error{Message: "argument to `first` must be ARRAY, got INTEGER"}},
		{`last([1, 2, 3])`, 3},
		{`last([])`, Null},
		{`last(1)`, &object.Error{Message: "argument to `last` must be ARRAY, got INTEGER"}},
		{`rest([1, 2, 3])`, []int{2, 3}},
		{`rest([])`, Null},
		{`push([], 1)`, []int{1}},
		{`push(1, 1)`, &object.Error{Message: "argument to `push` must be ARRAY, got=INTEGER"}},
	}
	runVMTests(t, tests)
}
