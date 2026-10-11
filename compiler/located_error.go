package compiler

import (
	"errors"

	"mutant/ast"
)

// LocatedError is a compile error together with the source range of the node
// whose compilation raised it.
//
// The compiler has always known where it was -- Compile tracks the position
// every instruction is attributed to -- and has always thrown that away on the
// way out, so a refusal reached its reader as a sentence with no line. The
// language server cannot put a sentence with no line anywhere, which is half of
// why the editor reported almost none of the compiler's refusals (M26-LSP-028).
//
// Error is the wrapped error's text, unchanged, so nothing that prints a compile
// error or compares its words sees a difference. Unwrap keeps errors.As working
// for a caller that needs the *sema.Refusal underneath.
type LocatedError struct {
	Err   error
	Range ast.Range
}

func (e *LocatedError) Error() string { return e.Err.Error() }

func (e *LocatedError) Unwrap() error { return e.Err }

// ErrorRange returns the range a compile error was raised at, when the node
// being compiled had one.
func ErrorRange(err error) (ast.Range, bool) {
	var located *LocatedError
	if errors.As(err, &located) {
		return located.Range, true
	}
	return ast.Range{}, false
}

// locate attaches rng to err unless err already carries a range.
//
// Called by Compile on the way out of every node, so the first frame to see an
// error is the innermost node with a recorded range, and every enclosing frame
// passes it on unchanged. That is the most precise answer the tree holds: the
// break statement for "break used outside of for loop", the identifier for
// "undefined variable", the assignment for "cannot assign to builtin".
func locate(err error, rng ast.Range, known bool) error {
	if err == nil || !known || !rng.Start.IsValid() {
		return err
	}
	var located *LocatedError
	if errors.As(err, &located) {
		return err
	}
	return &LocatedError{Err: err, Range: rng}
}
