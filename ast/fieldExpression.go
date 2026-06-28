package ast

import (
	"bytes"
	"mutant/token"
)

type FieldExpression struct {
	Token token.Token
	Left  Expression
	Field *Identifier
}

func (fe *FieldExpression) expressionNode()      {}
func (fe *FieldExpression) TokenLiteral() string { return fe.Token.Literal }
func (fe *FieldExpression) String() string {
	var out bytes.Buffer

	out.WriteString("(")
	if fe.Left != nil {
		out.WriteString(fe.Left.String())
	}
	out.WriteString(".")
	if fe.Field != nil {
		out.WriteString(fe.Field.String())
	}
	out.WriteString(")")

	return out.String()
}
