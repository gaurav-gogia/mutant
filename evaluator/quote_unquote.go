package evaluator

import (
	"mutant/ast"
	"mutant/object"
)

func quote(node ast.Node) object.Object {
	return &object.Quote{Node: node}
}
