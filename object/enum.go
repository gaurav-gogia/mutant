package object

import "fmt"

type EnumValue struct {
	TypeName string
	Tag      string
	Value    Object
}

func (ev *EnumValue) Type() ObjectType {
	return ENUM_VALUE_OBJ
}

func (ev *EnumValue) Inspect() string {
	if ev.Value == nil || ev.Value.Type() == NULL_OBJ {
		return fmt.Sprintf("%s.%s", ev.TypeName, ev.Tag)
	}
	return fmt.Sprintf("%s.%s(%s)", ev.TypeName, ev.Tag, ev.Value.Inspect())
}
