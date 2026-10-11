package parity

// What the editor calls a duplicate declaration, held against what the program
// does.
//
// The rule's premise is that one of two declarations of a name is pointless.
// That is a claim about the program, so it is checkable against the program:
// every row below is compiled and run, and the value it produces is the reason
// the lint may or may not speak. A row that returns 6 because both `Point`s did
// work is not a row where one of them is redundant.
//
// It is written this way because the rule used to walk the file with a scope
// chain of its own, and that chain disagreed with the compiler twice: it
// reported a shadow as a duplicate, and it filed type names beside values. Both
// disagreements were silent, and both carried a quick fix -- "Remove duplicate
// top-level declaration" -- whose edit deletes a line of a working program.

import (
	"strings"
	"testing"

	"mutant/lsp/api"
)

// compilerRefusesADuplicate reports whether the compiler rejects the program
// for a name declared twice in one scope, and returns what it said. Other
// refusals are not this rule's business, so they are not counted as agreement.
//
// Three sentences rather than one because the rule has three shapes: a single
// name taken twice, a whole multi-name binding with nothing new on it, and a
// parameter list naming one parameter twice. They are sema's words -- see
// sema.DuplicateDeclarationRefusal and sema.DuplicateParameterRefusal -- and
// matching on them here is deliberate: if the wording changes, this test should
// be made to say so rather than quietly passing on a substring.
func compilerRefusesADuplicate(src string) (string, bool) {
	message, refused := compilerComplaint(src)
	if !refused {
		return "", false
	}
	return message, strings.Contains(message, "already declared in this scope") ||
		strings.Contains(message, "declares nothing new") ||
		strings.Contains(message, "is declared twice")
}

// duplicateComplaints returns what the lint says about duplicate declarations,
// and nothing else: a row may legitimately draw an unused-declaration warning.
func duplicateComplaints(src string) []string {
	said := make([]string, 0, 2)
	for _, d := range api.Lint(src) {
		if d.Source == "mutant-lint" && strings.Contains(d.Message, "duplicate") {
			said = append(said, d.Message)
		}
	}
	return said
}

// redeclarationRefusals returns the refusals the editor reports for a let that
// declares nothing new: the diagnostics tagged "mutant-declarations". The tag
// is spelled out rather than imported because it is what the CLI prints and
// what a client filters on.
func redeclarationRefusals(src string) []api.Diagnostic {
	refusals := make([]api.Diagnostic, 0, 1)
	for _, d := range api.Lint(src) {
		if d.Source == "mutant-declarations" {
			refusals = append(refusals, d)
		}
	}
	return refusals
}

// saysWhatTheBuildSays reports whether one of diagnostics is an Error that
// reads exactly message: the compiler's own sentence, not a paraphrase of it.
func saysWhatTheBuildSays(diagnostics []api.Diagnostic, message string) bool {
	for _, d := range diagnostics {
		if d.Severity == api.SeverityError && d.Message == message {
			return true
		}
	}
	return false
}

// TestNothingIsADuplicateWhileBothDeclarationsAreLive pins the silence, and
// pins it to a value rather than to an opinion: each program is run, and the
// number it produces is only reachable if both declarations of the name are
// doing something.
func TestNothingIsADuplicateWhileBothDeclarationsAreLive(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{
			// 20 is only 20 because the parameter shadows the top-level x. If
			// the outer x were what `x * 10` read, this would be 10.
			name: "a parameter shadows a top-level let",
			src: `let x = 1;
let f = fn(x) { return x * 10; };
f(2);
`,
			want: "INTEGER(20)",
		},
		{
			// ... and the outer one is still 1 after the function returns,
			// which is what makes it a shadow rather than a second attempt at
			// the same binding.
			name: "the shadowed let survives the scope that shadowed it",
			src: `let x = 1;
let f = fn() { let x = 2; return x; };
f();
x;
`,
			want: "INTEGER(1)",
		},
		{
			// 6 is 5 from the struct and 1 from the value, added in one
			// expression. A type name never enters the compiler's symbol table,
			// so it takes nothing from a value of the same name.
			name: "a struct name and a value of that name are both live",
			src: `struct Point { x };
let Point = 1;
let p = Point{x: 5};
p.x + Point;
`,
			want: "INTEGER(6)",
		},
		{
			name: "an enum name and a value of that name are both live",
			src: `enum Colour { Red, Green };
let Colour = 9;
let c = Colour.Red;
Colour;
`,
			want: "INTEGER(9)",
		},
		{
			// Two loops over one binding name in one scope is how the language
			// is written. The walk this replaced never recorded a loop binding
			// at all, which got the answer right for the wrong reason.
			name: "two loops bind the same name in one scope",
			src: `for (item in [1, 2]) { item; }
for (item in [3, 4]) { item; }
7;
`,
			want: "INTEGER(7)",
		},
		{
			// The discard may be bound as often as the error idiom needs it.
			name: "the discard is bound twice",
			src: `let a, _ = gets();
let b, _ = gets();
7;
`,
			want: "INTEGER(7)",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			value, err := evalViaVM(t, c.src)
			if err != nil {
				t.Fatalf("the program does not run, so it cannot show the "+
					"declarations are live: %v\n\n%s", err, c.src)
			}
			if got := normalize(value); got != c.want {
				t.Fatalf("value = %s, want %s\n\n%s", got, c.want, c.src)
			}
			if said := duplicateComplaints(c.src); len(said) != 0 {
				t.Fatalf("the editor calls a declaration of this running "+
					"program a duplicate: %v\n\n%s", said, c.src)
			}
		})
	}
}

// TestASecondDeclarationInOneScopeIsStillReported is the other direction, and
// the reason the rule exists: these really are two declarations of one binding,
// and the first one never gets read.
//
// Three of these rows are refused by the build, and the editor says so the way
// the build does: an Error, in the compiler's own sentence, and no lint warning
// beside it. A let that declares nothing new is reported from
// "mutant-declarations"; a parameter list that names one parameter twice does
// not parse, so there the parser's Error is the build's. All three used to draw
// a Warning in the rule's own words -- "duplicate top-level declaration `x`" --
// which said less than the build would, and which for the top-level row came
// with a quick fix that deleted the line (M26-LSP-029).
//
// The two struct rows are not refusals: a type name never enters the compiler's
// symbol table, so two structs of one name is a lint finding about a program
// that compiles. Their wording is asserted rather than merely the presence of a
// complaint, because the two spellings are not synonyms. Only "duplicate
// top-level declaration" is offered the quick fix that deletes the
// declaration, and that is safe only at the top level.
func TestASecondDeclarationInOneScopeIsStillReported(t *testing.T) {
	for _, c := range []struct {
		name, src, want string
		refused         bool
	}{
		{
			refused: true,
			name:    "two top-level lets",
			src: `let x = 1;
let x = 2;
x;
`,
		},
		{
			refused: true,
			name:    "two lets in one function body",
			src: `let f = fn() { let a = 1; let a = 2; return a; };
f();
`,
		},
		{
			refused: true,
			name:    "two parameters of one function",
			src: `let f = fn(a, a) { return a; };
7;
`,
		},
		{
			name: "two structs of one name",
			src: `struct P { x };
struct P { y };
7;
`,
			want: "duplicate top-level declaration `P`",
		},
		{
			// A struct written inside a function is not a top-level statement,
			// so it gets the wording that carries no line-deleting quick fix.
			name: "two structs of one name inside a function",
			src: `let f = fn() {
  struct Inner { a };
  struct Inner { b };
  return 7;
};
f();
`,
			want: "duplicate declaration `Inner`",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			if c.refused {
				message, refused := compilerRefusesADuplicate(c.src)
				if !refused {
					t.Fatalf("the compiler accepts a second declaration in one "+
						"scope (%q), so the editor and the build disagree:\n\n%s",
						message, c.src)
				}
				if !saysWhatTheBuildSays(api.Lint(c.src), message) {
					t.Fatalf("the build refuses this with %q, and the editor "+
						"has no Error that says so: %v\n\n%s",
						message, api.Lint(c.src), c.src)
				}
				if said := duplicateComplaints(c.src); len(said) != 0 {
					t.Fatalf("the editor reports the build's refusal and a "+
						"lint warning in its own words as well: %v\n\n%s",
						said, c.src)
				}
				return
			}
			if _, err := evalViaVM(t, c.src); err != nil {
				t.Fatalf("the program does not compile, so the rule is not "+
					"what is being tested: %v\n\n%s", err, c.src)
			}
			said := duplicateComplaints(c.src)
			for _, message := range said {
				if message == c.want {
					return
				}
			}
			t.Fatalf("duplicate complaints = %v, want one reading %q\n\n%s",
				said, c.want, c.src)
		})
	}
}

// TestTheErrorIdiomRebindsANameOnlyBesideANewOne pins the error idiom to the
// compiler's rule, in both directions.
//
// `let text, err = read(path);` binds a pair, and the idiom binds the next pair
// beside it: `let more, err = read(other);` repeats err and is accepted,
// because more is new. A let is refused only when nothing on its left is new
// to its scope -- refuseIfNothingIsNew, which is Go's rule for :=.
//
// This test used to be TestTheErrorIdiomMayRebindTheValueHalf, and it pinned an
// exception the compiler does not make: once the value half of a pair had been
// read, the editor let it be rebound alone -- `let abc, err = gets(); abc; let
// abc = 1;` -- and said nothing, while the build refuses that let, because abc
// is not new and nothing else on its left is (M26-LSP-029). Every program below
// is decided by compiling it.
func TestTheErrorIdiomRebindsANameOnlyBesideANewOne(t *testing.T) {
	// The idiom. With err unread in between, the second pair drew "duplicate
	// top-level declaration `err`", which is how the shipped examples came to
	// carry 116 warnings on programs that compile.
	const nextPair = `let abc, err = gets();
let def, err = gets();
putln(abc, def, err);
`
	// The same with the error checked in between, which is how the examples
	// write it.
	const nextPairAfterTheCheck = `let abc, err = gets();
if (err) { putln(err); }
let def, err = gets();
putln(abc, def, err);
`
	for _, src := range []string{nextPair, nextPairAfterTheCheck} {
		if message, refused := compilerComplaint(src); refused {
			t.Errorf("the compiler refuses the idiom (%q), so this is not the "+
				"program under test:\n\n%s", message, src)
			continue
		}
		if said := duplicateComplaints(src); len(said) != 0 {
			t.Errorf("the idiom was called a duplicate: %v\n\n%s", said, src)
		}
		if refusals := redeclarationRefusals(src); len(refusals) != 0 {
			t.Errorf("the idiom was refused: %v\n\n%s", refusals, src)
		}
	}

	// Rebinding the value half alone declares nothing new, whether or not the
	// first binding was read. The first two are the programs this test used to
	// excuse: the value read before it is rebound, and the error checked first,
	// which is the idiom's recommended order. (checkedFirst used to test `err
	// != null` and call puts, neither of which this language has, so the
	// compiler stopped there and never reached the let.)
	const consumed = `let abc, err = gets();
abc;
let abc = 1;
abc;
err;
`
	const checkedFirst = `let abc, err = gets();
if (err) { putln("bad"); }
abc;
let abc = 1;
abc;
`
	const notConsumed = `let abc, err = gets();
let abc = 1;
abc;
err;
`
	// A plain let, read on the next line and then declared again: the pair was
	// never what made the second let a refusal.
	const plainLet = `let x = 1;
x;
let x = 2;
x;
`
	for _, src := range []string{consumed, checkedFirst, notConsumed, plainLet} {
		message, refused := compilerRefusesADuplicate(src)
		if !refused {
			t.Errorf("the compiler does not refuse this as a redeclaration "+
				"(%q), so this is not the program under test:\n\n%s", message, src)
			continue
		}
		if refusals := redeclarationRefusals(src); !saysWhatTheBuildSays(refusals, message) {
			t.Errorf("the build refuses this with %q, and the editor's "+
				"refusals are %v\n\n%s", message, refusals, src)
		}
	}
}

// TestTheEditorRefusesALetExactlyWhenTheCompilerDoes holds the editor to the
// compiler's rule for a let, row by row and in both directions.
//
// The compiler judges a let by its statement, not by its names: it refuses one
// only when none of the names on its left is new to the scope it is written
// in, and a blank counts as new. The rule the editor had judged name by name,
// and was wrong both ways: it warned about the (value, err) idiom, which
// compiles, and excused a pair's value half rebound alone, which does not
// (M26-LSP-029).
//
// Every row is decided by compiling it, and the verdict is pinned beside it so
// that a change to the compiler's rule shows up here as a stale row rather
// than as two rules quietly agreeing on something new. A refused row must draw
// an Error from "mutant-declarations" in exactly the compiler's sentence, and
// no lint warning beside it; an accepted row must draw neither.
func TestTheEditorRefusesALetExactlyWhenTheCompilerDoes(t *testing.T) {
	for _, c := range []struct {
		name, src string
		refused   bool
	}{
		// Accepted: at least one name on the left is new, or a blank is.
		{
			name: "the (value, err) idiom with err unread between",
			src: `let a, err = gets();
let b, err = gets();
putln(a, b, err);
`,
		},
		{
			name: "a blank counts as new",
			src: `let a = 1;
let _, a = [1, 2];
putln(a);
`,
		},
		{
			name: "a parameter reused beside a new name",
			src: `let f = fn(a) {
    let err, a = [1, 2];
    return a + err;
};
putln(f(1));
`,
		},
		{
			name: "sibling blocks",
			src: `if (true) {
    let m = 1;
    putln(m);
}
if (true) {
    let m = 2;
    putln(m);
}
`,
		},
		{
			name: "a shadow in a block",
			src: `let x = 1;
if (true) {
    let x = 2;
    putln(x);
}
putln(x);
`,
		},
		// Refused: nothing on the left is new.
		{
			refused: true,
			name:    "a let declared twice",
			src: `let x = 1;
let x = 2;
putln(x);
`,
		},
		{
			refused: true,
			name:    "a consumed half of a pair rebound alone",
			src: `let abc, err = gets();
abc;
let abc = 1;
putln(abc, err);
`,
		},
		{
			refused: true,
			name:    "a pair rebound with nothing new",
			src: `let a, err = gets();
let a, err = gets();
putln(a, err);
`,
		},
		{
			refused: true,
			name:    "a parameter redeclared by its body",
			src: `let f = fn(a) {
    let a = 2;
    return a;
};
putln(f(1));
`,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			message, refused := compilerComplaint(c.src)
			if refused != c.refused {
				t.Fatalf("the compiler's verdict is refused=%v (%q) and the row "+
					"says %v, so the row is stale:\n\n%s", refused, message, c.refused, c.src)
			}
			refusals := redeclarationRefusals(c.src)
			said := duplicateComplaints(c.src)
			if !refused {
				if len(refusals) != 0 || len(said) != 0 {
					t.Fatalf("the compiler accepts this, and the editor says "+
						"%v %v\n\n%s", refusals, said, c.src)
				}
				return
			}
			if _, redeclaration := compilerRefusesADuplicate(c.src); !redeclaration {
				t.Fatalf("the compiler refuses this for another reason (%q), so "+
					"it is not a row of this table:\n\n%s", message, c.src)
			}
			if !saysWhatTheBuildSays(refusals, message) {
				t.Fatalf("the compiler refuses this with %q, and the editor's "+
					"refusals are %v\n\n%s", message, refusals, c.src)
			}
			if len(said) != 0 {
				t.Fatalf("the editor reports the refusal and a lint warning in "+
					"its own words as well: %v\n\n%s", said, c.src)
			}
		})
	}
}
