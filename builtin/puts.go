package builtin

import (
	"fmt"
	"mutant/object"
)

func Putf(args ...object.Object) object.Object {
	for _, arg := range args {
		fmt.Print(arg.Inspect())
	}
	return nil
}
