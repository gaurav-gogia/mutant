package lexer

import (
	"reflect"
	"strings"
	"testing"

	"mutant/token"
)

// NewAt lexes a fragment cut out of a file and reports positions in the file's
// coordinates. It replaced padding -- a newline per line and a space per column
// prepended to the text -- which was quadratic in the fragment's column
// (M26-LEX-009) and which got the byte offsets wrong, because one byte of
// padding stands in for a line of any width (M26-LEX-018).
//
// Every assertion here compares what the lexer reports against the containing
// file itself, computed by offsetIn and lineColIn below, rather than against
// numbers written down in the test. Numbers written down would have agreed with
// the padded lexer just as readily, since the padding got line and column
// right; it was the relationship between the three coordinates that broke.

// cut locates fragment inside file and returns the position of its first byte,
// which is what NewAt is given. It fails if the fragment is absent or appears
// more than once, because either would make the rest of the test meaningless.
func cut(t *testing.T, file, fragment string) token.Position {
	t.Helper()
	at := strings.Index(file, fragment)
	if at < 0 {
		t.Fatalf("the fragment %q is not in the file", fragment)
	}
	if strings.Contains(file[at+1:], fragment) {
		t.Fatalf("the fragment %q appears more than once, so its position is ambiguous", fragment)
	}
	line, column := lineColIn(file, at)
	return token.Position{Line: line, Column: column, Offset: at}
}

// offsetIn is the byte offset of a 1-based line and column in file. It is the
// independent answer every reported Offset is checked against.
func offsetIn(file string, line, column int) int {
	at := 0
	for l := 1; l < line; l++ {
		next := strings.IndexByte(file[at:], '\n')
		if next < 0 {
			return -1
		}
		at += next + 1
	}
	return at + column - 1
}

// lineColIn is the inverse: the 1-based line and column of a byte offset.
func lineColIn(file string, offset int) (line, column int) {
	line = 1 + strings.Count(file[:offset], "\n")
	lineStart := strings.LastIndexByte(file[:offset], '\n') + 1
	return line, offset - lineStart + 1
}

// fragmentTokens drains a fragment lexed at at, bounded like allTokens.
func fragmentTokens(t *testing.T, fragment string, at token.Position) []token.Token {
	t.Helper()
	l := NewAt(fragment, at)
	out := []token.Token{}
	for i := 0; i <= len(fragment)+16; i++ {
		tok := l.NextToken()
		out = append(out, tok)
		if tok.Type == token.EOF {
			return out
		}
	}
	t.Fatalf("the lexer never reached EOF on the fragment %q", fragment)
	return nil
}

func TestNewAtPutsTheFragmentsFirstTokenWhereTheFileHasIt(t *testing.T) {
	file := "let a = 1;\n\n        let s = total + 1;\n"
	fragment := "total + 1"
	at := cut(t, file, fragment)

	first := fragmentTokens(t, fragment, at)[0]
	if first.Start != at {
		t.Fatalf("the first token starts at %+v, want the fragment's own position %+v", first.Start, at)
	}
	if got := file[first.Start.Offset:first.End.Offset]; got != "total" {
		t.Fatalf("slicing the file at the first token's range gives %q, want %q", got, "total")
	}
}

// The relationship between the three coordinates is the property the padding
// broke, so it is asserted for every token rather than for one.
func TestEveryPositionNewAtReportsIsAFilePosition(t *testing.T) {
	file := "let a = 1;\nlet b = 2;\n\n              let x = a + b * 2 - (a / b);\n"
	fragment := "a + b * 2 - (a / b)"
	at := cut(t, file, fragment)

	for _, tok := range fragmentTokens(t, fragment, at) {
		for what, pos := range map[string]token.Position{"start": tok.Start, "end": tok.End} {
			want := offsetIn(file, pos.Line, pos.Column)
			if want != pos.Offset {
				t.Errorf("%s of %s %q: offset %d, but line %d column %d is offset %d",
					what, tok.Type, tok.Literal, pos.Offset, pos.Line, pos.Column, want)
			}
		}
		if tok.Type == token.EOF {
			continue
		}
		if got := file[tok.Start.Offset:tok.End.Offset]; got != tok.Literal {
			t.Errorf("%s: the file at the token's range holds %q, but the token's literal is %q",
				tok.Type, got, tok.Literal)
		}
	}
}

// The column origin applies to the fragment's first line only. A newline inside
// the fragment starts a column 1 of the file's own, and adding the origin to it
// would put every later line to the right of where it is.
func TestANewlineInsideTheFragmentStartsColumnOneAgain(t *testing.T) {
	file := "let a = 1;\n\n          let x = a +\nb +\n    c;\n"
	fragment := "a +\nb +\n    c"
	at := cut(t, file, fragment)

	for _, tok := range fragmentTokens(t, fragment, at) {
		if tok.Type != token.IDENT {
			continue
		}
		// Each of a, b and c appears once in the fragment, so its index there
		// gives its file offset, and the file gives the line and column that
		// offset really has. Nothing here is written down.
		within := strings.Index(fragment, tok.Literal)
		wantLine, wantColumn := lineColIn(file, at.Offset+within)
		if tok.Start.Line != wantLine || tok.Start.Column != wantColumn {
			t.Errorf("%q is reported at line %d column %d, want line %d column %d",
				tok.Literal, tok.Start.Line, tok.Start.Column, wantLine, wantColumn)
		}
	}
}

// The control that matters most: a whole file is a fragment at line 1, column
// 1, offset 0, so New must be NewAt with that origin and nothing else.
func TestNewIsNewAtTheTopOfAFile(t *testing.T) {
	for _, src := range []string{
		"let a = 1;\nlet b = a + 2;\n",
		"// a comment\nlet s = \"text\";\n",
		"let t = \"a${ b + 1 }c\";\n",
		"r\"C:\\evidence\\\";\n",
		"",
	} {
		plain := allTokens(t, src)
		placed := fragmentTokens(t, src, token.Position{Line: 1, Column: 1, Offset: 0})
		if len(plain) != len(placed) {
			t.Fatalf("%q: New gave %d tokens and NewAt gave %d: %s vs %s",
				src, len(plain), len(placed), types(plain), types(placed))
		}
		for i := range plain {
			// DeepEqual rather than ==, because a TEMPLATE token carries its
			// parts as a slice, and those parts are exactly what has to agree.
			if !reflect.DeepEqual(plain[i], placed[i]) {
				t.Errorf("%q token %d: New gave %+v, NewAt gave %+v", src, i, plain[i], placed[i])
			}
		}
	}
}

// skipLineComment used to slice l.input by a position's Offset. With an origin
// set that offset belongs to the file and indexes nothing in the fragment, so
// the comment's text would be cut at the wrong place or the slice would panic.
func TestAFragmentsCommentKeepsItsOwnTextAndAFilePosition(t *testing.T) {
	file := "let a = 1;\n\n          b + 1 // why b\n"
	fragment := "b + 1 // why b"
	at := cut(t, file, fragment)

	l := NewAt(fragment, at)
	drain(l)

	comments := l.Comments()
	if len(comments) != 1 {
		t.Fatalf("got %d comments, want 1: %+v", len(comments), comments)
	}
	got := comments[0]
	if got.Text != "// why b" {
		t.Errorf("comment text is %q, want %q", got.Text, "// why b")
	}
	if want := offsetIn(file, got.Start.Line, got.Start.Column); want != got.Start.Offset {
		t.Errorf("comment starts at offset %d, but line %d column %d is offset %d",
			got.Start.Offset, got.Start.Line, got.Start.Column, want)
	}
	if sliced := file[got.Start.Offset:got.End.Offset]; sliced != got.Text {
		t.Errorf("the file at the comment's range holds %q, want %q", sliced, got.Text)
	}
}

// readStringToken slices l.input for the literal's spelling, and bodyPosition
// used to repeat currentPos's arithmetic instead of building on it -- so a
// literal was the one construct a fragment could get wrong in two ways at once.
func TestAFragmentsStringLiteralKeepsItsSpellingAndItsHolePositions(t *testing.T) {
	file := "let a = 1;\n\n       let s = \"x${ a + 1 }y\";\n"
	fragment := "\"x${ a + 1 }y\""
	at := cut(t, file, fragment)

	toks := fragmentTokens(t, fragment, at)
	if toks[0].Type != token.TEMPLATE {
		t.Fatalf("the literal lexed as %s, want TEMPLATE: %s", toks[0].Type, types(toks))
	}
	if toks[0].Raw != fragment {
		t.Errorf("the spelling recorded is %q, want the literal itself %q", toks[0].Raw, fragment)
	}
	if got := file[toks[0].Start.Offset:toks[0].End.Offset]; got != fragment {
		t.Errorf("the file at the literal's range holds %q, want %q", got, fragment)
	}

	var holes int
	for _, part := range toks[0].Parts {
		if !part.Expression {
			continue
		}
		holes++
		if want := offsetIn(file, part.Start.Line, part.Start.Column); want != part.Start.Offset {
			t.Errorf("the hole %q starts at offset %d, but line %d column %d is offset %d",
				part.Text, part.Start.Offset, part.Start.Line, part.Start.Column, want)
		}
		if got := file[part.Start.Offset : part.Start.Offset+len(part.Text)]; got != part.Text {
			t.Errorf("the file at the hole's start holds %q, want the hole's text %q", got, part.Text)
		}
	}
	if holes != 1 {
		t.Fatalf("got %d holes, want 1", holes)
	}
}

// A zero-valued origin would put the fragment on line 0, which no file has.
// NewAt's doc says the caller passes a 1-based position; this records what the
// requirement is for, so that a caller passing token.Position{} is a mistake
// someone has written down rather than a surprise in a diagnostic.
func TestAFragmentAtTheTopOfAFileIsLineOneColumnOne(t *testing.T) {
	at := token.Position{Line: 1, Column: 1, Offset: 0}
	first := fragmentTokens(t, "a", at)[0]
	if first.Start != at {
		t.Fatalf("the first token of a fragment at the top of a file starts at %+v, want %+v", first.Start, at)
	}
}
