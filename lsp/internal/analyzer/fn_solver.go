package analyzer

import (
	"math/bits"
	"strings"

	mast "mutant/ast"
	"mutant/builtin"
	"mutant/sema"
	"mutant/token"
)

// How a user-defined function's body uses its parameters, worked out from the
// uses.
//
// Mutant has no type annotation syntax, so a user function's hover card can say
// nothing about a parameter beyond its name unless something works it out from
// the body. Forward inference cannot: it walks a body once with every parameter
// as Any, which is why hover showed nothing but a list of names.
//
// This runs the other direction. Each parameter starts as "any kind", and every
// use of it in the body removes kinds it cannot be. `str_upper(host)` narrows
// host to STRING because that is what str_upper's verified contract demands;
// `a - b` narrows both to the numeric kinds because those are the only ones the
// VM's binary operation accepts. What survives describes how the body USES the
// parameter -- the function's own body, not the body of a function nested in
// it; see walkBodyExpressions -- which is not a type a caller is held to: a
// builtin given the wrong kind returns an error value and the function carries
// on, and a use behind a type guard says nothing about the other path.
//
// So it is shown only as what it is, on the function's card, as "used as
// STRING", and nothing else reads it. Inference used to seed it into the body,
// which carried it into hover, inlay hints and the argument-type diagnostic
// (M26-SEM-003), and signature help and the card's signature line used to show
// it as the parameter's type (M26-SEM-019). It must never feed a diagnostic: a
// constraint set is a deduction about a dynamically-typed program, and the bar
// for underlining someone's code is higher than the bar for describing it.
//
// Every name is resolved the way the compiler resolves it, by asking the file's
// name graph which declaration it refers to. A use counts against a parameter
// only when it resolves to that parameter's own declaration, and a call reaches
// a user function only when its callee resolves to the let that bound the
// function's literal. The solver used to match bare names, file-wide, and any
// sentence the card built on a match could be false: a block-local `let x` or a
// loop's `for (x in xs)` narrowed the parameter x it shadowed, a call through a
// parameter called `shout` was read as a call of the file's function shout and
// its argument held to what that function takes, and of two helpers both called
// `helper`, nested in two different functions, the one written last answered
// for calls to either. M26-SEM-003's evidence named that keying as a second
// source of wrong narrowing.
//
// The graph and the compiler part ways over one name, and the solver corrects
// for it rather than trusting the graph there (solverNames.resolve): a function
// literal's own name, inside its own body. The compiler binds it in every
// literal a let binds; the graph binds it before walking the literal only for a
// let of one name. So in
//
//	let up, err = fn(x, n) { if (n > 0) { return up(x, n - 1); } return x; };
//
// the graph reads the inner up as an earlier `let up`, or as bound to nothing --
// the builtin, where one has the name -- while the program calls the literal
// itself.
//
// Two writes take a name out of the solver's hands. A parameter the program
// assigns is not narrowed at all: after `x = 5` the uses describe the new value,
// not anything a caller passed. And a function whose name is assigned is no
// call target: after `f = other;` its literal no longer says what a call to f
// runs.

// kindSet is a set of ParamKinds, as a bitset so intersection is an AND and the
// fixpoint loop can compare rounds by value.
type kindSet uint16

const (
	ksString kindSet = 1 << iota
	ksBytes
	ksInt
	ksFloat
	ksBool
	ksArray
	ksHash
	ksFn
	ksNull
	ksStruct
	ksEnum
	ksError
)

// ksAny is the top of the lattice: a parameter nothing has constrained.
const ksAny = ksString | ksBytes | ksInt | ksFloat | ksBool | ksArray | ksHash | ksFn | ksNull | ksStruct | ksEnum | ksError

// kindSetWidth is how many kinds one set can hold. It is counted off ksAny
// rather than written down, so adding a kind to the lattice cannot leave the
// solver's round bound behind. See maxSolverRounds.
var kindSetWidth = bits.OnesCount16(uint16(ksAny))

// The operand domains below are read off the VM, not assumed.
//
//   - execBinaryOperation (vm.go) accepts INTEGER×INTEGER, STRING×STRING, or
//     numeric×numeric, and execBinaryStringOperation rejects every operator but
//     OpAdd — so `+` admits strings and the rest do not.
//   - execBinaryBytesOperation likewise rejects every operator but OpAdd, so
//     buffers concatenate and do nothing else.
//   - execComparison accepts INTEGER×INTEGER or numeric×numeric.
//     execBytesComparison handles a bytes on either side, but only for equality,
//     so bytes are deliberately absent from ksComparable. execErrorComparison
//     does the same for errors, and for the same reason: `<` on two errors has
//     no meaning, so errors are absent from ksComparable too.
//   - execMinusOperation asserts INTEGER or FLOAT.
//   - execBinaryOperation rejects a bitwise operator whose operands are not both
//     INTEGER before it dispatches on type at all, and execBitNotOperation does
//     the same for `~` -- so `& | ^ << >>` and the complement admit ksInt and
//     nothing else. There is no float promotion to widen that with.
//
// `!`, `==` and `!=` accept anything, and so appear nowhere here: truthiness and
// equality are defined for every value, and a constraint that excludes nothing
// is worth no code.
const (
	ksNumeric    = ksInt | ksFloat
	ksAddable    = ksInt | ksFloat | ksString | ksBytes
	ksComparable = ksInt | ksFloat
)

var kindBits = map[builtin.ParamKind]kindSet{
	builtin.ParamString: ksString,
	builtin.ParamBytes:  ksBytes,
	builtin.ParamInt:    ksInt,
	builtin.ParamFloat:  ksFloat,
	builtin.ParamBool:   ksBool,
	builtin.ParamArray:  ksArray,
	builtin.ParamHash:   ksHash,
	builtin.ParamFn:     ksFn,
	builtin.ParamNull:   ksNull,
	builtin.ParamStruct: ksStruct,
	builtin.ParamEnum:   ksEnum,
	builtin.ParamError:  ksError,
}

// kindSetFor converts a declared kind list into a set. An empty list, or one
// containing ANY, is the unconstrained top — the same rule the argument-type
// diagnostic follows, so the solver never narrows on a contract that says
// nothing.
func kindSetFor(kinds []builtin.ParamKind) kindSet {
	if len(kinds) == 0 {
		return ksAny
	}
	var set kindSet
	for _, kind := range kinds {
		if kind == builtin.ParamAny {
			return ksAny
		}
		set |= kindBits[kind]
	}
	if set == 0 {
		return ksAny
	}
	return set
}

// names renders a set in the runtime vocabulary the builtin cards use, so a user
// function's card reads in the same words as a builtin's. The order is fixed
// rather than bit order, so `INTEGER|FLOAT` never comes out reversed between
// runs.
//
// Every kind in the lattice has an entry, and has to: a kind with none is not
// rendered as unknown, it is left out of the sentence. BYTES and ERROR had none,
// so hash_sha256(payload), whose contract takes STRING|BYTES, put "used as
// STRING" on a function that hashes a buffer as readily as a string, and a
// parameter used only as a buffer rendered as nothing at all.
func (s kindSet) names() []string {
	ordered := []struct {
		bit  kindSet
		name string
	}{
		{ksString, "STRING"}, {ksBytes, "BYTES"}, {ksInt, "INTEGER"}, {ksFloat, "FLOAT"},
		{ksBool, "BOOLEAN"}, {ksArray, "ARRAY"}, {ksHash, "HASH"},
		{ksFn, "FUNCTION"}, {ksNull, "NULL"},
		{ksStruct, "STRUCT"}, {ksEnum, "ENUM_VALUE"}, {ksError, "ERROR"},
	}
	out := make([]string, 0, len(ordered))
	for _, entry := range ordered {
		if s&entry.bit != 0 {
			out = append(out, entry.name)
		}
	}
	return out
}

// text renders the set for display, or "" when it says nothing worth showing.
//
// The empty set means the constraints contradict each other — a parameter passed
// to str_upper and also negated. That is a bug in the program, not a type, and
// it is not this code's job to report it, so it shows nothing rather than an
// impossible claim.
func (s kindSet) text() string {
	if s == ksAny || s == 0 {
		return ""
	}
	return strings.Join(s.names(), "|")
}

// solvedFunction is how one user function's body uses its parameters.
type solvedFunction struct {
	name    string
	literal *mast.FunctionLiteral
	// params, kinds and observed hold one entry for each entry of the literal's
	// Parameters, in order, so an index names the same parameter here as it
	// does on the card that reads them.
	params []string
	// kinds is one set per parameter, positionally.
	kinds []kindSet
	// observed holds argument kinds seen at call sites in this document, used
	// only where the body constrained a parameter to nothing. It is evidence,
	// not a contract: one caller passing a string does not make the parameter a
	// string, so it is rendered as an observation and never as the type.
	observed []kindSet
	// uses is every use the function's own body makes of its own parameters.
	uses []paramUse
}

// paramUse is one use of a parameter in its function's own body: the kinds the
// use admits, or, where the use passes the parameter to another user function,
// that function's parameter, whose kinds are read afresh each round.
//
// Uses are collected once and applied every round. What a name refers to does
// not change between rounds, so resolving every name again each round was work
// that could only give the same answer; what does change is how far the
// functions a parameter flows into have been narrowed, and a flow carries that.
type paramUse struct {
	param   int
	allowed kindSet
	// target, when set, replaces allowed with target.kinds[targetParam].
	target      *solvedFunction
	targetParam int
}

// solveFunctionParams works out how every user function's body uses its
// parameters.
//
// Functions constrain each other — a parameter passed straight through to
// another function inherits that function's constraints — so this iterates to a
// fixpoint rather than making one pass.
//
// Two things about that loop are load-bearing, and it had neither.
//
// It visits functions in source order, from a slice, rather than ranging the
// map. Go randomises map order, so the direction information travelled changed
// between runs: a chain `f0(x) -> f1(x) -> ... -> str_upper(x)` reached its
// fixpoint on the run that happened to visit the chain backwards and stalled
// part way on the others. Measured over 40 fresh analyses of a 14-function
// chain, signature help on `f0("a")` came out `f0(x)` 39 times and
// `f0(x: STRING)` once — the same document, the same keystroke, a different
// answer. An editor that answers differently each time it is asked is worse
// than one that answers less.
//
// That chain was written caller-first at the top level, which the compiler
// refuses -- f1 is not declared yet where f0 calls it, `undefined variable: f1`
// -- and now that a callee is resolved the way the compiler resolves it, such a
// chain carries nothing. A chain that compiles still needs the rounds: functions
// nested one inside the next, each calling the one it declares, are written and
// visited outer-first, so each round carries a constraint one link outwards.
//
// And it runs until nothing changes, within a bound computed from the work
// rather than a constant. The cap was 8, which a chain of more than eight
// functions cannot reach the fixpoint inside however it is ordered, and the
// comment here called the cap a guard against a bug rather than an expected
// exit while it was in fact the normal exit. Applying a use only ever clears
// bits, so a round that changes anything has removed at least one kind from
// one parameter; the total number of kind bits across every parameter is
// therefore the number of rounds that can do any work, and one more round
// proves stability. Stopping early is safe but imprecise — a set that has not
// finished shrinking is a superset, which renders as no type rather than as a
// wrong one — so the cost of the old cap was a feature that quietly did not
// work on anything but small files.
//
// It asks the snapshot's name graph what every name refers to, from inside
// solvedOnce. solvedFunctions says why that is safe and what would make it a
// deadlock.
func solveFunctionParams(s *Snapshot) map[*mast.FunctionLiteral]*solvedFunction {
	solved := map[*mast.FunctionLiteral]*solvedFunction{}
	if s == nil || s.Program == nil {
		return solved
	}
	names := newSolverNames(s)

	// order is what the rounds below iterate: the same functions as solved, in
	// the order they are written. It also holds a function whose literal a
	// later binding shares, which ranging solved would have dropped.
	var order []*solvedFunction
	for _, binding := range functionBindings(s.Program) {
		fn := &solvedFunction{name: binding.name, literal: binding.literal}
		for i, p := range binding.literal.Parameters {
			name := ""
			if p != nil {
				name = p.Value
			}
			fn.params = append(fn.params, name)
			fn.kinds = append(fn.kinds, ksAny)
			fn.observed = append(fn.observed, 0)
			names.declareParam(fn, i, p)
		}
		order = append(order, fn)
		solved[binding.literal] = fn
		names.declareFunction(fn, binding.ident)
	}
	names.noteAssignments()
	for _, fn := range order {
		fn.uses = collectUses(fn, names)
	}

	for round := 0; round < maxSolverRounds(order); round++ {
		changed := false
		for _, fn := range order {
			if applyUses(fn) {
				changed = true
			}
		}
		if !changed {
			break
		}
	}

	observeCallSites(s.Program, names)
	return solved
}

// maxSolverRounds is how many rounds the loop above can possibly need.
//
// Every narrowing clears at least one bit of one parameter's set and nothing
// ever sets one, so there can be at most one productive round per bit in the
// whole document, and the round after the last productive one is the one that
// observes nothing changed. It is a termination guarantee and not a budget: a
// real file reaches the fixpoint in a handful of rounds, and a chain of n
// functions nested one inside the next, each calling the one it declares, needs
// about n.
func maxSolverRounds(order []*solvedFunction) int {
	rounds := 1
	for _, fn := range order {
		rounds += len(fn.kinds) * kindSetWidth
	}
	return rounds
}

type functionBinding struct {
	name string
	// ident is the name the let declares the function by -- the first, when it
	// declares several. It is how the solver finds the declaration a callee has
	// to resolve to before a call counts as a call to this function.
	ident   *mast.Identifier
	literal *mast.FunctionLiteral
}

// functionBindings collects every `let name = fn(...) {...}` in the document,
// including ones nested inside other functions.
func functionBindings(program *mast.Program) []functionBinding {
	var bindings []functionBinding
	var walkStatement func(mast.Statement)
	var walkExpression func(mast.Expression)

	walkExpression = func(e mast.Expression) {
		switch n := e.(type) {
		case *mast.FunctionLiteral:
			if n.Body != nil {
				for _, s := range n.Body.Statements {
					walkStatement(s)
				}
			}
		case *mast.CallExpression:
			walkExpression(n.Function)
			for _, arg := range n.Arguments {
				walkExpression(arg)
			}
		case *mast.InfixExpression:
			walkExpression(n.Left)
			walkExpression(n.Right)
		}
	}

	walkStatement = func(s mast.Statement) {
		switch n := s.(type) {
		case *mast.LetStatement:
			if literal, ok := n.Value.(*mast.FunctionLiteral); ok {
				name := ""
				if n.Name != nil {
					name = n.Name.Value
				}
				if literal.Name != "" {
					name = literal.Name
				}
				bindings = append(bindings, functionBinding{name: name, ident: n.Name, literal: literal})
			}
			walkExpression(n.Value)
		case *mast.ExpressionStatement:
			walkExpression(n.Expression)
		case *mast.BlockStatement:
			for _, inner := range n.Statements {
				walkStatement(inner)
			}
		case *mast.ForStatement:
			if n.Body != nil {
				walkStatement(n.Body)
			}
		case *mast.WhileStatement:
			if n.Body != nil {
				walkStatement(n.Body)
			}
		case *mast.ForInStatement:
			if n.Body != nil {
				walkStatement(n.Body)
			}
		case *mast.ReturnStatement:
			for _, value := range n.ReturnValues {
				walkExpression(value)
			}
			if n.ReturnValue != nil {
				walkExpression(n.ReturnValue)
			}
		}
	}

	for _, stmt := range program.Statements {
		walkStatement(stmt)
	}
	return bindings
}

// solverNames is the solver's one question about a name -- which declaration is
// it? -- answered by the file's name graph, which answers it the way the
// compiler does: a name refers to the nearest declaration of it in scope where
// it is written, and to nothing while no declaration of it has been reached.
// The one exception, a function literal's own name inside the literal, is
// corrected in resolve.
//
// What the solver learns is kept by declaration rather than by name. Two
// parameters called x in two functions, a parameter and the block-local let
// that shadows it, two helpers nested in different functions under one name:
// each pair is two declarations, and a use matched to the wrong one of the two
// is the card saying something false about a program that works.
type solverNames struct {
	program *mast.Program
	graph   *sema.Graph

	// scopeAt is what the file has bound where a range begins. Whether
	// `ns.member(...)` is a builtin call at all is builtinCalleeIn's decision,
	// and it needs this: a let, a parameter, an import namespace or an enum
	// called `str` makes `str.upper` something other than str_upper.
	scopeAt func(mast.Range) sema.LocalScope

	// functions is each let that binds a function literal, by the declaration
	// its name makes.
	functions map[*sema.Node]*solvedFunction
	// params is each parameter of those functions, by its declaration.
	params map[*sema.Node]paramSlot
	// assigned is each declaration an assignment anywhere in the file writes,
	// and assignedNames each name an assignment writes where the graph could
	// not say which declaration it meant.
	assigned      map[*sema.Node]bool
	assignedNames map[string]bool
	// unbound is where the graph recorded a name standing on its own that the
	// file binds nowhere it can see from there. It is what lets a name be read
	// as a builtin only on the graph's positive word, rather than because no
	// answer came back.
	unbound map[token.Position]bool
	// ownNames is every function literal the compiler gives a name inside its
	// own body, by that name. It is what resolve corrects the graph with.
	ownNames map[string][]ownNamed
}

// paramSlot is one parameter: the function that declares it, and where.
type paramSlot struct {
	fn    *solvedFunction
	index int
}

// ownNamed is one function literal the compiler names inside its own body --
// DefineFunctionName binds the name, and a use of it loads the literal with
// OpCurrentClosure -- and the declaration that name stands for: the one the
// let's first name makes, which is the declaration a callee has to resolve to
// before a call counts as a call of the literal (declareFunction). binding is
// nil where no let the solver can place binds the literal, and then a use of
// the name inside it has no answer the solver can give.
type ownNamed struct {
	rng     mast.Range
	binding *sema.Node
}

// holds reports whether rng lies inside the literal, its ends included.
func (own ownNamed) holds(rng mast.Range) bool {
	if !rng.IsValid() || !own.rng.IsValid() {
		return false
	}
	before := func(a, b token.Position) bool {
		return a.Line < b.Line || (a.Line == b.Line && a.Column < b.Column)
	}
	return !before(rng.Start, own.rng.Start) && !before(own.rng.End, rng.End)
}

func newSolverNames(s *Snapshot) *solverNames {
	names := &solverNames{
		program:       s.Program,
		graph:         s.Graph(),
		scopeAt:       s.localScopeAtRange,
		functions:     map[*sema.Node]*solvedFunction{},
		params:        map[*sema.Node]paramSlot{},
		assigned:      map[*sema.Node]bool{},
		assignedNames: map[string]bool{},
		unbound:       map[token.Position]bool{},
		ownNames:      map[string][]ownNamed{},
	}
	for _, miss := range names.graph.UnboundUses() {
		if miss.Kind == sema.UnboundValue {
			names.unbound[miss.UseRange.Start] = true
		}
	}
	names.noteOwnNames()
	return names
}

// noteOwnNames records every function literal the compiler names inside its own
// body, which is every literal the parser gave a name: it names the literal a
// let binds after the let's first name, whether the let binds one name or
// several. It ranges NodePositions, as noteAssignments does, so a literal
// functionBindings does not collect -- one bound inside an `if` -- is recorded
// too; a call inside it to its own name is still not a call of anything else.
func (n *solverNames) noteOwnNames() {
	letOf := map[*mast.FunctionLiteral]*mast.LetStatement{}
	for node := range n.program.NodePositions {
		if let, ok := node.(*mast.LetStatement); ok && let != nil {
			if literal, ok := let.Value.(*mast.FunctionLiteral); ok && literal != nil {
				letOf[literal] = let
			}
		}
	}
	for node, rng := range n.program.NodePositions {
		literal, ok := node.(*mast.FunctionLiteral)
		if !ok || literal == nil || literal.Name == "" || !rng.IsValid() {
			continue
		}
		own := ownNamed{rng: rng}
		if let := letOf[literal]; let != nil && let.Name != nil && let.Name.Value == literal.Name {
			own.binding, _ = n.declaredBy(let.Name)
		}
		n.ownNames[literal.Name] = append(n.ownNames[literal.Name], own)
	}
}

// ownNameAround is the innermost literal named name that holds rng: the literal
// the compiler reads a use of name at rng as, unless something declared inside
// that literal is nearer.
func (n *solverNames) ownNameAround(name string, rng mast.Range) (ownNamed, bool) {
	var innermost ownNamed
	found := false
	for _, own := range n.ownNames[name] {
		if !own.holds(rng) {
			continue
		}
		// Of two literals that both hold the use, one holds the other.
		if !found || innermost.holds(own.rng) {
			innermost, found = own, true
		}
	}
	return innermost, found
}

// rangeOf is where ident is written, which is the coordinate the graph is asked
// in.
func (n *solverNames) rangeOf(ident *mast.Identifier) (mast.Range, bool) {
	if ident == nil || n.program == nil {
		return mast.Range{}, false
	}
	return n.program.RangeOf(ident)
}

// declaredBy is the declaration ident itself makes. It is found where ident is
// written and then checked to be ident's own, so a position that lands anywhere
// else is no answer rather than a different one.
func (n *solverNames) declaredBy(ident *mast.Identifier) (*sema.Node, bool) {
	rng, ok := n.rangeOf(ident)
	if !ok {
		return nil, false
	}
	declared, found := n.graph.DeclarationAt(rng.Start.Line, rng.Start.Column)
	if !found || declared.Ident != ident {
		return nil, false
	}
	return declared, true
}

// resolve is what a use of ident refers to: its declaration, or nil when
// nothing in this file binds the name where it is written -- a builtin, a typo,
// a function declared further down. known is false when there is no answer at
// all, and then nothing may be concluded from the name, the builtin reading
// included.
//
// The graph answers, and is corrected in one place. The compiler gives a
// function literal the name of the let that binds it, inside the literal's own
// body, and the graph makes that binding before it walks the literal only for a
// let of one name. In
//
//	let up, err = fn(x, n) { if (n > 0) { return up(x, n - 1); } return x; };
//
// the graph walks the literal before it declares up, so it answers the inner up
// with an earlier `let up`, whose parameters the call would be held to, or with
// nothing, which reads as the builtin where one has the name, as str_upper does
// in `let str_upper, err = fn(x) { ... }` -- while the program calls the
// literal itself. So a name written inside a literal of that name is that
// literal's let, unless the graph found it declared inside the literal: a
// parameter of that name, or a let in the body ahead of the use, is nearer than
// the literal's own name, and in the compiler it wins too.
func (n *solverNames) resolve(ident *mast.Identifier) (declared *sema.Node, known bool) {
	rng, ok := n.rangeOf(ident)
	if !ok {
		return nil, false
	}
	declared, known = n.graphAnswer(ident, rng)
	own, inside := n.ownNameAround(ident.Value, rng)
	if !inside || !known {
		return declared, known
	}
	if declared != nil && own.holds(declared.DeclRange) {
		return declared, true
	}
	if own.binding == nil {
		// The compiler binds the name to a literal no let the solver can place
		// declares, which is no answer rather than the graph's wrong one.
		return nil, false
	}
	return own.binding, true
}

// graphAnswer is what the graph says a use of ident, written at rng, refers to,
// with resolve's meaning for the two results.
func (n *solverNames) graphAnswer(ident *mast.Identifier, rng mast.Range) (*sema.Node, bool) {
	if found, ok := n.graph.Resolve(rng.Start.Line, rng.Start.Column); ok {
		// A declaration of another name means the position landed on a
		// different identifier, which is no answer about this one.
		if found.Name != ident.Value {
			return nil, false
		}
		return found, true
	}
	if n.unbound[rng.Start] {
		return nil, true
	}
	return nil, false
}

// insideOwnName reports whether e is a bare name written inside a function
// literal of that name. The compiler binds the name there -- to the literal, or
// to something declared inside it that is nearer -- so `str.upper(p)` written
// inside a literal the compiler calls str reads a field of whatever str holds
// there, or names an enum's variant or an imported module's member, which the
// compiler looks for first, and never folds to str_upper, whatever the graph
// has bound at that point.
func (n *solverNames) insideOwnName(e mast.Expression) bool {
	ident, ok := e.(*mast.Identifier)
	if !ok || ident == nil {
		return false
	}
	rng, ok := n.rangeOf(ident)
	if !ok {
		return false
	}
	_, inside := n.ownNameAround(ident.Value, rng)
	return inside
}

// declareParam records the declaration a function's parameter makes.
func (n *solverNames) declareParam(fn *solvedFunction, index int, param *mast.Identifier) {
	if declared, ok := n.declaredBy(param); ok {
		n.params[declared] = paramSlot{fn: fn, index: index}
	}
}

// declareFunction records the declaration a callee has to resolve to before a
// call counts as a call to fn: the one the let's first name makes. A let of
// several names binds the literal to the first and NULL to the rest -- both
// engines' destructureValues put a value that is neither a pair nor an array in
// the first slot -- so a call through any other name runs nothing.
func (n *solverNames) declareFunction(fn *solvedFunction, ident *mast.Identifier) {
	if declared, ok := n.declaredBy(ident); ok {
		n.functions[declared] = fn
	}
}

// noteAssignments records every declaration an assignment writes, anywhere in
// the file, by asking the graph what each assignment's target refers to. It
// ranges NodePositions because the parser records the range of every
// assignment it builds, the compound and postfix forms included.
//
// Only a bare name counts. `x[0] = v` and `x.f = v` change what x holds, not
// which value x is, so the uses after them are still uses of the value the
// caller passed.
//
// A target the graph cannot place is counted against its name instead, which
// keeps every parameter and function of that name out of the solver. That
// costs some cards a line; trusting a parameter whose assignment went unseen
// could cost one a true sentence.
func (n *solverNames) noteAssignments() {
	for node := range n.program.NodePositions {
		assign, ok := node.(*mast.AssignExpression)
		if !ok || assign == nil {
			continue
		}
		target, ok := assign.Left.(*mast.Identifier)
		if !ok || target == nil {
			continue
		}
		declared, known := n.resolve(target)
		switch {
		case !known:
			n.assignedNames[target.Value] = true
		case declared != nil:
			n.assigned[declared] = true
		}
	}
}

// written reports whether an assignment may write declared.
func (n *solverNames) written(declared *sema.Node) bool {
	return n.assigned[declared] || n.assignedNames[declared.Name]
}

// paramOf is the parameter e is a use of: e is a bare name, it resolves to a
// parameter's own declaration, and nothing assigns that parameter.
func (n *solverNames) paramOf(e mast.Expression) (paramSlot, bool) {
	ident, ok := e.(*mast.Identifier)
	if !ok || ident == nil {
		return paramSlot{}, false
	}
	declared, known := n.resolve(ident)
	if !known || declared == nil || n.written(declared) {
		return paramSlot{}, false
	}
	slot, ok := n.params[declared]
	return slot, ok
}

// callTarget is the function a call through declared runs, where the solver can
// say: the literal a let bound to declared, provided nothing assigns it. After
// `f = other;` the literal no longer says what a call to f runs, so it is no
// call target at all -- neither its constraints nor the call's arguments are
// read through it.
func (n *solverNames) callTarget(declared *sema.Node) *solvedFunction {
	if declared == nil || n.written(declared) {
		return nil
	}
	return n.functions[declared]
}

// scopeOf is what the file has bound where node begins. A node with no position
// has no answer, which is not the same as an answer that binds nothing: the
// second would fold every `ns.member` it was asked about into a builtin.
func (n *solverNames) scopeOf(node mast.Node) (sema.LocalScope, bool) {
	rng, ok := n.program.RangeOf(node)
	if !ok {
		return sema.LocalScope{}, false
	}
	return n.scopeAt(rng), true
}

// collectUses records every use fn's own body makes of its own parameters.
func collectUses(fn *solvedFunction, names *solverNames) []paramUse {
	var uses []paramUse
	// ownParam is the parameter e is a use of, when it is one of fn's own. A
	// use that resolves to the parameter of a function fn is nested in is
	// made from inside a nested literal, which that function's own walk does
	// not count either.
	ownParam := func(e mast.Expression) (int, bool) {
		slot, ok := names.paramOf(e)
		if !ok || slot.fn != fn {
			return 0, false
		}
		return slot.index, true
	}
	narrow := func(e mast.Expression, allowed kindSet) {
		if param, ok := ownParam(e); ok {
			uses = append(uses, paramUse{param: param, allowed: allowed})
		}
	}
	flow := func(e mast.Expression, target *solvedFunction, targetParam int) {
		if param, ok := ownParam(e); ok {
			uses = append(uses, paramUse{param: param, target: target, targetParam: targetParam})
		}
	}

	visit := func(e mast.Expression) {
		switch n := e.(type) {
		case *mast.CallExpression:
			constrainCall(n, narrow, flow, names)
		case *mast.InfixExpression:
			switch n.Operator {
			case "+":
				narrow(n.Left, ksAddable)
				narrow(n.Right, ksAddable)
			case "&", "|", "^", "<<", ">>":
				narrow(n.Left, ksInt)
				narrow(n.Right, ksInt)
			case "-", "*", "/", "%":
				narrow(n.Left, ksNumeric)
				narrow(n.Right, ksNumeric)
			case "<", ">", "<=", ">=":
				narrow(n.Left, ksComparable)
				narrow(n.Right, ksComparable)
			}
		case *mast.PrefixExpression:
			switch n.Operator {
			case "-":
				narrow(n.Right, ksNumeric)
			case "~":
				narrow(n.Right, ksInt)
			}
		}
	}

	walkBodyExpressions(fn.literal.Body, visit)
	return uses
}

// applyUses narrows fn's parameters by its uses and reports whether any of them
// shrank. A flow narrows by the target's parameter as it stands now, so a round
// that narrows a target is followed by one that carries it to its callers.
func applyUses(fn *solvedFunction) bool {
	changed := false
	for _, use := range fn.uses {
		allowed := use.allowed
		if use.target != nil {
			allowed = use.target.kinds[use.targetParam]
		}
		if narrowed := fn.kinds[use.param] & allowed; narrowed != fn.kinds[use.param] {
			fn.kinds[use.param] = narrowed
			changed = true
		}
	}
	return changed
}

// walkBodyExpressions visits every expression in a function body, without
// descending into a nested function literal.
//
// It used to give a different reason: a closure's own parameters could shadow
// an outer one by name, and the solver worked by name. Names are resolved now,
// so a captured parameter could no longer be mistaken for a closure's own, and
// the walk still stops at a nested literal -- because the card says how this
// function's body uses a parameter, and a nested function's body is not that.
// It runs when something calls it, if anything does, and possibly after this
// function has returned. This f never uses x itself; it returns something that
// may:
//
//	let f = fn(x) { return fn() { return str_upper(x); }; };
//
// A missed use costs a less specific card; counting the closure's would put a
// sentence about one function on another's card. A nested function bound by a
// let is solved in its own right, and its card says how it uses its own
// parameters.
func walkBodyExpressions(body *mast.BlockStatement, visit func(mast.Expression)) {
	if body == nil {
		return
	}

	var walkStatement func(mast.Statement)
	var walkExpression func(mast.Expression)

	walkExpression = func(e mast.Expression) {
		if e == nil {
			return
		}
		visit(e)
		switch n := e.(type) {
		case *mast.CallExpression:
			walkExpression(n.Function)
			for _, arg := range n.Arguments {
				walkExpression(arg)
			}
		case *mast.InfixExpression:
			walkExpression(n.Left)
			walkExpression(n.Right)
		case *mast.PrefixExpression:
			walkExpression(n.Right)
		case *mast.IndexExpression:
			walkExpression(n.Left)
			walkExpression(n.Index)
		case *mast.AssignExpression:
			walkExpression(n.Left)
			walkExpression(n.Value)
		case *mast.FieldExpression:
			walkExpression(n.Left)
		case *mast.ArrayLiteral:
			for _, el := range n.Elements {
				walkExpression(el)
			}
		case *mast.TemplateLiteral:
			for _, el := range n.Parts {
				walkExpression(el)
			}
		case *mast.HashLiteral:
			for _, pair := range n.Pairs {
				walkExpression(pair.Key)
				walkExpression(pair.Value)
			}
		case *mast.IfExpression:
			walkExpression(n.Condition)
			if n.Consequence != nil {
				walkStatement(n.Consequence)
			}
			if n.Alternative != nil {
				walkStatement(n.Alternative)
			}
		case *mast.MatchExpression:
			walkExpression(n.Subject)
			for _, arm := range n.Arms {
				if arm != nil && arm.Body != nil {
					walkStatement(arm.Body)
				}
			}
		}
	}

	walkStatement = func(s mast.Statement) {
		switch n := s.(type) {
		case *mast.LetStatement:
			walkExpression(n.Value)
		case *mast.ExpressionStatement:
			walkExpression(n.Expression)
		case *mast.ReturnStatement:
			for _, value := range n.ReturnValues {
				walkExpression(value)
			}
			walkExpression(n.ReturnValue)
		case *mast.BlockStatement:
			for _, inner := range n.Statements {
				walkStatement(inner)
			}
		case *mast.ForStatement:
			if n.Init != nil {
				walkStatement(n.Init)
			}
			walkExpression(n.Condition)
			walkExpression(n.Post)
			if n.Body != nil {
				walkStatement(n.Body)
			}
		case *mast.WhileStatement:
			walkExpression(n.Condition)
			if n.Body != nil {
				walkStatement(n.Body)
			}
		case *mast.ForInStatement:
			walkExpression(n.Iterable)
			if n.Body != nil {
				walkStatement(n.Body)
			}
		}
	}

	for _, stmt := range body.Statements {
		walkStatement(stmt)
	}
}

// constrainCall narrows the arguments of one call against whatever the callee
// demands — a builtin's declared contract, or another user function's solved
// parameters — once the callee has been resolved the way the compiler resolves
// it. narrow records a fixed set of kinds an argument must be in; flow records
// that an argument lands on a user function's parameter, whose kinds are still
// being worked out.
//
// A bare name is decided by the declaration it resolves to. A let bound to a
// function literal is that function, while nothing assigns it (callTarget). A
// parameter, or any other binding, is nothing the solver can see into, though
// calling a parameter does say that parameter is a function. A name that
// resolve finds bound to nothing in this file -- the graph recorded it so, and
// it is not written inside a function literal of that name -- is the builtin of
// that name when there is one: that is what the compiler emits for it, and what
// the VM calls.
//
// The builtin half accepts either spelling: `fs.read(p)` and `fs_read(p)` are
// the same call, and asking only for an *mast.Identifier meant a dotted call
// contributed no evidence at all -- so a parameter used only through dotted
// calls stayed unconstrained and hovered as any. A dotted callee is a builtin
// call only when builtinCalleeIn says so, asked with the scope where the callee
// is written, which is the decision signature help makes: a let, a parameter,
// an import namespace or an enum called `str` makes `str.upper(p)` a call of
// something else, and holding p to str_upper's contract would describe a call
// the program never makes. So does a function literal the compiler calls str,
// when the call is written inside it: the compiler binds str there, and for a
// let of more than one name that scope has not bound it yet (insideOwnName).
//
// The other two halves stay identifier-only on purpose. "Calling a parameter
// means that parameter is a function" is a statement about a bare name, and a
// user function is reached by one: `util.helper` is a module member, which
// this file's graph does not declare and this solver cannot see across files.
func constrainCall(call *mast.CallExpression, narrow func(mast.Expression, kindSet), flow func(mast.Expression, *solvedFunction, int), names *solverNames) {
	switch callee := call.Function.(type) {
	case *mast.Identifier:
		if callee == nil {
			return
		}
		// Calling a parameter means that parameter is a function.
		narrow(callee, ksFn)

		declared, known := names.resolve(callee)
		if !known {
			return
		}
		if declared == nil {
			if params, ok := builtin.ParamSpecs(callee.Value); ok {
				constrainAgainstParams(call, params, narrow)
			}
			return
		}
		target := names.callTarget(declared)
		if target == nil {
			return
		}
		for i, arg := range call.Arguments {
			if i >= len(target.kinds) {
				break
			}
			flow(arg, target, i)
		}

	case *mast.FieldExpression:
		if callee == nil || names.insideOwnName(callee.Left) {
			return
		}
		local, ok := names.scopeOf(callee)
		if !ok {
			return
		}
		name, _, ok := builtinCalleeIn(callee, local)
		if !ok {
			return
		}
		if params, ok := builtin.ParamSpecs(name); ok {
			constrainAgainstParams(call, params, narrow)
		}
	}
}

// constrainAgainstParams narrows each argument against the parameter it lands
// on. An argument count the contract does not admit is left alone: that is
// the arity rule's complaint, not this one's, and narrowing against the wrong
// parameter would turn one diagnostic into two.
func constrainAgainstParams(call *mast.CallExpression, params []builtin.BuiltinParamDoc, narrow func(mast.Expression, kindSet)) {
	if !argumentCountFitsParams(params, len(call.Arguments)) {
		return
	}
	for i, arg := range call.Arguments {
		param, ok := paramForArgument(params, i)
		if !ok {
			continue
		}
		narrow(arg, kindSetFor(param.Kinds))
	}
}

// observeCallSites records the kinds actually passed at each call site in this
// document, for the parameters the body left unconstrained.
//
// This is deliberately weaker than a constraint and is presented as such. A
// function called once with a string is not a function that takes strings; it is
// a function nobody has yet called with anything else. Showing that as the type
// would be the one way this feature could actively mislead.
//
// A call is recorded on the function its callee resolves to and on no other.
// Matched by name, two helpers called `helper`, nested in different functions,
// pooled their callers, and each card listed kinds only the other's callers
// passed. A call through a name that is assigned anywhere is recorded nowhere:
// what it passes went to whatever the name held at the time.
func observeCallSites(program *mast.Program, names *solverNames) {
	if program.NodePositions == nil {
		return
	}
	for node := range program.NodePositions {
		call, ok := node.(*mast.CallExpression)
		if !ok || call == nil {
			continue
		}
		callee, ok := call.Function.(*mast.Identifier)
		if !ok || callee == nil {
			continue
		}
		declared, known := names.resolve(callee)
		if !known {
			continue
		}
		target := names.callTarget(declared)
		if target == nil {
			continue
		}
		for i, arg := range call.Arguments {
			if i >= len(target.observed) {
				break
			}
			if set, ok := literalKindSet(arg); ok {
				target.observed[i] |= set
			}
		}
	}
}

// literalKindSet reads the kind of an argument whose type is visible in the
// syntax. Only literals count. Anything else would need inference, and an
// observation is meant to be what the call site plainly passes, checkable by
// reading the call, rather than one deduction resting on another.
func literalKindSet(e mast.Expression) (kindSet, bool) {
	switch n := e.(type) {
	case *mast.StringLiteral:
		return ksString, true
	case *mast.IntegerLiteral:
		return ksInt, true
	case *mast.FloatLiteral:
		return ksFloat, true
	case *mast.Boolean:
		return ksBool, true
	case *mast.ArrayLiteral:
		return ksArray, true
	case *mast.TemplateLiteral:
		return ksString, true
	case *mast.HashLiteral:
		return ksHash, true
	case *mast.FunctionLiteral:
		return ksFn, true
	case *mast.PrefixExpression:
		if n.Operator == "-" {
			return literalKindSet(n.Right)
		}
		if n.Operator == "!" {
			return ksBool, true
		}
	}
	return 0, false
}
