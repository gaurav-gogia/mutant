package analyzer

// The compiler's one-declaration-per-scope refusal, judged the way the compiler
// judges it: by statement, not by name.
//
// compiler.refuseIfNothingIsNew refuses a `let` only when none of its names is
// new to the scope it is written in -- a blank counting as new -- because that
// is Go's rule for a short variable declaration and the (value, err) idiom
// depends on it: `let a, err = f(); let b, err = g();` is two declarations of
// err and is accepted, because b is new.
//
// The rule this replaces judged by name: a second declaration of err in one
// scope was a duplicate whatever else its statement declared, so the idiom drew
// "duplicate top-level declaration `err`" 116 times across 17 shipped examples,
// every one of which compiles. And it excused the opposite case by a heuristic
// -- an earlier binding that was grouped and read could be rebound alone -- so
// `let abc, err = gets(); abc; let abc = 1;`, which the compiler refuses, drew
// nothing (M26-LSP-029).
//
// Not a lint, and it takes no LintConfig: the build refuses every one of these,
// in this sentence, which is sema's and the compiler's both.

import (
	mast "mutant/ast"
	"mutant/sema"

	lsp "github.com/tliron/glsp/protocol_3_16"
)

// DiagnosticSourceDeclarations marks a refused redeclaration.
const DiagnosticSourceDeclarations = "mutant-declarations"

// duplicateDeclarationRefusals returns one Error per `let` the compiler refuses
// as declaring nothing new, and the names it found redeclared.
func (s *Snapshot) duplicateDeclarationRefusals() ([]lsp.Diagnostic, map[string]struct{}) {
	if s == nil || s.Program == nil {
		return nil, nil
	}
	graph := s.Graph()
	if graph == nil {
		return nil, nil
	}

	severity := lsp.DiagnosticSeverityError
	source := DiagnosticSourceDeclarations
	var result []lsp.Diagnostic
	names := make(map[string]struct{})

	declared := make(map[*sema.Scope]map[string]bool)
	note := func(scope *sema.Scope, name string) {
		if name == "" || name == "_" {
			return
		}
		if declared[scope] == nil {
			declared[scope] = make(map[string]bool, 8)
		}
		declared[scope][name] = true
	}

	decls := graph.Declarations()
	for i := 0; i < len(decls); {
		node := decls[i]
		switch node.Kind {
		case sema.KindParam, sema.KindLoopBind:
			// The compiler declares these through the same declare() a let
			// uses, so a let in the same scope that repeats one is refused.
			note(node.Scope, node.Name)
			i++
			continue
		case sema.KindValue, sema.KindFunction:
		default:
			i++
			continue
		}

		// One let statement: the run of value declarations sharing its scope
		// and its statement range. A grouped let whose statement range the
		// parser did not record cannot be told from its neighbours, so it is
		// not judged -- saying nothing is right where the grouping is unknown.
		run := []*sema.Node{node}
		j := i + 1
		for j < len(decls) && sameLet(decls[j], node) {
			run = append(run, decls[j])
			j++
		}
		i = j
		if node.Grouped && node.FullRange == node.DeclRange {
			for _, n := range run {
				note(n.Scope, n.Name)
			}
			continue
		}

		fresh, already := 0, []string{}
		var anchor mast.Range
		for _, n := range run {
			if n.Name == "_" {
				fresh++
				continue
			}
			if declared[n.Scope][n.Name] {
				if len(already) == 0 {
					anchor = n.DeclRange
				}
				already = append(already, n.Name)
				continue
			}
			fresh++
		}
		if fresh == 0 && len(already) > 0 && anchor.IsValid() {
			result = append(result, lsp.Diagnostic{
				Range:    s.Range(anchor),
				Severity: &severity,
				Source:   &source,
				Message:  sema.DuplicateDeclarationRefusal(already).Message,
			})
			for _, name := range already {
				names[name] = struct{}{}
			}
		}
		for _, n := range run {
			note(n.Scope, n.Name)
		}
	}
	return result, names
}

// sameLet reports whether next was declared by the same let statement as first.
func sameLet(next, first *sema.Node) bool {
	if next.Kind != sema.KindValue && next.Kind != sema.KindFunction {
		return false
	}
	if next.Scope != first.Scope || !first.Grouped || !next.Grouped {
		return false
	}
	return next.FullRange == first.FullRange && first.FullRange != first.DeclRange
}
