package object

import (
	"fmt"
	"reflect"
)

type BuiltinFunction func(args ...Object) Object
type BuiltIn struct{ Fn BuiltinFunction }

func (b *BuiltIn) Type() ObjectType { return BUILTIN_OBJ }
func (b *BuiltIn) Inspect() string  { return "builtin funciton" }

var Builtins = []struct {
	Name    string
	Builtin *BuiltIn
}{
	{
		Name: "len",
		Builtin: &BuiltIn{
			Fn: func(args ...Object) Object {
				if len(args) != 1 {
					return newError("wrong number of arguments. got=%d, want=1", len(args))
				}
				switch arg := args[0].(type) {
				case *Array:
					return &Integer{Value: int64(len(arg.Elements))}
				case *String:
					return &Integer{Value: int64(len(arg.Value))}
				default:
					return newError("argument to `len` not supported, got %s", args[0].Type())
				}
			},
		},
	},
	{
		Name: "puts",
		Builtin: &BuiltIn{
			Fn: func(args ...Object) Object {
				for _, arg := range args {
					fmt.Print(arg.Inspect())
				}
				return nil
			},
		},
	},
	{
		Name: "gets",
		Builtin: &BuiltIn{
			Fn: func(args ...Object) Object {
				if len(args) != 1 {
					return newError("wrong number of arguments. got=%d, want=1", len(args))
				}

				if args[0] == nil || (reflect.ValueOf(args[0]).Kind() == reflect.Ptr && reflect.ValueOf(args[0]).IsNil()) {
					return newError("argument to function is nil :/")
				}

				if args[0].Type() == STRING_OBJ {
					switch args[0].Inspect() {
					case "boolean":
						fallthrough
					case "bool":
						fallthrough
					case "BOOL":
						fallthrough
					case "BOOLEAN":
						var in bool
						if _, err := fmt.Scanln(&in); err != nil {
							return newError("input does not match declared data type")
						}
						return &Boolean{Value: in}
					case "integer":
						fallthrough
					case "int":
						fallthrough
					case "INT":
						fallthrough
					case "INTEGER":
						var in int64
						if _, err := fmt.Scanln(&in); err != nil {
							return newError("input does not match declared data type")
						}
						return &Integer{Value: in}
					case "string":
						fallthrough
					case "str":
						fallthrough
					case "STR":
						fallthrough
					case "STRING":
						var in string
						if _, err := fmt.Scanln(&in); err != nil {
							return newError("input does not match declared data type")
						}
						return &String{Value: in}
					default:
						return nil
					}
				}

				return newError("argument to `gets` must be a string declaring data type for user input. Ex: BOOLEAN or INTEGER or STRING, got %s", args[0].Type())
			},
		},
	},
	{
		Name: "first",
		Builtin: &BuiltIn{
			Fn: func(args ...Object) Object {
				if len(args) != 1 {
					return newError("wrong number of arguments. got=%d, want=1", len(args))
				}

				if args[0].Type() != ARRAY_OBJ {
					return newError("argument to `first` must be ARRAY, got %s", args[0].Type())
				}

				arr := args[0].(*Array)
				if len(arr.Elements) > 0 {
					return arr.Elements[0]
				}

				return nil
			},
		},
	},
	{
		Name: "last",
		Builtin: &BuiltIn{
			Fn: func(args ...Object) Object {
				if len(args) != 1 {
					return newError("wrong number of arguments. got=%d, want=1", len(args))
				}

				if args[0].Type() != ARRAY_OBJ {
					return newError("argument to `last` must be ARRAY, got %s", args[0].Type())
				}

				arr := args[0].(*Array)
				length := len(arr.Elements)
				if length > 0 {
					return arr.Elements[length-1]
				}

				return nil
			},
		},
	},
	{
		Name: "rest",
		Builtin: &BuiltIn{
			Fn: func(args ...Object) Object {
				if len(args) != 1 {
					return newError("wrong number of arguments. got=%d, want=1", len(args))
				}
				if args[0].Type() != ARRAY_OBJ {
					return newError("argument to `rest` must be ARRAY, got=%s", args[0].Type())
				}

				arr := args[0].(*Array)
				length := len(arr.Elements)
				if length > 1 {
					newElements := make([]Object, length-1, length-1)
					copy(newElements, arr.Elements[1:length])
					return &Array{Elements: newElements}
				}
				return nil
			},
		},
	},
	{
		Name: "push",
		Builtin: &BuiltIn{
			Fn: func(args ...Object) Object {
				if len(args) != 2 {
					return newError("wrong number of arguments. got=%d, want=2", len(args))
				}

				if args[0].Type() != ARRAY_OBJ {
					return newError("argument to `push` must be ARRAY, got=%s", args[0].Type())
				}

				arr := args[0].(*Array)
				length := len(arr.Elements)

				newElements := make([]Object, length+1, length+1)
				copy(newElements, arr.Elements)
				newElements[length] = args[1]

				return &Array{Elements: newElements}
			},
		},
	},
}

func GetBuiltinByName(name string) *BuiltIn {
	for _, fun := range Builtins {
		if name == fun.Name {
			return fun.Builtin
		}
	}
	return nil
}

func newError(format string, a ...interface{}) *Error {
	return &Error{Message: fmt.Sprintf(format, a...)}
}
