package builtin

import (
	"fmt"
	"mutant/object"
)

func Putln(args ...object.Object) object.Object {
	for _, arg := range args {
		fmt.Println(arg.Inspect())
	}
	return nil
}
