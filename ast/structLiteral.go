package ast

import (
	"bytes"
	"mutant/token"
	"strings"
)

type StructFieldValue struct {
	Name  *Identifier
	Value Expression
}

type StructLiteral struct {
	Token  token.Token
	Name   *Identifier
	Fields []*StructFieldValue
}

func (sl *StructLiteral) expressionNode()      {}
func (sl *StructLiteral) TokenLiteral() string { return sl.Token.Literal }
func (sl *StructLiteral) String() string {
	var out bytes.Buffer
	fields := []string{}

	for _, f := range sl.Fields {
		if f == nil || f.Name == nil || f.Value == nil {
			continue
		}
		fields = append(fields, f.Name.String()+": "+f.Value.String())
	}

	if sl.Name != nil {
		out.WriteString(sl.Name.String())
		out.WriteString(" ")
	}
	out.WriteString("{")
	out.WriteString(strings.Join(fields, ", "))
	out.WriteString("}")

	return out.String()
}
