package evaluator

import (
	"mutant/ast"
	"mutant/object"
)

var (
	NULL  = &object.Null{}
	TRUE  = &object.Boolean{Value: true}
	FALSE = &object.Boolean{Value: false}
)

func Eval(n ast.Node) object.Object {
	switch node := n.(type) {
	case *ast.Program:
		return evalProgram(node.Statements)
	case *ast.ExpressionStatement:
		return Eval(node.Expression)
	case *ast.IntegerLiteral:
		return &object.Integer{Value: node.Value}
	case *ast.Boolean:
		return &object.Boolean{Value: node.Value}
	case *ast.PrefixExpression:
		right := Eval(node.Right)
		return evalPrefixExpression(node.Operator, right)
	case *ast.InfixExpression:
		left := Eval(node.Left)
		right := Eval(node.Right)
		return evalInfixExpression(node.Operator, left, right)
	case *ast.BlockStatement:
		return evalBlockStatement(node)
	case *ast.IfExpression:
		return evalIfExpression(node)
	case *ast.ReturnStatement:
		val := Eval(node.ReturnValue)
		return &object.ReturnValue{Value: val}
	}
	return nil
}

func evalProgram(stmts []ast.Statement) object.Object {
	var res object.Object
	for _, s := range stmts {
		res = Eval(s)

		if returnValue, ok := res.(*object.ReturnValue); ok {
			return returnValue.Value
		}
	}
	return res
}

func evalBlockStatement(block *ast.BlockStatement) object.Object {
	var res object.Object
	for _, stmt := range block.Statements {
		res = Eval(stmt)
		if (res != nil) && (res.Type() == object.RETURN_VALUE_OBJ) {
			return res
		}
	}
	return res
}

func nativeBoolToBoolObject(input bool) *object.Boolean {
	if input {
		return TRUE
	}
	return FALSE
}

func evalPrefixExpression(operator string, right object.Object) object.Object {
	switch operator {
	case "!":
		return evalBangOperatorExpression(right)
	case "-":
		return evalMinusOperatorExpression(right)
	default:
		return NULL
	}
}

func evalBangOperatorExpression(right object.Object) object.Object {
	switch right.Inspect() {
	case TRUE.Inspect():
		return FALSE
	case FALSE.Inspect():
		return TRUE
	case NULL.Inspect():
		return TRUE
	default:
		return FALSE
	}
}

func evalMinusOperatorExpression(right object.Object) object.Object {
	if right.Type() != object.INTEGER_OBJ {
		return NULL
	}
	value := right.(*object.Integer).Value
	return &object.Integer{Value: -value}
}

func evalInfixExpression(operator string, left, right object.Object) object.Object {
	switch {
	case left.Type() == object.INTEGER_OBJ && right.Type() == object.INTEGER_OBJ:
		return evalIntegerInfixExpression(operator, left, right)
	case operator == "==":
		return nativeBoolToBoolObject(left.Inspect() == right.Inspect())
	case operator == "!=":
		return nativeBoolToBoolObject(left.Inspect() != right.Inspect())
	default:
		return NULL
	}
}

func evalIntegerInfixExpression(operator string, left, right object.Object) object.Object {
	leftVal := left.(*object.Integer).Value
	rightVal := right.(*object.Integer).Value

	switch operator {
	case "+":
		return &object.Integer{Value: leftVal + rightVal}
	case "-":
		return &object.Integer{Value: leftVal - rightVal}
	case "*":
		return &object.Integer{Value: leftVal * rightVal}
	case "/":
		return &object.Integer{Value: leftVal / rightVal}
	case "<":
		return nativeBoolToBoolObject(leftVal < rightVal)
	case ">":
		return nativeBoolToBoolObject(leftVal > rightVal)
	case "==":
		return nativeBoolToBoolObject(leftVal == rightVal)
	case "!=":
		return nativeBoolToBoolObject(leftVal != rightVal)
	}

	return NULL
}

func evalIfExpression(node *ast.IfExpression) object.Object {
	condition := Eval(node.Condition)
	if isTruthy(condition) {
		return Eval(node.Consequence)
	} else if node.Alternative != nil {
		return Eval(node.Alternative)
	}

	return NULL
}

func isTruthy(obj object.Object) bool {
	switch obj {
	case NULL:
		return false
	case TRUE:
		return true
	case FALSE:
		return false
	default:
		return true
	}
}
