package lexer

import (
	"mutant/token"
	"unicode"
)

// Lexer is the data structure for our lexer
// It performs lexical analysis and tokenizes code
type Lexer struct {
	input        string
	position     int // current character index
	readPosition int // next character index
	ch           rune
}

// New function initializes our lexer, takes input as a string
// that input is the source code
func New(input string) *Lexer {
	l := &Lexer{input: input}
	l.readRune()
	return l
}

// NextToken method makes use of lexer data structure
// Uses switch cases to identify whether a certain character
// in source code is legal or not. Mutant language only
// supports ascii characters
func (l *Lexer) NextToken() token.Token {
	var tok token.Token

	l.skipWhiteSpace()

	switch l.ch {
	case '=':
		if l.peekRune() == '=' {
			ch := string(l.ch)
			l.readRune()
			tok = token.Token{Type: token.EQUALITY, Literal: ch + string(l.ch)}
		} else {
			tok = newToken(token.ASSIGN, l.ch)
		}
	case '+':
		tok = newToken(token.PLUS, l.ch)
	case '-':
		tok = newToken(token.MINUS, l.ch)
	case '*':
		tok = newToken(token.ASTERISK, l.ch)
	case '/':
		tok = newToken(token.FSLASH, l.ch)
	case '\\':
		tok = newToken(token.FSLASH, l.ch)
	case '<':
		tok = newToken(token.LT, l.ch)
	case '>':
		tok = newToken(token.GT, l.ch)
	case '!':
		if l.peekRune() == '=' {
			ch := string(l.ch)
			l.readRune()
			tok = token.Token{Type: token.INEQUALITY, Literal: ch + string(l.ch)}
		} else {
			tok = newToken(token.BANG, l.ch)
		}
	case '(':
		tok = newToken(token.LPAREN, l.ch)
	case ')':
		tok = newToken(token.RPAREN, l.ch)
	case '{':
		tok = newToken(token.LBRACE, l.ch)
	case '}':
		tok = newToken(token.RBRACE, l.ch)
	case '[':
		tok = newToken(token.LSQUARE, l.ch)
	case ']':
		tok = newToken(token.RSQUARE, l.ch)
	case ',':
		tok = newToken(token.COMMA, l.ch)
	case ':':
		tok = newToken(token.COLON, l.ch)
	case ';':
		tok = newToken(token.SEMICOLON, l.ch)
	case 0:
		tok = newToken(token.EOF, l.ch)
	case '"':
		tok.Type = token.STRING
		tok.Literal = l.readString()
	default:
		if unicode.IsLetter(l.ch) {
			tok.Literal = l.readIdentifier()
			tok.Type = token.LookupIdent(tok.Literal)
			return tok
		} else if unicode.IsNumber(l.ch) {
			tok.Literal = l.readNumber()
			tok.Type = token.INT
			return tok
		}
		tok = newToken(token.ILLEGAL, l.ch)
	}

	l.readRune()

	return tok
}

func (l *Lexer) readRune() {
	if l.readPosition >= len(l.input) {
		l.ch = 0
	} else {
		l.ch = rune(l.input[l.readPosition])
	}

	l.position = l.readPosition
	l.readPosition++
}

func (l *Lexer) readString() string {
	position := l.position + 1
	for {
		l.readRune()
		if l.ch == '"' || l.ch == 0 {
			break
		}
	}
	return l.input[position:l.position]
}

func newToken(tokenType token.TokenType, ch rune) token.Token {
	var tok token.Token

	tok.Type = tokenType
	tok.Literal = string(ch)

	return tok
}

func (l *Lexer) readIdentifier() string {
	position := l.position
	for unicode.IsLetter(l.ch) {
		l.readRune()
	}
	return l.input[position:l.position]
}

func (l *Lexer) readNumber() string {
	position := l.position
	for unicode.IsDigit(l.ch) {
		l.readRune()
	}
	return l.input[position:l.position]
}

func (l *Lexer) skipWhiteSpace() {
	for unicode.IsSpace(l.ch) {
		l.readRune()
	}
}

func (l *Lexer) peekRune() rune {
	if l.readPosition >= len(l.input) {
		return 0
	}
	return rune(l.input[l.readPosition])
}
