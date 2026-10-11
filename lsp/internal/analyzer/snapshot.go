package analyzer

import (
	"sync"

	mast "mutant/ast"
	localprotocol "mutant/lsp/internal/protocol"
	"mutant/parser"
	"mutant/sema"
)

// Snapshot is a parsed view of one document.
//
// ParseErrors are hard failures: the tree cannot be trusted and the formatter
// falls back to whitespace normalisation. Recoverables are problems the
// parser repaired around — missing or redundant semicolons — which leave the
// tree fully usable. Keeping the two apart is what lets the formatter fix a
// missing `;` instead of bailing out on it.
type Snapshot struct {
	Source       string
	Program      *mast.Program
	ParseErrors  []parser.ParseError
	Recoverables []parser.RecoverableError

	// ModuleKey and workspace are what this document knows about the rest of
	// the program, and both are optional: a nil workspace means "this file and
	// nothing else", which is exactly what the REPL, the playground, api.Lint
	// and ~99 tests want, and what Analyze(src) has always produced.
	//
	// They are read-only here. The workspace is owned by the server and shared
	// by every open document -- deliberately not per-snapshot, because a
	// module's exports are read by every file that imports it and rebuilding
	// them once per importer per keystroke is the latency regression no unit
	// test would catch.
	ModuleKey string
	workspace *sema.Workspace

	// typeMap is the lazily-built, best-effort type of each confidently-typed AST
	// node (see infer.go). It powers hover/completion/inlay type info and has no
	// runtime effect. Built once via typeOnce. structFields (structName ->
	// fieldName -> Type) is built in the same pass and backs struct field-type
	// hover/completion detail.
	typeOnce     sync.Once
	typeMap      map[mast.Node]Type
	structFields map[string]map[string]Type

	// graphOnce guards the name graph: which declaration each name in this
	// document refers to, which declarations are in scope where, and where each
	// one is used. It is sema.BuildFile's single walk, and it replaced five
	// separate ones in this package. Built once, on the first question that
	// needs it, so an analysis that only lints never pays for it.
	graphOnce sync.Once
	graph     *sema.Graph

	// structIndexOnce guards the two lookups struct_type.go answers from: the
	// struct literal each let-bound name was initialised from, keyed by the
	// identifier that declares it, and the first top-level declaration of each
	// struct name. Both were a walk per question, and the graph build and
	// StructFieldDiagnostics ask once per field read (M26-SEM-004).
	structIndexOnce sync.Once
	structLiterals  map[*mast.Identifier]string
	structDecls     map[string]*mast.StructStatement

	// solvedOnce guards the constraint solver in fn_solver.go, which works out
	// how each user function's body uses its parameters. Only the function card
	// reads it, as "used as STRING". Inference does not: it used to seed the
	// result into each body, which made a usage deduction into every alias's,
	// return's and call result's type (M26-SEM-003), so the solver is built only
	// when a card is. It resolves every name through the graph, so building it
	// builds the graph too; solvedFunctions has the rule that keeps that from
	// deadlocking.
	solvedOnce sync.Once
	solvedFns  map[*mast.FunctionLiteral]*solvedFunction

	// boundFnOnce guards boundFns, which maps each name a let binds to a
	// function literal, at any depth, to that literal. Signature help, hover and
	// inlay hints ask it once per call site, so it is built once.
	boundFnOnce sync.Once
	boundFns    map[*mast.Identifier]*mast.FunctionLiteral

	// mapperOnce guards the index that converts between the byte columns the
	// lexer and the AST record and the UTF-16 code units Position.character is
	// measured in. The two agree on an ASCII line and on no other, so every
	// position crossing the protocol boundary goes through it -- see
	// protocol.Mapper for why the conversion lives at the boundary rather than
	// in the lexer. Built from Source on the first question that needs one, so
	// an analysis that answers none never pays for it.
	mapperOnce sync.Once
	mapper     *localprotocol.Mapper
}

// solvedFunctions returns how the body of every user function in the document
// uses its parameters, working it out once on first use.
//
// The solver asks the graph what each name refers to, so the first call builds
// the graph from inside solvedOnce. That is safe for one reason, and the reason
// has to stay true: nothing reachable from the graph's build calls this. The
// build asks structHeldBy which struct a declaration holds, and structHeldBy
// reaches the struct index (structIndexOnce) and the inference pass (typeOnce).
// A sync.Once that is entered again from inside its own Do waits for itself, so
// if anything on that path ever asked for the solver, the first card hovered
// would hang solvedOnce -> graphOnce -> typeOnce -> solvedOnce, and a
// deadlocked language server reports nothing at all. Inference stopped reading
// the solver for reasons of its own (M26-SEM-003) and must not start again.
// TestBuildingTheGraphNeverBuildsTheSolver pins the rule.
func (s *Snapshot) solvedFunctions() map[*mast.FunctionLiteral]*solvedFunction {
	if s == nil {
		return nil
	}
	s.solvedOnce.Do(func() {
		s.solvedFns = solveFunctionParams(s)
	})
	return s.solvedFns
}

// types returns the inferred type map, building it once on first use.
func (s *Snapshot) types() map[mast.Node]Type {
	if s == nil {
		return nil
	}
	s.typeOnce.Do(func() {
		s.typeMap, s.structFields = inferTypes(s)
	})
	return s.typeMap
}

// TypeOf returns the inferred type of an AST node and whether one was inferred.
func (s *Snapshot) TypeOf(node mast.Node) (Type, bool) {
	m := s.types()
	if m == nil {
		return AnyType, false
	}
	t, ok := m[node]
	return t, ok
}

// StructFieldType returns a struct field's inferred type, present only when every
// initializer of that struct in the document agreed on one known type for it. It
// backs field-type detail in hover and member completion; zero runtime effect.
func (s *Snapshot) StructFieldType(structName, field string) (Type, bool) {
	if s == nil {
		return AnyType, false
	}
	s.types() // ensure the inference pass has run and populated structFields
	m, ok := s.structFields[structName]
	if !ok {
		return AnyType, false
	}
	t, ok := m[field]
	if !ok || !t.IsKnown() {
		return AnyType, false
	}
	return t, true
}

// SemicolonProblems returns the recoverable semicolon issues in source order.
// It exists so diagnostics and code actions do not have to know how
// recoverables are stored.
func (s *Snapshot) SemicolonProblems() []parser.RecoverableError {
	if s == nil {
		return nil
	}
	return s.Recoverables
}
