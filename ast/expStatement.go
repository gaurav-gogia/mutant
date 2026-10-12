package ast

import (
	"mutant/token"
)

type ExpressionStatement struct {
	Token      token.Token // first token of expression
	Expression Expression
}

func (es *ExpressionStatement) statementNode() {}

// RequiresSemicolon is true for every expression statement except the two
// that end in a closing brace. `if` and `match` are expressions in Mutant, but
// when one stands alone as a statement its canonical form ends with the brace
// of a block or of the arm list, so no terminator is required.
//
// The formatter emits terminators from this answer rather than copying them
// from the source, so getting it wrong rewrites the user's file rather than
// reporting anything. Not required is not the same as never written: an `if`
// or `match` statement followed by one that begins with `(`, `[`, `-` or `{`
// keeps its `;`, because without it the parser reads the second statement as
// a continuation of the first (lsp/internal/server continuesExpression).
func (es *ExpressionStatement) RequiresSemicolon() bool {
	// A statement with no expression is the residue of a parse error; it is
	// never printed, so demanding a terminator would only add noise.
	if es == nil || es.Expression == nil {
		return false
	}
	switch es.Expression.(type) {
	case *IfExpression, *MatchExpression:
		return false
	}
	return true
}
func (es *ExpressionStatement) TokenLiteral() string { return es.Token.Literal }
func (es *ExpressionStatement) String() string {
	if es == nil {
		return missingNode
	}
	return render(es.Expression)
}
