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
	"putln":          builtin.GetBuiltinByName("putln"),
	"debug_status":   builtin.GetBuiltinByName("debug_status"),
	"sandbox_status": builtin.GetBuiltinByName("sandbox_status"),
	"exec_string":    builtin.GetBuiltinByName("exec_string"),
	"cmd_builder":    builtin.GetBuiltinByName("cmd_builder"),
	"cmd_add":        builtin.GetBuiltinByName("cmd_add"),
	"cmd_run":        builtin.GetBuiltinByName("cmd_run"),
}
