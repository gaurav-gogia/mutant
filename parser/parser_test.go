package parser

import (
	"mutant/ast"
	"mutant/lexer"
	"testing"
)

func TestLetStatements(t *testing.T) {
	input :=
		`
			let x = 5 + 5 + 5;
			let y = 10;
			let foobar = 838383;
		`

	l := lexer.New(input)
	p := New(l) // new parser

	program := p.ParseProgram()
	checkParserErrors(t, p)

	if program == nil {
		t.Fatalf("ParseProgram() returned nil")
	}

	if len(program.Statements) != 3 {
		t.Fatalf("program.Statements does not contain 3 statements. Got = %d", len(program.Statements))
	}

	tests := []struct {
		expectedIdentifier string
	}{
		{"x"},
		{"y"},
		{"foobar"},
	}

	for i, tt := range tests {
		stmt := program.Statements[i]
		if !testLetStmt(t, stmt, tt.expectedIdentifier) {
			return
		}
	}
}

func TestReturnStatements(t *testing.T) {
	input :=
		`
			return 5;
			return 151050;
			return 10;
		`

	l := lexer.New(input)
	p := New(l)

	program := p.ParseProgram()
	checkParserErrors(t, p)

	if program == nil {
		t.Fatalf("ParseProgram() returned nil")
	}

	if len(program.Statements) != 3 {
		t.Fatalf("program.Statements does not contain 3 statements. Got = %d", len(program.Statements))
	}

	for _, stmt := range program.Statements {
		returnStatement, ok := stmt.(*ast.ReturnStatement)
		if !ok {
			t.Errorf("stmt not *ast.ReturnStatement.Got = %T", stmt)
			continue
		}

		if returnStatement.TokenLiteral() != "return" {
			t.Errorf("returnStmt.TokenLiteral not 'return', got %q", returnStatement.TokenLiteral())
		}
	}
}
