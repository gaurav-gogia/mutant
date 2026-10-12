package parser

import (
	"fmt"
	"maps"
	"strings"

	"mutant/ast"
	"mutant/lexer"
	"mutant/token"
)

// parseTemplateLiteral builds the node for a string literal with ${...} holes.
//
// The lexer has already cut the literal into text and hole parts and recorded
// where each one starts, so all that is left here is to parse each hole's
// source as an expression and keep the text between them.
func (p *Parser) parseTemplateLiteral() ast.Expression {
	start := p.startMark()
	lit := &ast.TemplateLiteral{Token: p.curToken, Texts: []string{""}}

	for _, part := range p.curToken.Parts {
		if !part.Expression {
			// Text accumulates into the slot after the last accepted hole.
			// Two text parts in a row only happen when a hole between them
			// failed to parse, and merging them is right: the program is
			// already being rejected.
			lit.Texts[len(lit.Texts)-1] += part.Text
			continue
		}
		hole := p.parseHole(part)
		if hole == nil {
			continue
		}
		lit.Parts = append(lit.Parts, hole)
		lit.Texts = append(lit.Texts, "")
	}

	p.recordRange(lit, start)
	return lit
}

// parseHole parses one ${...} hole's source as a single expression.
//
// The sub-lexer is told where the hole's text sits in this file, so every
// position it records is a file position in all three coordinates and nothing
// needs correcting afterwards.
//
// It used to be given padding instead -- a newline per line and a space per
// column, lexed along with the text -- and that was wrong twice. It was
// quadratic, because each hole paid for padding as long as its own column and
// N holes on one line are a sum (M26-LEX-009). And it bought only two of the
// three coordinates: one byte of padding stands in for a whole line, so every
// offset came out short by the difference and had to be shifted back by
// whoever remembered to, which the typed errors and the recoverables below
// never were (M26-LEX-018).
//
// It is parsed as an expression rather than as a program on purpose. A hole
// holds no statement, so nothing in it should be asked for a terminating
// semicolon -- asking would put a missing-`;` complaint on every interpolated
// string in the file.
func (p *Parser) parseHole(part token.StringPart) ast.Expression {
	if strings.TrimSpace(part.Text) == "" {
		p.appendError(p.curToken, fmt.Sprintf(
			"empty ${} at line %d:%d: a hole needs an expression between the braces, and a literal ${ is written \\${",
			part.Start.Line, part.Start.Column))
		return nil
	}

	sub := New(lexer.NewAt(part.Text, part.Start))

	// The sub-parser recurses on this parser's stack, so it inherits the depth
	// already spent. Without this a hole inside a hole inside a hole is the
	// same stack with the counter reset to zero at every level, and the bound
	// bounds nothing.
	sub.depth = p.depth

	if statementKeyword(sub.curToken.Type) {
		p.appendError(p.curToken, fmt.Sprintf(
			"%s at line %d:%d starts a statement, and a hole has to be an expression: it has to produce the value that goes into the string",
			sub.curToken.Type, part.Start.Line, part.Start.Column))
		return nil
	}

	expression := sub.parseExpression(LOWEST)
	if expression != nil && !sub.peekTokenIs(token.EOF) {
		p.appendError(p.curToken, fmt.Sprintf(
			"${%s} at line %d:%d holds more than one expression: a hole produces one value",
			strings.TrimSpace(part.Text), part.Start.Line, part.Start.Column))
		expression = nil
	}

	// The sub-parser's plain-text errors are marked as coming from inside a
	// hole, which is worth saying because the author is looking at a string
	// literal. They used to carry the hole's line and column written in as
	// well; Parser.PositionedErrors now prefixes every message with its own
	// position, and the hole's start is a different place from the mistake, so
	// writing it here printed two positions and sent the reader to the wrong
	// one of them.
	//
	// The typed errors and the recoverables need no help at all, and that is
	// now true of their offsets as well as of their lines and columns: NewAt
	// put the sub-lexer at the hole's own position, so there is nothing left to
	// rebase.
	for _, msg := range sub.errors {
		p.errors = append(p.errors, fmt.Sprintf("inside ${...}: %s", msg))
	}
	p.typedErrors = append(p.typedErrors, sub.typedErrors...)
	p.recoverables = append(p.recoverables, sub.recoverables...)

	if expression == nil {
		return nil
	}

	// Taken as they are. This was a loop that shifted each range as it copied
	// it; there is nothing to shift now, and a copy is all it ever was.
	if p.nodeRanges == nil {
		p.nodeRanges = make(map[ast.Node]ast.Range)
	}
	maps.Copy(p.nodeRanges, sub.nodeRanges)

	return expression
}

// statementKeyword reports whether a token can only begin a statement. A hole
// starting with one gets a message about what a hole is, rather than the
// parser's "no prefix parse function for LET".
func statementKeyword(t token.TokenType) bool {
	switch t {
	case token.LET, token.RETURN, token.FOR, token.BREAK, token.CONTINUE,
		token.STRUCT, token.ENUM, token.IMPORT:
		return true
	}
	return false
}
