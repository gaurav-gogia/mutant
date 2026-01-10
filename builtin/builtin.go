package builtin

import (
	"fmt"

	"mutant/object"
)

type BuiltinFunction func(args ...object.Object) object.Object
type BuiltIn struct{ Fn BuiltinFunction }

func (b *BuiltIn) Type() object.ObjectType { return object.BUILTIN_OBJ }
func (b *BuiltIn) Inspect() string         { return "builtin funciton" }

var Builtins = []struct {
	Name    string
	Builtin *BuiltIn
}{
	{"len", &BuiltIn{Len}},
	{"puts", &BuiltIn{Puts}},
	{"putln", &BuiltIn{Putln}},
	{"gets", &BuiltIn{Gets}},
	{"first", &BuiltIn{First}},
	{"last", &BuiltIn{Last}},
	{"rest", &BuiltIn{Rest}},
	{"push", &BuiltIn{Push}},
	{"pop", &BuiltIn{Pop}},
}

func GetBuiltinByName(name string) *BuiltIn {
	for _, fun := range Builtins {
		if name == fun.Name {
			return fun.Builtin
		}
	}
	return nil
}

func newError(format string, a ...interface{}) *object.Error {
	return &object.Error{Message: fmt.Sprintf(format, a...)}
}
