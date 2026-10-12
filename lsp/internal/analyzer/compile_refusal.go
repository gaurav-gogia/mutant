package analyzer

// The compiler's refusals, reported by the compiler.
//
// Every other refusal the editor shows is a rule written a second time: the
// undefined-name rule, the assignment-target rule, struct_field.go and
// module_member.go each reproduce one family of the compiler's errors by hand.
// Measured at 2c5f24f, fifteen families had no such rule -- break and continue
// outside a loop, a continue in a for loop's post section, an assignment to a
// builtin or to the function being defined, a for-in binding one name twice, a
// macro literal inside a function, four struct-literal and struct-declaration
// refusals, a write through an import namespace, a struct name declared in two
// modules -- and each drew nothing in the editor and a failed build from
// `mutant gen` (M26-LSP-028).
//
// So the compiler is run on the document, and its first refusal is reported in
// its own words at the node it was compiling. Whatever the compiler learns to
// refuse next is in the editor the day it lands, with no rule to remember.
//
// It runs only where the editor's compile IS the build's compile of this file:
//
//   - The file parses. A program with a parse error is refused before the
//     compiler sees it, and the parser's diagnostics already say so.
//   - No macro is expanded. `mutant gen` expands macros before it compiles,
//     and expanding one runs the program's own code (the evaluator over
//     unquote arguments) -- which the editor must never do on a keystroke. A
//     file that declares a macro at its top level, or calls a name a module
//     in its closure declares as a macro, is therefore not compiled at all.
//     Saying nothing there is the status quo; compiling it unexpanded would
//     refuse the macro call as an undefined name, which the build does not.
//   - Every import is known. A file with imports needs a workspace whose
//     closure for it is complete and clean -- every import indexed, no cycle,
//     no alias bound twice -- because the build stops at load otherwise. The
//     imported modules are not compiled: their exports and types are declared
//     from the workspace facts, which is everything the compile of THIS file
//     reads from them.
//
// These are not lints and take no LintConfig, for the reason
// ModuleMemberDiagnostics gives: a refusal is the language, not a taste.

import (
	"fmt"
	"sort"
	"strings"

	mast "mutant/ast"
	"mutant/builtin"
	"mutant/compiler"
	"mutant/sema"

	lsp "github.com/tliron/glsp/protocol_3_16"
)

// DiagnosticSourceCompiler marks a diagnostic the compiler itself raised.
const DiagnosticSourceCompiler = "mutant-compiler"

// CompileDiagnostics returns the compiler's first refusal of the document, or
// nothing when it compiles or cannot be compiled the way the build compiles it.
//
// reported is what the rules that reproduce a compiler refusal by hand found
// (module members, struct fields, redeclarations, undefined names, assignment
// targets). A refusal one of them already marks inside the range the compiler
// raised it at is not reported twice: those rules report EVERY occurrence,
// where the compiler stops at the first, so theirs is the more useful copy.
func (s *Snapshot) CompileDiagnostics(reported []lsp.Diagnostic) []lsp.Diagnostic {
	message, rng, located, refused := s.compileRefusal()
	if !refused {
		return nil
	}

	at := lsp.Range{}
	if located {
		at = s.Range(rng)
	}
	if located {
		for _, d := range reported {
			if rangeWithin(d.Range, at) {
				return nil
			}
		}
	}

	severity := lsp.DiagnosticSeverityError
	source := DiagnosticSourceCompiler
	return []lsp.Diagnostic{{
		Range:    at,
		Severity: &severity,
		Source:   &source,
		Message:  message,
	}}
}

// compileRefusal compiles the document and returns the first line of what the
// compiler refused, and where.
func (s *Snapshot) compileRefusal() (message string, rng mast.Range, located, refused bool) {
	if s == nil || s.Program == nil || len(s.ParseErrors) > 0 {
		return "", mast.Range{}, false, false
	}
	if declaresAMacro(s.Program) || s.callsAClosureMacro() {
		return "", mast.Range{}, false, false
	}
	comp, ok := s.compilerForDocument()
	if !ok {
		return "", mast.Range{}, false, false
	}

	err := compileRecovering(comp, s.Program)
	if err == nil {
		return "", mast.Range{}, false, false
	}
	rng, located = compiler.ErrorRange(err)
	return strings.SplitN(err.Error(), "\n", 2)[0], rng, located, true
}

// compileRecovering turns a compiler panic into an error the editor can show.
// A panic here is a compiler defect `mutant gen` would hit on the same file,
// and it must not cost the author every lint finding beside it.
func compileRecovering(comp *compiler.Compiler, program *mast.Program) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("the compiler failed on this file with an internal error (%v); "+
				"`mutant gen` will fail the same way -- please report it", recovered)
		}
	}()
	return comp.Compile(program)
}

// compilerForDocument returns a compiler set up the way buildByteCode sets one
// up for this file as the entry, or false when the editor cannot know that.
func (s *Snapshot) compilerForDocument() (*compiler.Compiler, bool) {
	table := compiler.NewSymbolTable()
	for i, b := range builtin.Builtins {
		table.DefineBuiltin(i, b.Name)
	}
	comp := compiler.NewWithState(table, nil)

	if !fileImportsAnything(s) {
		return comp, true
	}

	ws, key := s.workspace, s.ModuleKey
	if ws == nil || key == "" {
		return nil, false
	}
	closure, cycles := ws.Closure(key)
	if len(cycles) > 0 {
		return nil, false
	}
	for _, k := range closure {
		if _, known := ws.Facts(k); !known {
			return nil, false
		}
		if len(ws.Diagnostics(k)) > 0 {
			return nil, false
		}
	}

	for _, k := range closure {
		if k == key {
			continue
		}
		facts, _ := ws.Facts(k)
		comp.EnterModule(compiler.ModuleScope{Key: k, Display: facts.Path, Namespaces: ws.Namespaces(k)})
		names := make([]string, 0, len(facts.Exports))
		for name := range facts.Exports {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			table.Define(name)
		}
		typeNames := make([]string, 0, len(facts.Types))
		for name := range facts.Types {
			typeNames = append(typeNames, name)
		}
		sort.Strings(typeNames)
		for _, name := range typeNames {
			if err := comp.Compile(typeStatementFor(facts.Types[name])); err != nil {
				// Two modules of the closure collide with each other: the build
				// stops in one of them, before it reaches this file.
				return nil, false
			}
		}
	}

	facts, _ := ws.Facts(key)
	display := ""
	if facts != nil {
		display = facts.Path
	}
	comp.EnterModule(compiler.ModuleScope{Key: key, Display: display, Namespaces: ws.Namespaces(key)})
	return comp, true
}

// typeStatementFor rebuilds the declaration a TypeFact was read from, so the
// compiler records it exactly as compiling that module would have.
func typeStatementFor(fact sema.TypeFact) mast.Statement {
	idents := make([]*mast.Identifier, 0, len(fact.Members))
	for _, member := range fact.Members {
		idents = append(idents, &mast.Identifier{Value: member})
	}
	name := &mast.Identifier{Value: fact.Name}
	if fact.Kind == sema.SymEnum {
		return &mast.EnumStatement{Name: name, Variants: idents}
	}
	return &mast.StructStatement{Name: name, Fields: idents}
}

// declaresAMacro reports whether DefineMacros would collect anything from this
// file: a top-level `let name = macro(...) {...}`.
func declaresAMacro(program *mast.Program) bool {
	for _, statement := range program.Statements {
		if let, ok := statement.(*mast.LetStatement); ok {
			if _, isMacro := let.Value.(*mast.MacroLiteral); isMacro {
				return true
			}
		}
	}
	return false
}

// callsAClosureMacro reports whether a name this file calls without binding it
// is a macro some module of its closure declares -- a call the build expands.
func (s *Snapshot) callsAClosureMacro() bool {
	if s.workspace == nil || s.ModuleKey == "" || !fileImportsAnything(s) {
		return false
	}
	macros := make(map[string]struct{})
	closure, _ := s.workspace.Closure(s.ModuleKey)
	for _, k := range closure {
		if k == s.ModuleKey {
			continue
		}
		if facts, known := s.workspace.Facts(k); known {
			for name := range facts.Macros {
				macros[name] = struct{}{}
			}
		}
	}
	if len(macros) == 0 {
		return false
	}
	for _, miss := range s.Graph().UnboundUses() {
		if _, isMacro := macros[miss.Name]; isMacro && miss.InCall {
			return true
		}
	}
	return false
}

// rangeWithin reports whether inner lies inside outer.
func rangeWithin(inner, outer lsp.Range) bool {
	return !positionBefore(inner.Start, outer.Start) && !positionBefore(outer.End, inner.End)
}

func positionBefore(a, b lsp.Position) bool {
	return a.Line < b.Line || (a.Line == b.Line && a.Character < b.Character)
}
