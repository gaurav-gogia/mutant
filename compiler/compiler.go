package compiler

import (
	"mutant/ast"
	"mutant/code"
	"mutant/object"
)

type Compiler struct {
	instructions code.Instructions
	constants    []object.Object
}

type ByteCode struct {
	Instructions code.Instructions
	Constants    []object.Object
}

func New() *Compiler {
	return &Compiler{
		instructions: code.Instructions{},
		constants:    []object.Object{},
	}
}

func (c *Compiler) Compile(node ast.Node) error { return nil }

func (c *Compiler) ByteCode() *ByteCode {
	return &ByteCode{
		Instructions: c.instructions,
		Constants:    []object.Object{},
	}
}
