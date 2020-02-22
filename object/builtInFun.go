package object

type BuiltinFunction func(args ...Object) Object
type BuiltIn struct{ Fn BuiltinFunction }

func (b *BuiltIn) Type() ObjectType { return BUILTIN_OBJ }
func (b *BuiltIn) Inspect() string  { return "builtin funciton" }
