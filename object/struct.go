package object

import (
	"bytes"
	"fmt"
	"strings"
)

type Struct struct {
	TypeName string
	Fields   map[string]Object
}

func (s *Struct) Type() ObjectType {
	return STRUCT_OBJ
}

func (s *Struct) Inspect() string {
	var out bytes.Buffer
	fields := []string{}

	for k, v := range s.Fields {
		fields = append(fields, fmt.Sprintf("%s: %s", k, v.Inspect()))
	}

	out.WriteString(s.TypeName)
	out.WriteString(" { ")
	out.WriteString(strings.Join(fields, ", "))
	out.WriteString(" }")

	return out.String()
}
