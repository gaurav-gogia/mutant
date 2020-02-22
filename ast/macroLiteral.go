package ast

import (
	"bytes"
	"mutant/token"
	"strings"
)

type MacroLiteral struct {
	Token      token.Token
	Parameters []*Identifier
	Body       *BlockStatement
}

func (ml *MacroLiteral) expressionNode()      {}
func (ml *MacroLiteral) TokenLiteral() string { return ml.Token.Literal }
func (ml *MacroLiteral) String() string {
	var out bytes.Buffer

	params := []string{}
	for _, p := range ml.Parameters {
		params = append(params, p.String())
	}

	out.WriteString(ml.TokenLiteral())
	out.WriteString("(")
	out.WriteString(strings.Join(params, ", "))
	out.WriteString(") ")
	out.WriteString(ml.Body.String())

	return out.String()
}
