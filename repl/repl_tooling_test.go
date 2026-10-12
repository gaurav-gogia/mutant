package repl

import (
	"bytes"
	"os"
	"os/user"
	"strings"
	"testing"

	"mutant/compiler"

	"github.com/chzyer/readline"
)

// runReplSession drives Start over a reader that is not os.Stdin and returns
// everything it printed.
//
// runReplLines in repl_seed_test.go splits the same output into one result per
// line entered, which is what a test of values wants. These tests are about
// what the REPL refuses and what it offers, so they read the whole transcript.
func runReplSession(t *testing.T, lines ...string) string {
	t.Helper()

	var out bytes.Buffer
	Start(strings.NewReader(strings.Join(lines, "\n")+"\n"), &out, "test", false, "")
	return out.String()
}

// The three spellings the old substring test refused: the word inside a string
// literal, the word as part of a longer identifier, and both in one session.
//
// The second assertion is the one that showed the cost. A refused line is never
// compiled, so the binding it was meant to make does not exist, and the next
// line that uses it reports an undefined variable -- a second, misleading error
// about a program that was correct.
func TestReplAcceptsTheWordQuoteOutsideAMacro(t *testing.T) {
	for _, line := range []string{
		`let q = "a quote";`,
		`let macroCount = 2;`,
		`let unquoted = "needs no unquote";`,
		`let quotation = "x";`,
	} {
		t.Run(line, func(t *testing.T) {
			text := runReplSession(t, line)
			if strings.Contains(text, macrosDisabledNotice) {
				t.Fatalf("%s was refused as macro use:\n%s", line, text)
			}
			if strings.Contains(text, "undefined variable") {
				t.Fatalf("%s did not bind its name:\n%s", line, text)
			}
		})
	}
}

func TestAQuoteInsideAStringLiteralStillBinds(t *testing.T) {
	text := runReplSession(t, `let q = "a quote";`, `q;`)
	if strings.Contains(text, macrosDisabledNotice) {
		t.Fatalf("the session was refused as macro use:\n%s", text)
	}
	if !strings.Contains(text, "a quote") {
		t.Fatalf("the bound string was not echoed back:\n%s", text)
	}
}

// The other half of the row: the refusal still has to happen. Each of these is
// a shape the macro machinery acts on -- a macro definition, a bare quote call,
// an unquote, and a macro call nested where ast.Modify reaches it only through
// a CallExpression argument.
func TestWithoutTheFlagMacroUseIsStillRefused(t *testing.T) {
	for _, line := range []string{
		`let m = macro(x) { quote(x) };`,
		`quote(1 + 2);`,
		`unquote(1);`,
		`putln(quote(1));`,
	} {
		t.Run(line, func(t *testing.T) {
			text := runReplSession(t, line)
			if !strings.Contains(text, macrosDisabledNotice) {
				t.Fatalf("%s was accepted without --enable-macros:\n%s", line, text)
			}
		})
	}
}

// A line that does not parse is reported as a parse error rather than as macro
// use, because the refusal is now decided from a tree and a line with no tree
// cannot be examined. The old substring test refused it before the parser saw
// it, which named the wrong problem.
func TestABrokenLineMentioningMacrosIsAParseError(t *testing.T) {
	text := runReplSession(t, `let m = macro(x) { quote(x) ;`)
	if strings.Contains(text, macrosDisabledNotice) {
		t.Fatalf("a line that does not parse was reported as macro use:\n%s", text)
	}
}

func TestReplStructDeclaredOnOneLineIsUsableOnTheNext(t *testing.T) {
	text := runReplSession(t,
		`struct Point { x; y; };`,
		`let p = Point { x: 1, y: 2 };`,
		`p.x;`,
	)
	if strings.Contains(text, "undefined struct type") {
		t.Fatalf("the struct did not survive the line it was declared on:\n%s", text)
	}
	if strings.Contains(text, "nil pointer dereference") {
		t.Fatalf("the session panicked after the failed line:\n%s", text)
	}
	if !strings.Contains(text, "1") {
		t.Fatalf("p.x was not echoed:\n%s", text)
	}
}

func TestReplEnumDeclaredOnOneLineIsUsableOnTheNext(t *testing.T) {
	text := runReplSession(t,
		`enum Colour { Red, Green };`,
		`let c = Colour.Red;`,
		`c;`,
	)
	for _, bad := range []string{"undefined enum", "undefined variable: Colour", "nil pointer dereference"} {
		if strings.Contains(text, bad) {
			t.Fatalf("the enum did not survive the line it was declared on (%q):\n%s", bad, text)
		}
	}
	// The value, not only the absence of an error: an assertion that nothing
	// went wrong passes just as well when nothing happened.
	if !strings.Contains(text, "Colour.Red") {
		t.Fatalf("the variant was not echoed:\n%s", text)
	}
}

// Completion offers the session's own names. The candidates come from the
// symbol table rather than from an object.Environment nothing writes to, so a
// name defined on an earlier line is a candidate on a later one.
func TestReplCompleteOffersSessionGlobals(t *testing.T) {
	text := runReplSession(t,
		`let myvariable = 3;`,
		`:complete myva`,
	)
	if strings.Contains(text, "No completion candidates.") {
		t.Fatalf("the session's own name was not offered:\n%s", text)
	}
	if !strings.Contains(text, "myvariable") {
		t.Fatalf("completion did not name myvariable:\n%s", text)
	}
}

func TestReplCompleteStillOffersBuiltins(t *testing.T) {
	text := runReplSession(t, `:complete putl`)
	if !strings.Contains(text, "putln") {
		t.Fatalf("completion stopped offering builtins:\n%s", text)
	}
}

// With stdin on a pipe there is no keyboard, so there must be no readline: a
// nil reader is what makes Start read the pipe with its scanner instead.
//
// This is the choice itself, which is the half a session driven through a
// strings.Reader cannot reach -- that path is already the scanner's, whatever
// the condition says. `go test` connects stdin to the null device, so the
// assertion holds here; the skip is for a runner that hands its own terminal
// through, where the right answer is the opposite one.
func TestNoLineReaderWhenStdinIsNotATerminal(t *testing.T) {
	if readline.IsTerminal(int(os.Stdin.Fd())) {
		t.Skip("stdin is a terminal in this runner, so the interactive path is the correct one")
	}

	reader := newInteractiveLineReader(os.Stdin, os.Stdout, compiler.NewSymbolTable())
	if reader != nil {
		reader.Close()
		t.Fatal("readline was selected for a stdin that is not a terminal; piped input would be read and discarded")
	}
}

func TestNoLineReaderForAnythingThatIsNotStdin(t *testing.T) {
	var out bytes.Buffer
	if reader := newInteractiveLineReader(strings.NewReader("1;\n"), &out, compiler.NewSymbolTable()); reader != nil {
		reader.Close()
		t.Fatal("readline was selected for a reader that is not os.Stdin")
	}
}

// welcome prints a greeting and must not be able to stop the REPL starting. It
// used to panic on a user.Current error, which on a container running a uid
// with no passwd entry is the ordinary outcome.
func TestWelcomeSurvivesAUserLookupFailure(t *testing.T) {
	restore := currentUser
	currentUser = func() (*user.User, error) { return nil, os.ErrNotExist }
	defer func() { currentUser = restore }()

	var out bytes.Buffer
	welcome(&out, "test", false)

	text := out.String()
	if !strings.Contains(text, "Welcome to mutant") {
		t.Fatalf("no greeting was printed:\n%s", text)
	}
	if strings.Contains(text, "Hello") {
		t.Fatalf("a nameless host was greeted by name:\n%s", text)
	}
}

// The same fallback that errors when $USER is unset returns Name: "" when it is
// set, so the host this row is about also reached welcome with no name at all.
func TestWelcomeFallsBackToTheUsernameWhenTheNameIsEmpty(t *testing.T) {
	restore := currentUser
	currentUser = func() (*user.User, error) {
		return &user.User{Uid: "12345", Username: "mew", Name: ""}, nil
	}
	defer func() { currentUser = restore }()

	var out bytes.Buffer
	welcome(&out, "test", false)

	text := out.String()
	if !strings.Contains(text, "Hello mew!") {
		t.Fatalf("the username was not used as the greeting name:\n%s", text)
	}
}

func TestWelcomeUsesTheFullNameWhenThereIsOne(t *testing.T) {
	restore := currentUser
	currentUser = func() (*user.User, error) {
		return &user.User{Uid: "1000", Username: "mew", Name: "Mew Two"}, nil
	}
	defer func() { currentUser = restore }()

	var out bytes.Buffer
	welcome(&out, "test", false)

	if !strings.Contains(out.String(), "Hello Mew Two!") {
		t.Fatalf("the full name was not used:\n%s", out.String())
	}
}

// A nil *user.User with a nil error is not a shape os/user produces, and it is
// the shape a greeting must not dereference: greetingName is the only caller
// and the panic it replaced was in this function.
func TestGreetingNameHandlesANilUser(t *testing.T) {
	restore := currentUser
	currentUser = func() (*user.User, error) { return nil, nil }
	defer func() { currentUser = restore }()

	if name := greetingName(); name != "" {
		t.Fatalf("greetingName() = %q for a nil user, want the empty string", name)
	}
}
