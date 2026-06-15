package builtin

import (
	"fmt"
	"mutant/object"
)

func Puts(args ...object.Object) object.Object {
	for _, arg := range args {
		fmt.Print(arg.Inspect())
	}
	return nil
}
