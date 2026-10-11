package analyzer

import (
	"fmt"
	"sort"
	"strings"

	mast "mutant/ast"
	"mutant/builtin"
	"mutant/token"

	lsp "github.com/tliron/glsp/protocol_3_16"
)

func (s *Snapshot) SignatureHelp(pos lsp.Position) (*lsp.SignatureHelp, bool) {
	if s == nil || s.Program == nil {
		return nil, false
	}

	call, ok := s.callExpressionAt(pos)
	if !ok {
		return nil, false
	}

	sig, ok := s.signatureInformationForCall(call, pos)
	if !ok {
		return nil, false
	}
	// The offsets stay bytes inside the package, where InlayHints reads them,
	// and become UTF-16 units here, where they leave for the client.
	sig = withUTF16ParameterOffsets(sig)

	activeSignature := lsp.UInteger(0)
	activeParameter := lsp.UInteger(s.activeParameterForCall(call, pos))

	result := &lsp.SignatureHelp{
		Signatures:      []lsp.SignatureInformation{sig},
		ActiveSignature: &activeSignature,
	}

	if len(sig.Parameters) > 0 {
		if int(activeParameter) >= len(sig.Parameters) {
			activeParameter = lsp.UInteger(len(sig.Parameters) - 1)
		}
		result.ActiveParameter = &activeParameter
	}

	return result, true
}

func (s *Snapshot) callExpressionAt(pos lsp.Position) (*mast.CallExpression, bool) {
	if s == nil || s.Program == nil || s.Program.NodePositions == nil {
		return nil, false
	}

	var best *mast.CallExpression
	bestSize := int(^uint(0) >> 1)

	line, column := s.TokenPosition(pos)
	for node, rng := range s.Program.NodePositions {
		call, ok := node.(*mast.CallExpression)
		if !ok || call == nil || !rng.IsValid() {
			continue
		}
		if !rangeContains(rng, line, column) {
			continue
		}
		size := rng.End.Offset - rng.Start.Offset
		if size < bestSize {
			best = call
			bestSize = size
		}
	}

	if best == nil {
		return nil, false
	}
	return best, true
}

func (s *Snapshot) signatureInformationForCall(call *mast.CallExpression, pos lsp.Position) (lsp.SignatureInformation, bool) {
	if call == nil || call.Function == nil {
		return lsp.SignatureInformation{}, false
	}

	switch fn := call.Function.(type) {
	case *mast.FunctionLiteral:
		return s.functionLiteralSignature(fn, functionDisplayName("fn", fn.Name)), true
	case *mast.Identifier:
		if fn == nil || fn.Value == "" {
			return lsp.SignatureInformation{}, false
		}
		if sig, ok := macroSpecialFormSignatureInformation(fn.Value); ok {
			return sig, true
		}
		// A binding of a builtin's name is what the VM calls, so it is asked
		// about first, with the compiler's own predicate: `let max = fn(a, b,
		// c)` followed by `max(1, 2, 3)` is the user's max (M26-SEM-011).
		at := positionInsideIdentifier(s, fn, pos)
		if builtin.GetBuiltinByName(fn.Value) != nil && !s.isBoundAt(fn.Value, at) {
			if sig, ok := builtinSignatureInformation(fn.Value); ok {
				return sig, true
			}
			return lsp.SignatureInformation{Label: fmt.Sprintf("%s(...)", fn.Value)}, true
		}

		resolved, ok := s.resolveDefinition(at)
		if !ok || resolved.ident == nil {
			return lsp.SignatureInformation{}, false
		}
		literal, ok := s.functionLiteralForBindingIdent(resolved.ident)
		if !ok {
			return lsp.SignatureInformation{}, false
		}
		return s.functionLiteralSignature(literal, functionDisplayName(fn.Value, literal.Name)), true
	case *mast.FieldExpression:
		// The dotted spelling of a builtin. Without this arm it fell to the
		// default below, so `fs.read(` offered nothing while `fs_read(` offered
		// the typed signature -- and the inlay hints built on this went with it.
		//
		// The scope is the callee's own, not the one at pos: InlayHints asks
		// with the zero position, where a parameter named str is not yet bound,
		// and `str.repeat(3)` inside the function would become str_repeat.
		name, _, ok := builtinCalleeIn(fn, s.localScopeAtNode(fn))
		if !ok {
			return lsp.SignatureInformation{}, false
		}
		if sig, ok := builtinSignatureInformation(name); ok {
			return sig, true
		}
		return lsp.SignatureInformation{Label: fmt.Sprintf("%s(...)", name)}, true
	default:
		return lsp.SignatureInformation{}, false
	}
}

// functionLiteralSignature builds signature help for a user-defined function:
// its parameter names.
//
// It used to append the kinds fn_solver.go deduced, `low_byte(v: INTEGER)`,
// which read as a type the parameter is held to. A user function has no
// parameter types; the solver describes how the body uses a name, and working
// programs pass that parameter other kinds (M26-SEM-019). The function card
// says it as what it is, "used as INTEGER".
func (s *Snapshot) functionLiteralSignature(fn *mast.FunctionLiteral, displayName string) lsp.SignatureInformation {
	if fn == nil {
		return lsp.SignatureInformation{Label: fmt.Sprintf("%s()", displayName)}
	}

	var label strings.Builder
	label.WriteString(displayName)
	label.WriteString("(")
	parameters := make([]lsp.ParameterInformation, 0, len(fn.Parameters))
	for _, p := range fn.Parameters {
		if p == nil {
			continue
		}
		if len(parameters) > 0 {
			label.WriteString(", ")
		}
		// A parameter is addressed by where it lies in the label, recorded
		// as the label is joined, the way a builtin's is. A string label is
		// found by the client as the first whole-word match of the name, so
		// once the solver's kinds stopped making each label unique, the x of
		// `let x = fn(x)` was highlighted on the function's name in x(x). The
		// offsets are bytes, which is what parameterHintName reads them as;
		// SignatureHelp converts them to the protocol's UTF-16 units.
		start := label.Len()
		label.WriteString(p.Value)
		parameters = append(parameters, lsp.ParameterInformation{
			Label: [2]lsp.UInteger{lsp.UInteger(start), lsp.UInteger(label.Len())},
		})
	}
	label.WriteString(")")

	return lsp.SignatureInformation{
		Label:      label.String(),
		Parameters: parameters,
	}
}

// withUTF16ParameterOffsets converts sig's parameter offsets from the bytes
// they are recorded in to the UTF-16 code units LSP 3.16 counts a label in.
// The two agree on an ASCII label, which every builtin signature is, but a
// user function's label is made of its names, and the lexer admits any Unicode
// letter in a name: in f(é, b) the b starts at byte 6 and at unit 5, and its
// byte pair [6,7), counted by the client in units, would mark the ")" after it.
//
// A pair that does not fit the label is sent as the empty range, which
// highlights nothing, rather than as offsets that would mark some other text.
func withUTF16ParameterOffsets(sig lsp.SignatureInformation) lsp.SignatureInformation {
	if len(sig.Parameters) == 0 {
		return sig
	}
	parameters := make([]lsp.ParameterInformation, len(sig.Parameters))
	for i, param := range sig.Parameters {
		if span, ok := param.Label.([2]lsp.UInteger); ok {
			start, end := int(span[0]), int(span[1])
			if start <= end && end <= len(sig.Label) {
				param.Label = [2]lsp.UInteger{
					lsp.UInteger(utf16Len(sig.Label[:start])),
					lsp.UInteger(utf16Len(sig.Label[:end])),
				}
			} else {
				param.Label = [2]lsp.UInteger{0, 0}
			}
		}
		parameters[i] = param
	}
	sig.Parameters = parameters
	return sig
}

func functionDisplayName(fallback, literalName string) string {
	if literalName != "" {
		return literalName
	}
	return fallback
}

func (s *Snapshot) activeParameterForCall(call *mast.CallExpression, pos lsp.Position) int {
	if s == nil || s.Program == nil || call == nil || len(call.Arguments) == 0 {
		return 0
	}

	type argRange struct {
		index int
		rng   mast.Range
	}
	ranges := make([]argRange, 0, len(call.Arguments))
	for i, arg := range call.Arguments {
		rng, ok := s.Program.RangeOf(arg)
		if !ok {
			continue
		}
		ranges = append(ranges, argRange{index: i, rng: rng})
	}

	if len(ranges) == 0 {
		return 0
	}

	sort.Slice(ranges, func(i, j int) bool {
		if ranges[i].rng.Start.Line != ranges[j].rng.Start.Line {
			return ranges[i].rng.Start.Line < ranges[j].rng.Start.Line
		}
		return ranges[i].rng.Start.Column < ranges[j].rng.Start.Column
	})

	line, column := s.TokenPosition(pos)
	for _, item := range ranges {
		if rangeContains(item.rng, line, column) {
			return item.index
		}
		if positionBeforeTokenPosition(line, column, item.rng.Start) {
			return item.index
		}
	}

	return ranges[len(ranges)-1].index
}

// positionBeforeTokenPosition takes an already-converted 1-based line and byte
// column, so that a caller comparing one cursor against many ranges converts it
// once rather than per comparison -- and so that the conversion happens in one
// place rather than here as well.
func positionBeforeTokenPosition(line, column int, tokenPos token.Position) bool {
	if line != tokenPos.Line {
		return line < tokenPos.Line
	}
	return column < tokenPos.Column
}

func positionInsideIdentifier(s *Snapshot, ident *mast.Identifier, fallback lsp.Position) lsp.Position {
	if s == nil || s.Program == nil || ident == nil {
		return fallback
	}
	rng, ok := s.Program.RangeOf(ident)
	if !ok {
		return fallback
	}
	return s.Position(rng.Start)
}

// functionLiteralForBindingIdent returns the function literal a let bound to
// target, at any depth.
//
// It used to walk the program for each question and stop at the first function
// literal it met, so a helper declared inside main never had signature help, a
// function card or parameter hints (M26-SEM-008). Searching each body as well
// would have answered correctly and made InlayHints, which asks once per call,
// quadratic in the size of the file; the index is built once per snapshot
// instead.
func (s *Snapshot) functionLiteralForBindingIdent(target *mast.Identifier) (*mast.FunctionLiteral, bool) {
	if s == nil || s.Program == nil || target == nil {
		return nil, false
	}
	literal, ok := s.boundFunctionLiterals()[target]
	return literal, ok
}

// boundFunctionLiterals maps each name a let binds to a function literal to
// that literal. NodePositions records every let at any depth, so one pass over
// it stands in for the walk.
func (s *Snapshot) boundFunctionLiterals() map[*mast.Identifier]*mast.FunctionLiteral {
	s.boundFnOnce.Do(func() {
		s.boundFns = make(map[*mast.Identifier]*mast.FunctionLiteral)
		for node := range s.Program.NodePositions {
			let, ok := node.(*mast.LetStatement)
			if !ok || let == nil {
				continue
			}
			literal, ok := let.Value.(*mast.FunctionLiteral)
			if !ok || literal == nil {
				continue
			}
			names := let.Names
			if len(names) == 0 && let.Name != nil {
				names = []*mast.Identifier{let.Name}
			}
			for _, name := range names {
				if name != nil {
					s.boundFns[name] = literal
				}
			}
		}
	})
	return s.boundFns
}
