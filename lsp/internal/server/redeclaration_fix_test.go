package server

import (
	"testing"

	lsp "github.com/tliron/glsp/protocol_3_16"
)

// A let the compiler refuses, because its scope already holds every name it
// binds, drew a lint warning, and at the top level the warning's fix was
// "Remove duplicate top-level declaration", which deleted the later line. The
// program then compiled and quietly went on with the FIRST value -- the one the
// author had just written a second let to replace (M26-LSP-029). The
// compiler's own sentence names the remedy, "to change this one write x = ...
// instead", and the fix is now that edit: the let keyword goes, up to the name,
// and the statement assigns. A let binding several names gets no fix, because
// the language has no assignment to several names at once.

const changeLetToAssignment = "Change the let to an assignment"

func TestARefusedRedeclarationIsFixedByChangingTheLetToAnAssignment(t *testing.T) {
	for _, tc := range []struct {
		name string
		text string
		want string
	}{
		{
			name: "at the top level",
			text: "let x = 1;\nlet x = 2;\nputln(x);\n",
			want: "let x = 1;\nx = 2;\nputln(x);\n",
		},
		{
			name: "indented in a function body",
			text: "let f = fn() {\n    let y = 1;\n    let y = 2;\n    return y;\n};\nputln(f());\n",
			want: "let f = fn() {\n    let y = 1;\n    y = 2;\n    return y;\n};\nputln(f());\n",
		},
		{
			// The edit's columns are UTF-16 units, and a character outside the
			// basic multilingual plane earlier on the line is two of them.
			name: "after a character outside the basic multilingual plane on its line",
			text: "let s = \"\xf0\x9f\x98\x80\"; let s = 2;\nputln(s);\n",
			want: "let s = \"\xf0\x9f\x98\x80\"; s = 2;\nputln(s);\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := New(false)
			initializeServer(t, s)
			uri := lsp.DocumentUri("file:///redeclaration-fix.mut")
			refusal := redeclarationRefusal(t, openAndReadDiagnostics(t, s, uri, tc.text), tc.text)

			actions := quickFixesOffered(t, s, uri, refusal)
			if len(actions) != 1 || actions[0].Title != changeLetToAssignment {
				t.Fatalf("offered %q, want exactly one fix, %q", actionTitles(actions), changeLetToAssignment)
			}
			if actions[0].Edit == nil {
				t.Fatal("the fix carries no edit")
			}
			after := applyLikeAConformingClient(t, tc.text, actions[0].Edit.Changes[uri])
			if after != tc.want {
				t.Fatalf("the fix left %q, want %q", after, tc.want)
			}
			if message, refused := compilerRefusalOf(after); refused {
				t.Fatalf("the compiler refuses the fixed program: %s", message)
			}
		})
	}
}

// TestAGroupedRefusalIsOfferedNoFix: the second `let a, err = gets();`
// declares nothing new and is refused, and there is no assignment to two names
// to turn it into -- nor is deleting it a fix, since that drops the call.
func TestAGroupedRefusalIsOfferedNoFix(t *testing.T) {
	const text = "let a, err = gets();\nlet a, err = gets();\n"
	s := New(false)
	initializeServer(t, s)
	uri := lsp.DocumentUri("file:///redeclaration-grouped.mut")
	refusal := redeclarationRefusal(t, openAndReadDiagnostics(t, s, uri, text), text)

	if actions := quickFixesOffered(t, s, uri, refusal); len(actions) != 0 {
		t.Fatalf("offered %q for a let binding two names, want no fix", actionTitles(actions))
	}
}

// TestARedeclarationFixDeclinesWhenThePositionNoLongerHoldsTheName covers the
// request that arrives late: the diagnostic the client sends back was measured
// in older text, and where it points there is no longer a name.
func TestARedeclarationFixDeclinesWhenThePositionNoLongerHoldsTheName(t *testing.T) {
	const published = "let x = 1;\nlet x = 2;\nputln(x);\n"
	for _, tc := range []struct {
		name string
		now  string
	}{
		{name: "blank where the name stood", now: "let x = 1;\nlet  x = 2;\nputln(x);\n"},
		{name: "the middle of another name", now: "let x = 1;\nputln(x);\n"},
		// A name in the same place, but not the one the refusal is about:
		// offering the edit here left `y = 2;`, an assignment to a name
		// nothing declares.
		{name: "another name where the name stood", now: "let x = 1;\nlet y = 2;\nputln(x);\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := New(false)
			initializeServer(t, s)
			uri := lsp.DocumentUri("file:///redeclaration-stale.mut")
			stale := redeclarationRefusal(t, openAndReadDiagnostics(t, s, uri, published), published)
			if _, offered := actionTitled(quickFixesOffered(t, s, uri, stale), changeLetToAssignment); !offered {
				t.Fatal("the fix is not offered against the text it was published for, so declining below would prove nothing")
			}

			replaceWholeDocument(t, s, uri, 2, tc.now)
			if action, offered := actionTitled(quickFixesOffered(t, s, uri, stale), changeLetToAssignment); offered {
				t.Fatalf("the fix was offered against %q and leaves %q",
					tc.now, applyLikeAConformingClient(t, tc.now, action.Edit.Changes[uri]))
			}
		})
	}
}
