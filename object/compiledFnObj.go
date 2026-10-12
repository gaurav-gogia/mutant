package object

import (
	"fmt"
	"mutant/code"
	"strings"
)

type CompiledFunction struct {
	Instructions code.Instructions
	NumLocals    int
	NumParams    int

	// CapturedLocals are the local slot indices an inner function closes over,
	// ascending. The VM boxes exactly these slots when it pushes the frame --
	// after the arguments are in place, so a captured *parameter* needs no
	// special case -- and the compiler has already rewritten every read and
	// write of them to the cell opcodes.
	//
	// This is semantic, not debug information, and must never join
	// ByteCode.StripDebugInfo's list: a release build that lost it would stop
	// boxing and quietly resume the by-value capture bug it exists to fix.
	// Empty is not "unknown", it is "this function has no captured locals",
	// which is also what every .mu compiled before boxing existed says -- and
	// correctly so, since none of them can contain a cell.
	CapturedLocals []int

	// Name is the function's source name, or empty for an anonymous literal.
	// It exists so a traceback can say which function a frame is in; nothing
	// resolves or dispatches on it.
	Name string

	// Params are the declared parameter names, in order. They exist so a
	// traceback can print the arguments a frame was called with: the VM holds
	// them on the stack at the frame's base pointer, but only the compiler
	// knows what they were called. Empty for a function compiled without
	// debug info, in which case a frame renders its arguments positionally.
	//
	// len(Params) is NumParams for every function this compiler produces; the
	// renderer does not assume it and pairs whichever is shorter.
	Params []string

	// LineTable maps an offset in Instructions back to the line and column
	// that produced it. MacroTable does the same for the macro definition an
	// instruction was expanded from, and is populated only for the stretches
	// of the stream that came out of a macro.
	//
	// Both are empty for a function compiled before positions existed and for
	// one whose positions were deliberately removed -- see
	// compiler.ByteCode.StripDebugInfo. A consumer cannot tell those apart,
	// which is intended.
	LineTable  code.LineTable
	MacroTable code.LineTable

	// EndTable maps the same offsets to where the source construct ends, in
	// the same encoding. LineTable alone gives a point, which is enough to
	// name a line and no help at all in `total / count(xs)` -- two divisions
	// and one column. With both ends the reporter can underline the span that
	// failed, the way rustc and Python 3.11 do.
	EndTable code.LineTable

	// LocalNames are the function's local slots, indexed by slot number, each
	// entry the name declared there. The VM addresses a local by its index and
	// never reads this; a debugger stopped in the frame has nothing else to
	// call slot 3 but "slot 3".
	//
	// An entry is empty where the slot has no name to report, which happens
	// when a later declaration takes the name over: `let x = 1; let x = 2;`
	// allocates two slots and only the second still answers to `x`. Reporting
	// the first as `x` too would point a watch window at the wrong value.
	//
	// Parameters occupy the first NumParams slots, so LocalNames covers them
	// as well and agrees with Params there. Params is kept because it is what
	// a traceback prints and it predates this table.
	//
	// Debug info: nil for a function compiled before this existed and for one
	// stripped of positions, and the two are indistinguishable on purpose.
	LocalNames []string
}

func (cf *CompiledFunction) Type() ObjectType { return COMPILED_FN_OBJ }

// Inspect renders the function as its source declared it: `fn mean(values)`
// for one bound to a name, `fn(values)` for an anonymous literal. It never
// prints an address: one made a program that ends on a function print
// something different on every run, and told a reader nothing about which
// function it was (M26-EVL-002).
//
// Name and Params are debug info, and a release build strips both -- see
// compiler.ByteCode.StripDebugInfo. The parameter count survives, so such a
// function renders as `fn(<2 parameters>)`; the angle brackets keep that from
// reading as a parameter list a source file could have written.
//
// This names a function for a reader and is not its identity: two closures
// over one function render alike, and so do two anonymous literals that take
// the same parameters.
func (cf *CompiledFunction) Inspect() string {
	if cf == nil {
		return "fn()"
	}

	var out strings.Builder
	out.WriteString("fn")
	if cf.Name != "" {
		out.WriteString(" ")
		out.WriteString(cf.Name)
	}
	out.WriteString("(")
	switch {
	case len(cf.Params) > 0:
		out.WriteString(strings.Join(cf.Params, ", "))
	case cf.NumParams == 1:
		out.WriteString("<1 parameter>")
	case cf.NumParams > 1:
		fmt.Fprintf(&out, "<%d parameters>", cf.NumParams)
	}
	out.WriteString(")")
	return out.String()
}
