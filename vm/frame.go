package vm

import (
	"mutant/code"
	"mutant/object"
)

type Frame struct {
	cl *object.Closure
	ip int
	bp int
}

func NewFrame(cl *object.Closure, basePointer int) *Frame {
	return &Frame{cl: cl, ip: -1, bp: basePointer}
}
func (f *Frame) Instructions() code.Instructions { return f.cl.Fn.Instructions }
