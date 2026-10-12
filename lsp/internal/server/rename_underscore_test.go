package server

// M26-LSP-021: rename refused every new name beginning with an underscore,
// although the lexer reads `_total` as an identifier and a leading underscore
// is how the language marks a top-level name private to its module.
// isValidIdentifierName asked unicode.IsLetter of the first rune where the
// lexer starts an identifier on a letter or '_', so the refactor the
// unused-private lint invites -- making an export private -- was the one
// rename that could not be made.
//
// Accepting the underscore and nothing more would have been worse than the
// refusal. Another module reads a value as `alias.name`, and the compiler
// refuses `alias._name`, so the rename would have written a program that does
// not build into every importer. What builds is decided here by compiling, in
// both directions: the rename is refused while an importer reads the value,
// and made when none does, or when the name is a type's, which crosses a
// module boundary bare and is not filtered by the underscore rule.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mutant/compiler"
	"mutant/generator"
	"mutant/lexer"
	"mutant/parser"
	"mutant/token"

	lsp "github.com/tliron/glsp/protocol_3_16"
)

// requestRename asks for a rename at pos the way a client does.
func requestRename(s *Server, uri lsp.DocumentUri, pos lsp.Position, newName string) (*lsp.WorkspaceEdit, error) {
	return s.rename(nil, &lsp.RenameParams{
		TextDocumentPositionParams: lsp.TextDocumentPositionParams{
			TextDocument: lsp.TextDocumentIdentifier{URI: uri},
			Position:     pos,
		},
		NewName: newName,
	})
}

// buildRefusal compiles the program whose entry file is entry the way the
// build does, with the import graph walked and linked, and returns what it was
// refused with, or nil when it builds.
func buildRefusal(entry string) error {
	_, err, _, _ := generator.CompileForTest(entry, nil)
	return err
}

// singleFileRefusal is buildRefusal for a program held in one string.
func singleFileRefusal(src string) error {
	p := parser.New(lexer.New(src))
	program := p.ParseProgram()
	if errs := p.Errors(); len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return compiler.New().Compile(program)
}

// The row's repro and its neighbours: an underscore followed by a letter, by
// another underscore and by a digit. The lexer reads each as an identifier and
// the compiler builds each, so rename has no business refusing any of them.
func TestRenameAcceptsANewNameWithALeadingUnderscore(t *testing.T) {
	const text = "let total = 1;\nputln(total);\n"
	for _, newName := range []string{"_total", "__", "_1", "_Total2"} {
		t.Run(newName, func(t *testing.T) {
			s := New(false)
			initializeServer(t, s)
			const uri = lsp.DocumentUri("file:///rename-underscore.mut")
			openDocument(t, s, uri, text)

			edit, err := requestRename(s, uri, lsp.Position{Line: 1, Character: 7}, newName)
			if err != nil {
				t.Fatalf("rename to %q refused: %v", newName, err)
			}
			if edit == nil || len(edit.Changes[uri]) != 2 {
				t.Fatalf("rename to %q gave %+v, want 2 edits: the declaration and the use", newName, edit)
			}
			renamed := applyTextEdits(t, text, edit.Changes[uri])
			if want := strings.ReplaceAll(text, "total", newName); renamed != want {
				t.Fatalf("renamed text = %q, want %q", renamed, want)
			}
			if err := singleFileRefusal(renamed); err != nil {
				t.Fatalf("the renamed program %q does not build: %v", renamed, err)
			}
		})
	}
}

// The one underscore name rename still refuses. `_` alone is the discard, which
// the one-declaration rule exempts, so nothing would catch a binding renamed to
// it being rebound by the next `let _, err = f();` -- the build accepts that,
// as the second half shows, and the rename is the only thing that can refuse.
func TestRenameRefusesTheBareDiscard(t *testing.T) {
	s := New(false)
	initializeServer(t, s)
	const uri = lsp.DocumentUri("file:///rename-discard.mut")
	openDocument(t, s, uri, "let total = 1;\nputln(total);\n")

	if edit, err := requestRename(s, uri, lsp.Position{Line: 1, Character: 7}, "_"); err == nil {
		t.Fatalf("rename to _ was accepted: %+v", edit)
	}

	if err := singleFileRefusal("let _ = 1;\nlet _ = 2;\nputln(_);\n"); err != nil {
		t.Fatalf("the build now refuses a second declaration of _ (%v), so the reason rename refuses _ has changed", err)
	}
}

// isValidIdentifierName held to the lexer over every one-, two- and three-rune
// name drawn from a set that straddles each test the lexer makes: a letter
// outside ASCII, a digit outside ASCII, a number that is not a digit, a
// letter-number that is not a letter, the underscore, and runes no identifier
// holds. A name is valid when it lexes as one IDENT token spanning all of it,
// with one intended difference: the bare discard. Before the fix 72 names
// disagreed, every one of them beginning with an underscore.
func TestRenameNamesAreExactlyTheLexersIdentifiers(t *testing.T) {
	// The four runes after the digits are e-acute, the Arabic-Indic digit
	// three, superscript two and the Roman numeral twelve, as UTF-8 bytes.
	charset := []rune("_aZr09\xc3\xa9\xd9\xa3\xc2\xb2\xe2\x85\xab$- \"")
	names := []string{"let", "fn", "match", "import", "null", "true"}
	for _, a := range charset {
		names = append(names, string(a))
		for _, b := range charset {
			names = append(names, string([]rune{a, b}))
			for _, c := range charset {
				names = append(names, string([]rune{a, b, c}))
			}
		}
	}

	lexesAsOneIdentifier := func(name string) bool {
		l := lexer.New(name)
		first := l.NextToken()
		second := l.NextToken()
		return first.Type == token.IDENT && first.Literal == name && second.Type == token.EOF
	}

	disagreements := 0
	for _, name := range names {
		want := lexesAsOneIdentifier(name) && name != "_"
		if got := isValidIdentifierName(name); got != want {
			disagreements++
			if disagreements <= 8 {
				t.Errorf("isValidIdentifierName(%q) = %t, want %t", name, got, want)
			}
		}
	}
	if disagreements > 8 {
		t.Errorf("%d of %d names disagree with the lexer in all", disagreements, len(names))
	}
}

// What accepting the underscore alone would have broken. main.mut reads total
// as stats.total, so the rename would write stats._total there, which the build
// refuses. The rename refuses instead, and for that reason rather than by
// calling _total an invalid name, which it is not.
func TestRenameToAPrivateNameIsRefusedWhileAnotherModuleReadsIt(t *testing.T) {
	files := map[string]string{
		"lib/stats.mut": "let total = fn(xs) { return 1; };\n",
		"main.mut":      "import stats \"lib/stats.mut\";\nputln(stats.total([1]));\n",
	}
	root := writeModules(t, files)
	if err := buildRefusal(filepath.Join(root, "main.mut")); err != nil {
		t.Fatalf("the fixture does not build before the rename: %v", err)
	}
	renamedTree := writeModules(t, map[string]string{
		"lib/stats.mut": strings.ReplaceAll(files["lib/stats.mut"], "total", "_total"),
		"main.mut":      strings.ReplaceAll(files["main.mut"], "total", "_total"),
	})
	buildSaid := buildRefusal(filepath.Join(renamedTree, "main.mut"))
	if buildSaid == nil {
		t.Fatal("the build accepts stats._total from another module, so there is nothing for the rename to refuse")
	}

	s, uri := serverOver(t, root, "lib/stats.mut")
	edit, err := requestRename(s, uri, lsp.Position{Line: 0, Character: 4}, "_total")
	if err == nil {
		written := 0
		if edit != nil {
			written = len(edit.Changes[pathToURI(filepath.Join(root, "main.mut"))])
		}
		t.Fatalf("rename to _total went through with %d edit(s) in main.mut, writing stats._total, which the build refuses: %v",
			written, buildSaid)
	}
	if strings.Contains(err.Error(), "invalid identifier name") {
		t.Fatalf("refused for the wrong reason: %v (_total is an identifier)", err)
	}
	const rule = "visible only inside the module that declares it"
	if !strings.Contains(err.Error(), rule) || !strings.Contains(buildSaid.Error(), rule) {
		t.Errorf("the rename and the build do not name the same rule:\n rename: %v\n build:  %v", err, buildSaid)
	}
}

// The refactor the unused-private lint asks for, and the other half of the
// refusal above: with no importer reading total, the rename goes through,
// touches nothing outside stats.mut, and the program it leaves builds.
func TestRenameToAPrivateNameIsMadeWhenNoOtherModuleReadsIt(t *testing.T) {
	files := map[string]string{
		"lib/stats.mut": "let total = fn(xs) { return 1; };\nlet mean = fn(xs) { return total(xs); };\n",
		"main.mut":      "import stats \"lib/stats.mut\";\nputln(stats.mean([1]));\n",
	}
	root := writeModules(t, files)
	s, uri := serverOver(t, root, "lib/stats.mut")

	edit, err := requestRename(s, uri, lsp.Position{Line: 0, Character: 4}, "_total")
	if err != nil {
		t.Fatalf("rename to _total refused: %v", err)
	}
	if edit == nil {
		t.Fatal("rename to _total produced no edit")
	}
	if got := len(edit.Changes[uri]); got != 2 {
		t.Fatalf("stats.mut got %d edits, want 2: the declaration and the call in mean", got)
	}
	if got := len(edit.Changes[pathToURI(filepath.Join(root, "main.mut"))]); got != 0 {
		t.Fatalf("main.mut got %d edits, want 0: it reads mean, not total", got)
	}

	renamed := applyTextEdits(t, files["lib/stats.mut"], edit.Changes[uri])
	if err := os.WriteFile(filepath.Join(root, "lib", "stats.mut"), []byte(renamed), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := buildRefusal(filepath.Join(root, "main.mut")); err != nil {
		t.Fatalf("the renamed program does not build: %v\nstats.mut is now %q", err, renamed)
	}
}

// The refusal is kept to values. A struct name crosses a module boundary bare
// and never reaches the symbol table the underscore rule guards, so `_Point`
// declared in one module builds in another; refusing this rename would be
// refusing a program that builds.
func TestRenameToAPrivateNameIsMadeForATypeAnotherModuleNamesBare(t *testing.T) {
	files := map[string]string{
		"lib/shapes.mut": "struct Point { x, y };\n",
		"main.mut":       "import shapes \"lib/shapes.mut\";\nlet p = Point{x: 1, y: 2};\nputln(p.x);\n",
	}
	root := writeModules(t, files)
	s, uri := serverOver(t, root, "lib/shapes.mut")

	edit, err := requestRename(s, uri, lsp.Position{Line: 0, Character: 7}, "_Point")
	if err != nil {
		t.Fatalf("rename of the struct to _Point refused: %v", err)
	}
	if edit == nil {
		t.Fatal("rename of the struct to _Point produced no edit")
	}
	for rel, want := range map[string]int{"lib/shapes.mut": 1, "main.mut": 1} {
		path := filepath.Join(root, rel)
		edits := edit.Changes[pathToURI(path)]
		if len(edits) != want {
			t.Fatalf("%s got %d edits, want %d", rel, len(edits), want)
		}
		if err := os.WriteFile(path, []byte(applyTextEdits(t, files[rel], edits)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := buildRefusal(filepath.Join(root, "main.mut")); err != nil {
		t.Fatalf("the renamed program does not build: %v", err)
	}
}
