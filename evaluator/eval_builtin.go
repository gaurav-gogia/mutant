package evaluator

import (
	"mutant/builtin"
)

var builtins = map[string]*builtin.BuiltIn{
	"len":            builtin.GetBuiltinByName("len"),
	"first":          builtin.GetBuiltinByName("first"),
	"last":           builtin.GetBuiltinByName("last"),
	"rest":           builtin.GetBuiltinByName("rest"),
	"push":           builtin.GetBuiltinByName("push"),
	"putf":           builtin.GetBuiltinByName("putf"),
	"debug_status":   builtin.GetBuiltinByName("debug_status"),
	"sandbox_status": builtin.GetBuiltinByName("sandbox_status"),
}
