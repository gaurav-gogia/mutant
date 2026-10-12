package sema

import (
	"strconv"
	"strings"
	"testing"

	"mutant/ast"
)

// seqWrapCount is one more than a uint16 can count. Seq was a uint16, so the
// declaration at this ordinal was the first to wrap to 0 and take the first
// declaration's identity.
const seqWrapCount = 65537

// declarationsNamed returns every declaration of name, in source order.
func declarationsNamed(g *Graph, name string) []*Node {
	var found []*Node
	for _, node := range g.Declarations() {
		if node.Name == name {
			found = append(found, node)
		}
	}
	return found
}

// Mutant permits a name to be bound twice in one scope, and Seq is the whole of
// what keeps the two apart. Nothing pinned that until the counter was found to
// wrap, so the small case is pinned here first: whatever the wide case does, it
// has to still do this.
func TestTwoBindingsOfOneNameAreTwoDeclarations(t *testing.T) {
	g := graphOf(t, "let x = 1;\nx;\nlet x = 2;\nx;\n")

	decls := declarationsNamed(g, "x")
	if len(decls) != 2 {
		t.Fatalf("two bindings of `x` gave %d declarations, want 2", len(decls))
	}

	first, second := decls[0], decls[1]
	if first.ID == second.ID {
		t.Fatalf("both bindings of `x` carry one identity %s", first.ID)
	}
	if first.Seq() != 0 || second.Seq() != 1 {
		t.Fatalf("the two bindings are numbered %d and %d, want 0 and 1",
			first.Seq(), second.Seq())
	}

	if uses := g.UsesOf(first.ID); len(uses) != 1 || uses[0].Start.Line != 2 {
		t.Fatalf("the first binding's uses are %v; the only one is on line 2", uses)
	}
	if uses := g.UsesOf(second.ID); len(uses) != 1 || uses[0].Start.Line != 4 {
		t.Fatalf("the second binding's uses are %v; the only one is on line 4", uses)
	}
}

// A wrapped Seq does not degrade an answer, it swaps one: the later declaration
// overwrites the earlier one in the node table, so the earlier one's identity
// now names the later declaration and carries both sets of uses. Rename on the
// first `let x` would have edited the last one's uses too.
//
// 65,537 bindings is a 720 KB file, which a generator reaches without trying.
// The fixture is built rather than checked in for that reason, and it is cheap:
// the file parses and the graph builds in well under a second.
func TestADeclarationPastTheOldCounterWidthKeepsItsOwnIdentity(t *testing.T) {
	var src strings.Builder
	src.Grow(seqWrapCount * 12)

	// The first binding, and the only use of it in the file.
	src.WriteString("let x = 0;\nx;\n")
	for i := 1; i < seqWrapCount-1; i++ {
		src.WriteString("let x = ")
		src.WriteString(strconv.Itoa(i))
		src.WriteString(";\n")
	}
	// The binding whose ordinal wrapped, and the only use of that one.
	src.WriteString("let x = ")
	src.WriteString(strconv.Itoa(seqWrapCount - 1))
	src.WriteString(";\nx;\n")

	g := graphOf(t, src.String())

	decls := declarationsNamed(g, "x")
	if len(decls) != seqWrapCount {
		t.Fatalf("the file binds `x` %d times and the graph declares it %d times",
			seqWrapCount, len(decls))
	}

	first, last := decls[0], decls[len(decls)-1]
	if first.ID == last.ID {
		t.Fatalf("binding 1 and binding %d share the identity %s, so one of "+
			"them is not reachable", seqWrapCount, first.ID)
	}
	// Widened through uint64 on both sides deliberately: a narrower Seq would
	// make the comparison a compile error rather than the failure this is for.
	if got := uint64(last.Seq()); got != uint64(seqWrapCount-1) {
		t.Fatalf("binding %d is numbered %d, want %d", seqWrapCount, got, seqWrapCount-1)
	}

	// The first binding is used once, on line 2. Two uses here is the defect:
	// it means the last binding's use was filed under the first one's identity.
	uses := g.UsesOf(first.ID)
	if len(uses) != 1 {
		t.Fatalf("the first binding of `x` has %d uses, want 1 -- the use on "+
			"line 2. Lines: %v", len(uses), useLines(uses))
	}
	if uses[0].Start.Line != 2 {
		t.Fatalf("the first binding's one use is on line %d, want 2", uses[0].Start.Line)
	}

	// And its identity still names it, rather than the declaration that wrapped
	// onto it.
	node, found := g.NodeFor(first.ID)
	if !found {
		t.Fatalf("the first binding's identity %s names no declaration", first.ID)
	}
	if node != first {
		t.Fatalf("the first binding's identity names the declaration on line %d, "+
			"and the first binding is on line %d",
			node.DeclRange.Start.Line, first.DeclRange.Start.Line)
	}
}

func useLines(uses []ast.Range) []int {
	lines := make([]int, 0, len(uses))
	for _, use := range uses {
		lines = append(lines, use.Start.Line)
	}
	return lines
}
