package ast

import "mutant/token"

type ReturnStatement struct {
	Token       token.Token // RETURN token
	ReturnValue Expression
}

func (rs *ReturnStatement) statementNode()       {}
func (rs *ReturnStatement) TokenLiteral() string { return rs.Token.Literal }
