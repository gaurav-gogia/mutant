package sema_test

import (
	"fmt"
	"testing"

	"mutant/ast"
	"mutant/compiler"
	"mutant/evaluator"
	"mutant/global"
	"mutant/lexer"
	"mutant/mutil"
	"mutant/object"
	"mutant/parser"
	"mutant/security"
	"mutant/sema"
	"mutant/vm"
)

// Which names of a multi-name let hold a function is decided at run time, by
// how destructureValues splits the value, and the walk does not model that
// split. So whatever kind the graph gives such a name has to be true of what
// the program puts in it, and the only way to know that is to run the program.
// That is why this file is package sema_test: the compiler, the VM and the
// evaluator all import sema.
//
// A function is true of a name that holds one and a value is true of every
// name, so the rule is narrow: the graph may call a destructured name a
// function only if both engines put a function there. The table also pins the
// split itself, because letStatement gives it as the reason every such name is
// a value, and a reason nothing checks can stop being true unnoticed.

func TestADestructuredNameIsAFunctionOnlyIfItHoldsOne(t *testing.T) {
	for _, c := range []struct {
		src   string
		holds map[string]string // what each name holds at run time
	}{
		{
			// Neither an array nor a multi-value, so the value goes whole into
			// the first name. Handing both names the literal's kind gave b a's
			// function card.
			src:   "let a, b = fn(x) { return x * 2; };\n",
			holds: map[string]string{"a": "function", "b": "null"},
		},
		{
			// The same split for a function that is named rather than written.
			src:   "let double = fn(x) { return x * 2; };\nlet a, b = double;\n",
			holds: map[string]string{"a": "function", "b": "null"},
		},
		{
			// An array goes element by element, so a name after the first can
			// hold a function too.
			src:   "let a, b = [fn(x) { return x * 2; }, fn(x) { return x + 1; }];\n",
			holds: map[string]string{"a": "function", "b": "function"},
		},
		{
			// And so does a multi-value.
			src:   "let pair = fn() { return 1, fn(x) { return x * 2; }; };\nlet a, b = pair();\n",
			holds: map[string]string{"a": "integer", "b": "function"},
		},
	} {
		graph := sema.BuildFile("", destructuredParse(t, c.src), nil, nil)
		for _, name := range []string{"a", "b"} {
			probe := c.src + name + ";\n"
			onVM := destructuredHolds(destructuredRunVM(t, probe))
			onEvaluator := destructuredHolds(destructuredRunEvaluator(t, probe))
			if onVM != c.holds[name] || onEvaluator != c.holds[name] {
				t.Errorf("%q: %s holds %s on the VM and %s on the evaluator, want %s",
					c.src, name, onVM, onEvaluator, c.holds[name])
			}

			node := destructuredFind(t, graph, name)
			switch node.Kind {
			case sema.KindValue:
			case sema.KindFunction:
				if onVM != "function" || onEvaluator != "function" {
					t.Errorf("%q: the graph calls %s a function, and it holds %s on the VM and %s on the evaluator",
						c.src, name, onVM, onEvaluator)
				}
			default:
				t.Errorf("%q: the graph calls %s a %s, which no let declares", c.src, name, node.Kind)
			}
		}
	}
}

func destructuredParse(t *testing.T, src string) *ast.Program {
	t.Helper()
	p := parser.New(lexer.New(src))
	program := p.ParseProgram()
	if errs := p.Errors(); len(errs) > 0 {
		t.Fatalf("%q does not parse: %v", src, errs)
	}
	return program
}

// destructuredRunVM runs src the way a program runs, as vm/vm_test.go's
// runVMTests does: the bytecode encrypted under the password derived from it.
func destructuredRunVM(t *testing.T, src string) object.Object {
	t.Helper()
	comp := compiler.New()
	if err := comp.Compile(destructuredParse(t, src)); err != nil {
		t.Fatalf("%q does not compile: %s", src, err)
	}
	byteCode := comp.ByteCode()
	password := fmt.Sprint(security.DerivePasswordFromInstructions(byteCode.Instructions))
	byteCode = mutil.EncryptByteCode(byteCode, password)
	machine := vm.NewWithGlobalStoreAndPassword(byteCode, make([]object.Object, global.GlobalSize), password)
	if err := machine.Run(); err != nil {
		t.Fatalf("%q fails on the VM: %s", src, err)
	}
	return machine.LastPoppedStackElement()
}

func destructuredRunEvaluator(t *testing.T, src string) object.Object {
	t.Helper()
	result := evaluator.Eval(destructuredParse(t, src), object.NewEnvironment())
	if failure, failed := result.(*object.Error); failed {
		t.Fatalf("%q fails on the evaluator: %s", src, failure.Inspect())
	}
	return result
}

// destructuredHolds names what a value is, in the words the table uses. The VM
// holds a function as a closure and the evaluator as a function object, and
// both are a function here.
func destructuredHolds(value object.Object) string {
	if value == nil {
		return "nothing"
	}
	switch value.Type() {
	case object.CLOSURE_OBJ, object.COMPILED_FN_OBJ, object.FUNCTION_OBJ, object.BUILTIN_OBJ:
		return "function"
	case object.NULL_OBJ:
		return "null"
	case object.INTEGER_OBJ:
		return "integer"
	}
	return string(value.Type())
}

func destructuredFind(t *testing.T, g *sema.Graph, name string) *sema.Node {
	t.Helper()
	for _, node := range g.Declarations() {
		if node.Name == name {
			return node
		}
	}
	t.Fatalf("nothing named %q is declared", name)
	return nil
}
