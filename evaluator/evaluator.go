package evaluator

import (
	"mutant/ast"
	"mutant/object"
)

func Eval(n ast.Node) object.Object {
	switch node := n.(type) {
	case *ast.Program:
		return evalStatements(node.Statements)
	case *ast.ExpressionStatement:
		return Eval(node.Expression)
	case *ast.IntegerLiteral:
		return &object.Integer{Value: node.Value}
	case *ast.Boolean:
		return &object.Boolean{Value: node.Value}
	}
	return nil
}

func evalStatements(stmts []ast.Statement) object.Object {
	var res object.Object
	for _, s := range stmts {
		res = Eval(s)
	}

	return res
}
