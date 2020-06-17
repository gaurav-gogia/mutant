package object

import (
	"fmt"
	"mutant/code"
)

type CompiledFunction struct {
	Instructions code.Instructions
	NumLocals    int
}

func (cf *CompiledFunction) Type() ObjectType { return COMPILED_FN_OBJ }
func (cf *CompiledFunction) Inspect() string  { return fmt.Sprintf("Compiled Function[%p]", cf) }
