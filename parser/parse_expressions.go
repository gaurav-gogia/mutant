package parser

import (
	"fmt"
	"mutant/ast"
	"mutant/token"
)

func (p *Parser) parseIfExpression() ast.Expression {
	exp := &ast.IfExpression{Token: p.curToken}
	if !p.expectPeek(token.LPAREN) {
		return nil
	}

	p.nextToken()
	exp.Condition = p.parseExpression(LOWEST)

	if !p.expectPeek(token.RPAREN) {
		return nil
	}
	if !p.expectPeek(token.LBRACE) {
		return nil
	}

	exp.Consequence = p.parseBlockStatement()

	if p.peekTokenIs(token.ELSE) {
		p.nextToken()
		if !p.expectPeek(token.LBRACE) {
			return nil
		}
		exp.Alternative = p.parseBlockStatement()
	}

	return exp
}

func (p *Parser) parsePrefixExpression() ast.Expression {
	// defer untrace(trace("parsePrefixExpression"))

	expression := &ast.PrefixExpression{
		Token:    p.curToken,
		Operator: p.curToken.Literal,
	}

	p.nextToken()
	expression.Right = p.parseExpression(PREFIX)

	return expression
}

func (p *Parser) parseInfixExpression(left ast.Expression) ast.Expression {
	// defer untrace(trace("parseInfixExpression"))

	expression := &ast.InfixExpression{
		Token:    p.curToken,
		Operator: p.curToken.Literal,
		Left:     left,
	}

	prec := p.curPrecedence()
	p.nextToken()
	expression.Right = p.parseExpression(prec)

	return expression
}

func (p *Parser) parseGroupedExpression() ast.Expression {
	p.nextToken()
	exp := p.parseExpression(LOWEST)
	if !p.expectPeek(token.RPAREN) {
		return nil
	}
	return exp
}

func (p *Parser) parseExpression(precedence int) ast.Expression {
	// defer untrace(trace("parseExpression"))

	prefix := p.prefixParseFns[p.curToken.Type]
	if prefix == nil {
		p.notPrefixParseFnError(p.curToken.Type)
		return nil
	}

	leftExp := prefix()

	for !p.peekTokenIs(token.SEMICOLON) && precedence < p.peekPrecedence() {
		infix := p.infixParseFns[p.peekToken.Type]
		if infix == nil {
			return leftExp
		}
		p.nextToken()
		leftExp = infix(leftExp)
	}

	return leftExp
}

func (p *Parser) parseExpressionStatement() *ast.ExpressionStatement {
	// defer untrace(trace("parseExpressionStatement"))

	stmt := &ast.ExpressionStatement{Token: p.curToken}
	stmt.Expression = p.parseExpression(LOWEST)

	if p.peekTokenIs(token.SEMICOLON) {
		p.nextToken()
	}

	return stmt
}

func (p *Parser) parseCallExpression(fun ast.Expression) ast.Expression {
	exp := &ast.CallExpression{Token: p.curToken, Function: fun}
	exp.Arguments = p.parseExpressionList(token.RPAREN)
	return exp
}

func (p *Parser) parseExpressionList(end token.TokenType) []ast.Expression {
	list := []ast.Expression{}

	if p.peekTokenIs(end) {
		p.nextToken()
		return list
	}

	p.nextToken()
	list = append(list, p.parseExpression(LOWEST))

	for p.peekTokenIs(token.COMMA) {
		p.nextToken()
		p.nextToken()
		list = append(list, p.parseExpression(LOWEST))
	}

	if !p.expectPeek(end) {
		return nil
	}

	return list
}

func (p *Parser) parseIndexExpression(left ast.Expression) ast.Expression {
	exp := &ast.IndexExpression{Token: p.curToken, Left: left}
	p.nextToken()
	exp.Index = p.parseExpression(LOWEST)
	if !p.expectPeek(token.RSQUARE) {
		return nil
	}
	return exp
}

func (p *Parser) parseAssignExpression(left ast.Expression) ast.Expression {
	switch left.(type) {
	case *ast.Identifier, *ast.FieldExpression:
	default:
		msg := fmt.Sprintf("invalid assignment target: %T", left)
		p.errors = append(p.errors, msg)
		return nil
	}

	exp := &ast.AssignExpression{Token: p.curToken, Left: left}
	precedence := p.curPrecedence()
	p.nextToken()
	exp.Value = p.parseExpression(precedence - 1)

	return exp
}

func (p *Parser) parseFieldExpression(left ast.Expression) ast.Expression {
	exp := &ast.FieldExpression{Token: p.curToken, Left: left}

	if !p.expectPeek(token.IDENT) {
		return nil
	}

	exp.Field = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}
	return exp
}

func (p *Parser) parseStructLiteralExpression(left ast.Expression) ast.Expression {
	name, ok := left.(*ast.Identifier)
	if !ok {
		return left
	}

	lit := &ast.StructLiteral{Token: p.curToken, Name: name, Fields: []*ast.StructFieldValue{}}

	for !p.peekTokenIs(token.RBRACE) {
		p.nextToken()
		if !p.curTokenIs(token.IDENT) {
			msg := fmt.Sprintf("expected struct literal field identifier, got %s", p.curToken.Type)
			p.errors = append(p.errors, msg)
			return nil
		}

		field := &ast.StructFieldValue{Name: &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}}

		if !p.expectPeek(token.COLON) {
			return nil
		}

		p.nextToken()
		field.Value = p.parseExpression(LOWEST)
		lit.Fields = append(lit.Fields, field)

		if !p.peekTokenIs(token.RBRACE) && !p.expectPeek(token.COMMA) {
			return nil
		}
	}

	if !p.expectPeek(token.RBRACE) {
		return nil
	}

	return lit
}
