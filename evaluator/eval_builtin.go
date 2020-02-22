package evaluator

import (
	"mutant/object"
)

var builtins = map[string]*object.BuiltIn{
	"len": &object.BuiltIn{
		Fn: func(args ...object.Object) object.Object {
			return NULL
		},
	},
}
