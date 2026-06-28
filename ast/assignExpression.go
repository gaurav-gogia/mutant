package ast

import (
	"bytes"
	"mutant/token"
)

type AssignExpression struct {
	Token token.Token
	Left  Expression
	Value Expression
}

func (ae *AssignExpression) expressionNode()      {}
func (ae *AssignExpression) TokenLiteral() string { return ae.Token.Literal }
func (ae *AssignExpression) String() string {
	var out bytes.Buffer

	out.WriteString("(")
	if ae.Left != nil {
		out.WriteString(ae.Left.String())
	}
	out.WriteString(" = ")
	if ae.Value != nil {
		out.WriteString(ae.Value.String())
	}
	out.WriteString(")")

	return out.String()
}
