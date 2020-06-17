package vm

import (
	"mutant/code"
	"mutant/object"
)

type Frame struct {
	fn *object.CompiledFunction
	ip int
	bp int
}

func NewFrame(fn *object.CompiledFunction, basePointer int) *Frame {
	return &Frame{fn: fn, ip: -1, bp: basePointer}
}
func (f *Frame) Instructions() code.Instructions { return f.fn.Instructions }
