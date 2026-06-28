package ast

import (
	"bytes"
	"mutant/token"
)

type ForStatement struct {
	Token     token.Token
	Init      Statement
	Condition Expression
	Post      Expression
	Body      *BlockStatement
}

func (fs *ForStatement) statementNode()       {}
func (fs *ForStatement) TokenLiteral() string { return fs.Token.Literal }
func (fs *ForStatement) String() string {
	var out bytes.Buffer

	out.WriteString("for (")
	if fs.Init != nil {
		out.WriteString(fs.Init.String())
	}
	out.WriteString("; ")
	if fs.Condition != nil {
		out.WriteString(fs.Condition.String())
	}
	out.WriteString("; ")
	if fs.Post != nil {
		out.WriteString(fs.Post.String())
	}
	out.WriteString(") ")
	if fs.Body != nil {
		out.WriteString(fs.Body.String())
	}

	return out.String()
}
