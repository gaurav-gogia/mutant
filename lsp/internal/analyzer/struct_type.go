package analyzer

import (
	mast "mutant/ast"

	lsp "github.com/tliron/glsp/protocol_3_16"
)

// Which struct a value holds is not a question about names, so it is not a
// question sema.Graph answers. The graph records that `p.x` was written and
// that `p` refers to some declaration; which field `x` names depends on what
// `p` contains, and that is inference.
//
// This is the syntactic half of that inference, kept from the walk this package
// used to do five of: it finds the struct literal a binding was initialised
// from by looking at the statement that declares it. Snapshot.TypeOf reaches
// strictly more -- a struct arriving through a function's return, or through
// another binding -- and type_definition.go already prefers it. This stays
// because it answers without running the inference pass, and the graph build
// asks it, through structHeldBy, inside Graph's sync.Once.
//
// Both of its questions are answered from one walk per snapshot. They used to
// be a walk of the whole program per question, and the graph build and
// StructFieldDiagnostics each ask once per field read, so a file twice as long
// cost four times as much on every keystroke (M26-SEM-004).

// structTypeNameForDeclaration takes the identifier that DECLARES a binding,
// never a use of it. Handing it a use would mean first asking what the use
// refers to, and that is the question this helps answer.
func (s *Snapshot) structTypeNameForDeclaration(target *mast.Identifier) (string, bool) {
	if s == nil || s.Program == nil || target == nil {
		return "", false
	}
	s.buildStructIndex()
	name, ok := s.structLiterals[target]
	return name, ok
}

// buildStructIndex is the one walk both lookups are answered from. It runs
// inside Graph's sync.Once when the graph build asks structHeldBy, so it must
// never ask for the graph.
func (s *Snapshot) buildStructIndex() {
	s.structIndexOnce.Do(func() {
		s.structLiterals = make(map[*mast.Identifier]string)
		s.structDecls = make(map[string]*mast.StructStatement)
		for _, stmt := range s.Program.Statements {
			if decl, ok := stmt.(*mast.StructStatement); ok && decl != nil && decl.Name != nil {
				if _, seen := s.structDecls[decl.Name.Value]; !seen {
					s.structDecls[decl.Name.Value] = decl
				}
			}
			s.indexStructLiteralsInStatement(stmt)
		}
	})
}

// indexStructLiteral records every name a let binds from a named struct
// literal. The first record wins, which is what the per-target search did: it
// stopped at the first statement, in this walk's order, that declared target.
func (s *Snapshot) indexStructLiteral(names []*mast.Identifier, value mast.Expression) {
	literal, ok := value.(*mast.StructLiteral)
	if !ok || literal == nil || literal.Name == nil {
		return
	}
	for _, name := range names {
		if name == nil {
			continue
		}
		if _, seen := s.structLiterals[name]; !seen {
			s.structLiterals[name] = literal.Name.Value
		}
	}
}

func (s *Snapshot) indexStructLiteralsInStatement(stmt mast.Statement) {
	switch node := stmt.(type) {
	case *mast.LetStatement:
		names := node.Names
		if len(names) == 0 && node.Name != nil {
			names = []*mast.Identifier{node.Name}
		}
		s.indexStructLiteral(names, node.Value)
		if node.Value != nil {
			s.indexStructLiteralsInExpression(node.Value)
		}
	case *mast.ReturnStatement:
		for _, expr := range node.ReturnValues {
			s.indexStructLiteralsInExpression(expr)
		}
		if len(node.ReturnValues) == 0 && node.ReturnValue != nil {
			s.indexStructLiteralsInExpression(node.ReturnValue)
		}
	case *mast.ExpressionStatement:
		if node.Expression != nil {
			s.indexStructLiteralsInExpression(node.Expression)
		}
	case *mast.BlockStatement:
		for _, inner := range node.Statements {
			s.indexStructLiteralsInStatement(inner)
		}
	case *mast.ForInStatement:
		if node.Iterable != nil {
			s.indexStructLiteralsInExpression(node.Iterable)
		}
		if node.Body != nil {
			s.indexStructLiteralsInStatement(node.Body)
		}
	case *mast.WhileStatement:
		if node.Condition != nil {
			s.indexStructLiteralsInExpression(node.Condition)
		}
		if node.Body != nil {
			s.indexStructLiteralsInStatement(node.Body)
		}
	case *mast.ForStatement:
		if node.Init != nil {
			s.indexStructLiteralsInStatement(node.Init)
		}
		if node.Condition != nil {
			s.indexStructLiteralsInExpression(node.Condition)
		}
		if node.Post != nil {
			s.indexStructLiteralsInExpression(node.Post)
		}
		if node.Body != nil {
			s.indexStructLiteralsInStatement(node.Body)
		}
	}
}

func (s *Snapshot) indexStructLiteralsInExpression(expr mast.Expression) {
	switch node := expr.(type) {
	case *mast.FunctionLiteral:
		if node.Body != nil {
			s.indexStructLiteralsInStatement(node.Body)
		}
	case *mast.IfExpression:
		if node.Condition != nil {
			s.indexStructLiteralsInExpression(node.Condition)
		}
		if node.Consequence != nil {
			s.indexStructLiteralsInStatement(node.Consequence)
		}
		if node.Alternative != nil {
			s.indexStructLiteralsInStatement(node.Alternative)
		}
	case *mast.MatchExpression:
		if node.Subject != nil {
			s.indexStructLiteralsInExpression(node.Subject)
		}
		for _, arm := range node.Arms {
			if arm == nil || arm.Body == nil {
				continue
			}
			s.indexStructLiteralsInStatement(arm.Body)
		}
	case *mast.CallExpression:
		if node.Function != nil {
			s.indexStructLiteralsInExpression(node.Function)
		}
		for _, arg := range node.Arguments {
			s.indexStructLiteralsInExpression(arg)
		}
	case *mast.PrefixExpression:
		if node.Right != nil {
			s.indexStructLiteralsInExpression(node.Right)
		}
	case *mast.InfixExpression:
		if node.Left != nil {
			s.indexStructLiteralsInExpression(node.Left)
		}
		if node.Right != nil {
			s.indexStructLiteralsInExpression(node.Right)
		}
	case *mast.IndexExpression:
		if node.Left != nil {
			s.indexStructLiteralsInExpression(node.Left)
		}
		if node.Index != nil {
			s.indexStructLiteralsInExpression(node.Index)
		}
	case *mast.AssignExpression:
		if node.Left != nil {
			s.indexStructLiteralsInExpression(node.Left)
		}
		if node.Value != nil {
			s.indexStructLiteralsInExpression(node.Value)
		}
	case *mast.FieldExpression:
		if node.Left != nil {
			s.indexStructLiteralsInExpression(node.Left)
		}
	case *mast.StructLiteral:
		for _, field := range node.Fields {
			if field != nil && field.Value != nil {
				s.indexStructLiteralsInExpression(field.Value)
			}
		}
	case *mast.ArrayLiteral:
		for _, element := range node.Elements {
			s.indexStructLiteralsInExpression(element)
		}
	case *mast.TemplateLiteral:
		for _, element := range node.Parts {
			s.indexStructLiteralsInExpression(element)
		}
	case *mast.HashLiteral:
		for _, pair := range node.Pairs {
			s.indexStructLiteralsInExpression(pair.Key)
			s.indexStructLiteralsInExpression(pair.Value)
		}
	case *mast.MacroLiteral:
		if node.Body != nil {
			s.indexStructLiteralsInStatement(node.Body)
		}
	}
}

func (s *Snapshot) structFieldBinding(structName, fieldName string) (binding, bool) {
	if s == nil || s.Program == nil || structName == "" || fieldName == "" {
		return binding{}, false
	}
	for _, stmt := range s.Program.Statements {
		structStmt, ok := stmt.(*mast.StructStatement)
		if !ok || structStmt.Name == nil || structStmt.Name.Value != structName {
			continue
		}
		for _, field := range structStmt.Fields {
			if field == nil || field.Value != fieldName {
				continue
			}
			rng, ok := s.identifierRange(field)
			if !ok {
				return binding{}, false
			}
			return binding{ident: field, rng: rng, kind: lsp.CompletionItemKindField}, true
		}
	}
	return binding{}, false
}
