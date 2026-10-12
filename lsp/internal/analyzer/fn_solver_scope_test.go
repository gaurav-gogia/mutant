package analyzer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mutant/lexer"
	mutantparser "mutant/parser"
	"mutant/sema"

	lsp "github.com/tliron/glsp/protocol_3_16"
)

// A user function's card says one thing about each parameter: how the body uses
// it, "used as INTEGER", or the kinds its callers were seen to pass. That display
// was chosen on the premise that every sentence on it is true, and it was not,
// because the solver attributed a use to whatever parameter or function had the
// same name anywhere in the file. Where a case says what its program printed,
// the program was run, and the card it got contradicted the run. The solver
// resolves names through the graph now, the way the compiler does.
//
// Every case asserts both halves where it has both: the sentence the run
// contradicts is gone, and the one it bears out is there. A card that said
// nothing at all would pass the first half on its own.

// solverCards renders the card of every function the snapshot binds, in the
// order functionBindings collects them.
func solverCards(t *testing.T, s *Snapshot) []string {
	t.Helper()
	if len(s.ParseErrors) > 0 {
		t.Fatalf("fixture did not parse: %v", s.ParseErrors)
	}
	var cards []string
	for _, binding := range functionBindings(s.Program) {
		cards = append(cards, userFunctionCard(s, binding.name, binding.literal, "").render())
	}
	return cards
}

// solverCardClaim is one card and what it must and must not say.
type solverCardClaim struct {
	name string
	src  string
	// card indexes solverCards.
	card int
	// wrong is what the run contradicts, and right is what it bears out.
	wrong, right []string
}

func checkSolverCardClaims(t *testing.T, claims []solverCardClaim) {
	t.Helper()
	for _, claim := range claims {
		t.Run(claim.name, func(t *testing.T) {
			cards := solverCards(t, New().Analyze(claim.src))
			if claim.card >= len(cards) {
				t.Fatalf("the fixture binds %d functions, so there is no card %d", len(cards), claim.card)
			}
			checkSolverCard(t, cards[claim.card], claim.wrong, claim.right)
		})
	}
}

func checkSolverCard(t *testing.T, card string, wrong, right []string) {
	t.Helper()
	for _, sentence := range wrong {
		if strings.Contains(card, sentence) {
			t.Errorf("the card says %q, which the program's run contradicts:\n%s", sentence, card)
		}
	}
	for _, sentence := range right {
		if !strings.Contains(card, sentence) {
			t.Errorf("the card does not say %q:\n%s", sentence, card)
		}
	}
}

// A use narrows a parameter only when the name resolves to that parameter's own
// declaration. A call is a call of a file function only when its callee
// resolves to that function's let, and of a builtin only when nothing the file
// binds there has the name. These are the shapes where the name and the
// declaration came apart.
func TestAUseNarrowsOnlyTheParameterItResolvesTo(t *testing.T) {
	sameNamedHelpers := `let a = fn(v) {
    let helper = fn(x) { return str_upper(x); };
    return helper(v);
};
let b = fn(w) {
    let helper = fn(y) { return y * 2; };
    return helper(w);
};
putln(a("hi"), b(21));
`
	checkSolverCardClaims(t, []solverCardClaim{
		{
			// Prints GO, then 5: the x upper-cased is the block's own, and the
			// parameter x is returned as it was passed.
			name: "a block-local let shadows the parameter",
			src: `let f = fn(x, label) {
    if (len(label) > 0) { let x = label; putln(str_upper(x)); }
    return x;
};
putln(f(5, "go"));
`,
			card:  0,
			wrong: []string{"- `x` _(used as"},
			right: []string{"- `x` · `INTEGER` _(observed at call sites)_", "- `label` _(used as STRING|BYTES|ARRAY|HASH)_"},
		},
		{
			// Prints [seed, 6]: the loop's x is each number, and the
			// parameter x is still the string.
			name: "a for-in binding shadows the parameter",
			src: `let tally = fn(x, xs) {
    let total = 0;
    for (x in xs) { total = total + x * 2; }
    return [x, total];
};
putln(tally("seed", [1, 2]));
`,
			card:  0,
			wrong: []string{"- `x` _(used as"},
			right: []string{"- `x` · `STRING` _(observed at call sites)_"},
		},
		{
			// Prints 42: v goes to the function apply was handed, not to the
			// file's shout.
			name: "a parameter named like a file function",
			src: `let shout = fn(s) { return str_upper(s); };
let apply = fn(shout, v) { return shout(v); };
putln(apply(fn(n) { return n + 1; }, 41));
`,
			card:  1,
			wrong: []string{"- `v` _(used as"},
			right: []string{"- `shout` _(used as FUNCTION)_", "- `v` · `INTEGER` _(observed at call sites)_"},
		},
		{
			// Prints HI 42: a's helper upper-cases v, and b's doubles w.
			name:  "two nested helpers with one name, the first",
			src:   sameNamedHelpers,
			card:  0,
			wrong: []string{"- `v` _(used as INTEGER|FLOAT)_"},
			right: []string{"- `v` _(used as STRING)_"},
		},
		{
			name:  "two nested helpers with one name, the second",
			src:   sameNamedHelpers,
			card:  2,
			wrong: []string{"- `w` _(used as STRING)_"},
			right: []string{"- `w` _(used as INTEGER|FLOAT)_"},
		},
		{
			// Prints 5: str.upper is the Box's field, not str_upper.
			name: "a struct value named like a builtin family",
			src: `struct Box { upper }
let str = Box { upper: fn(v) { return v; } };
let shout = fn(p) { return str.upper(p); };
putln(shout(5));
`,
			card:  0,
			wrong: []string{"- `p` _(used as"},
			right: []string{"- `p` · `INTEGER` _(observed at call sites)_"},
		},
		{
			// Prints 5 beside a str.mut whose upper returns its argument:
			// str.upper is that module's function, not str_upper. The alias
			// binds `str` whether or not the module has been read.
			name:  "an import named like a builtin family",
			src:   solverImportsStr,
			card:  0,
			wrong: []string{"- `p` _(used as"},
			right: []string{"- `p` · `INTEGER` _(observed at call sites)_"},
		},
	})
}

const solverImportsStr = `import "str.mut";

let shout = fn(p) {
    return str.upper(p);
};
putln(shout(5));
`

// The import case again, as the editor sees it inside a workspace that has read
// the module, so the alias names a real file rather than an unread one.
func TestAnImportedModuleNamedLikeABuiltinFamilyIsNotTheFamily(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"main.mut": solverImportsStr,
		"str.mut":  "let upper = fn(v) {\n    return v;\n};\n",
	}
	workspace := sema.NewWorkspace(nil)
	for name, src := range files {
		path := filepath.Join(root, name)
		if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
		parser := mutantparser.New(lexer.New(src))
		program := parser.ParseProgram()
		if errs := parser.Errors(); len(errs) > 0 {
			t.Fatalf("%s did not parse: %v", name, errs)
		}
		workspace.PutFile("file:///"+filepath.ToSlash(path), path, program)
	}

	s := New().AnalyzeInWorkspace(filepath.Join(root, "main.mut"), solverImportsStr, workspace)
	resolved := false
	for _, node := range s.Graph().TopLevel() {
		if node.Kind == sema.KindNamespace && node.Name == "str" && node.Target != "" {
			resolved = true
		}
	}
	if !resolved {
		t.Fatal("the workspace did not resolve `import \"str.mut\"`, so this case is not the one it means to be")
	}

	cards := solverCards(t, s)
	if len(cards) != 1 {
		t.Fatalf("main.mut binds one function, got %d cards", len(cards))
	}
	checkSolverCard(t, cards[0],
		[]string{"- `p` _(used as"},
		[]string{"- `p` · `INTEGER` _(observed at call sites)_"})
}

// The nested helpers M26-SEM-008 gave a card through the real hover path, and
// what that card said about them.
func TestANestedHelpersCardDescribesThatHelper(t *testing.T) {
	// Prints s 1: a's helper is only ever called with "s", and b's with 1.
	pooledHelpers := `let a = fn() {
    let helper = fn(x) { return x; };
    return helper("s");
};
let b = fn() {
    let helper = fn(y) { return y; };
    return helper(1);
};
putln(a(), b());
`
	cases := []struct {
		name, src, at string
		wrong, right  []string
	}{
		{
			name:  "call sites are observed on the helper they call",
			src:   pooledHelpers,
			at:    "helper(1)",
			wrong: []string{"`STRING|INTEGER` _(observed at call sites)_"},
			right: []string{"function `helper(y)`", "- `y` · `INTEGER` _(observed at call sites)_"},
		},
		{
			name:  "and the other helper keeps its own",
			src:   pooledHelpers,
			at:    "helper(\"s\")",
			right: []string{"function `helper(x)`", "- `x` · `STRING` _(observed at call sites)_"},
		},
		{
			// Prints 42 A: show's v goes to main's own fmt, n * 2. The file's
			// fmt is declared after main, where nothing in main can see it.
			name: "a nested callee is the nested function, not a later top-level one",
			src: `let main = fn() {
    let fmt = fn(n) { return n * 2; };
    let show = fn(v) { return fmt(v); };
    return show(21);
};
let fmt = fn(s) { return str_upper(s); };
putln(main(), fmt("a"));
`,
			at:    "show(21)",
			wrong: []string{"- `v` _(used as STRING)_"},
			right: []string{"function `show(v)`", "- `v` _(used as INTEGER|FLOAT)_"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := New().Analyze(c.src)
			if len(s.ParseErrors) > 0 {
				t.Fatalf("fixture did not parse: %v", s.ParseErrors)
			}
			card, _, ok := s.HoverText(solverPositionOf(t, c.src, c.at))
			if !ok {
				t.Fatalf("no hover on %q", c.at)
			}
			checkSolverCard(t, card, c.wrong, c.right)
		})
	}
}

// solverPositionOf is the position of the first occurrence of needle in src.
func solverPositionOf(t *testing.T, src, needle string) lsp.Position {
	t.Helper()
	at := strings.Index(src, needle)
	if at < 0 {
		t.Fatalf("%q is not in the fixture", needle)
	}
	line := strings.Count(src[:at], "\n")
	column := at - (strings.LastIndex(src[:at], "\n") + 1)
	return lsp.Position{Line: lsp.UInteger(line), Character: lsp.UInteger(column)}
}

// Every kind the lattice holds has a word, and a kind added to it later has to
// get one too: a kind with no word is not rendered as unknown, it is left out of
// the sentence.
func TestEveryKindTheSolverCanNarrowToHasAName(t *testing.T) {
	names := ksAny.names()
	if len(names) != kindSetWidth {
		t.Fatalf("the lattice holds %d kinds and names() renders %d of them: %v", kindSetWidth, len(names), names)
	}
	seen := map[string]bool{}
	for _, name := range names {
		if seen[name] {
			t.Fatalf("names() renders %q twice: %v", name, names)
		}
		seen[name] = true
	}
}

// BYTES had no word, so a contract that takes a buffer lost it from the card,
// and the card named only the kinds that were left.
func TestACardSaysWhenAParameterIsUsedAsABuffer(t *testing.T) {
	checkSolverCardClaims(t, []solverCardClaim{
		{
			// Prints the SHA-256 of "abc": hash_sha256 takes STRING|BYTES.
			name: "a contract that takes a string or a buffer",
			src: `let digest = fn(payload, algo) { return hash_sha256(payload); };
let raw, err = string_to_bytes("abc", "raw");
putln(digest(raw, "sha256"));
`,
			card:  0,
			wrong: []string{"- `payload` _(used as STRING)_"},
			right: []string{"- `payload` _(used as STRING|BYTES)_"},
		},
		{
			// Prints 3: len takes STRING|BYTES|ARRAY|HASH.
			name: "len",
			src: `let size = fn(b) { return len(b); };
let raw, err = string_to_bytes("abc", "raw");
putln(size(raw));
`,
			card:  0,
			wrong: []string{"- `b` _(used as STRING|ARRAY|HASH)_"},
			right: []string{"- `b` _(used as STRING|BYTES|ARRAY|HASH)_"},
		},
		{
			// join of two buffers prints BYTES 4: `+` concatenates them.
			name:  "addition",
			src:   "let join = fn(a, b) { return a + b; };\n",
			card:  0,
			wrong: []string{"- `a` _(used as STRING|INTEGER|FLOAT)_"},
			right: []string{"- `a` _(used as STRING|BYTES|INTEGER|FLOAT)_", "- `b` _(used as STRING|BYTES|INTEGER|FLOAT)_"},
		},
		{
			// record_release takes a buffer and nothing else, which rendered
			// as no words at all.
			name:  "a contract that takes only a buffer",
			src:   "let release = fn(buf, why) { return record_release(buf, why); };\n",
			card:  0,
			right: []string{"- `buf` _(used as BYTES)_", "- `why` _(used as STRING)_"},
		},
	})
}

// A parameter the program assigns is not narrowed at all: after the assignment
// its uses describe the value it was given there, not anything a caller passed.
func TestAnAssignedParameterIsNotNarrowed(t *testing.T) {
	checkSolverCardClaims(t, []solverCardClaim{
		{
			// Prints 5: shout is handed an integer and works.
			name: "assigned in the body",
			src: `let shout = fn(x) {
    x = to_string(x);
    return str_upper(x);
};
putln(shout(5));
`,
			card:  0,
			wrong: []string{"- `x` _(used as"},
			right: []string{"- `x` · `INTEGER` _(observed at call sites)_"},
		},
		{
			// Prints 6: the closure shares x with triple, so its assignment
			// is what x * 3 reads.
			name: "assigned in a nested closure",
			src: `let triple = fn(x) {
    let reset = fn() { x = 2; };
    reset();
    return x * 3;
};
putln(triple("a"));
`,
			card:  0,
			wrong: []string{"- `x` _(used as"},
			right: []string{"- `x` · `STRING` _(observed at call sites)_"},
		},
		{
			// Prints 2. A write through an index changes what xs holds, not
			// which value xs is, so the use after it is still a use of the
			// array the caller passed.
			name: "written through an index, which is not an assignment of it",
			src: `let first = fn(xs) {
    xs[0] = 1;
    return len(xs);
};
putln(first([5, 6]));
`,
			card:  0,
			right: []string{"- `xs` _(used as STRING|BYTES|ARRAY|HASH)_"},
		},
	})
}

// A function whose name is assigned is no call target: its let's literal no
// longer says what a call through the name runs.
func TestAnAssignedFunctionIsNoCallTarget(t *testing.T) {
	// Prints 42: by the time g runs, f doubles.
	reassigned := `let f = fn(x) { return str_upper(x); };
f = fn(y) { return y * 2; };
let g = fn(v) { return f(v); };
putln(g(21));
`
	checkSolverCardClaims(t, []solverCardClaim{
		{
			name:  "its constraints do not reach a caller",
			src:   reassigned,
			card:  1,
			wrong: []string{"- `v` _(used as"},
			right: []string{"- `v` · `INTEGER` _(observed at call sites)_"},
		},
		{
			// The literal's own card still describes its own body.
			name:  "its own card is unchanged",
			src:   reassigned,
			card:  0,
			right: []string{"- `x` _(used as STRING)_"},
		},
		{
			// Prints 42: f(21) runs the doubling function, so the literal
			// that returns x unchanged was never passed 21.
			name: "a call through it is not observed on it",
			src: `let f = fn(x) { return x; };
f = fn(y) { return y * 2; };
putln(f(21));
`,
			card:  0,
			wrong: []string{"`INTEGER` _(observed at call sites)_"},
			right: []string{"**Parameters**\n- `x`\n\n**Returns**"},
		},
	})
}

// A function is named inside its own body: the compiler binds the name of the
// let that binds the literal there, so a call through that name is a call of
// the literal itself. The graph binds it there only for a let of one name, and
// read the call in a let that binds a function beside an error name as a call
// of an earlier function of that name, or of the builtin of that name, and the
// card held the function's parameters to what that other callee takes.
func TestAFunctionsOwnNameInsideItIsThatFunction(t *testing.T) {
	// Prints 5: up(5, 2) runs the second up, which calls itself until n is 0
	// and returns the 5 it was given. Nothing calls the first up.
	earlierUp := `let up = fn(s) { return str_upper(s); };
let up, err = fn(x, n) {
    if (n > 0) { return up(x, n - 1); }
    return x;
};
putln(up(5, 2));
`
	// Prints s: the second up's call of itself passes it "s", and nothing
	// calls the first up.
	observedUp := `let up = fn(s) { return s; };
let up, err = fn(x, n) {
    if (n > 0) { return up("s", n - 1); }
    return x;
};
putln(up(5, 2));
`
	// Prints 5: the again nested in the second up calls up, which there is
	// the second up and not the first.
	nestedCaller := `let up = fn(s) { return str_upper(s); };
let up, err = fn(x, n) {
    let again = fn(y, m) { return up(y, m); };
    if (n > 0) { return again(x, n - 1); }
    return x;
};
putln(up(5, 2));
`
	checkSolverCardClaims(t, []solverCardClaim{
		{
			// Prints 5: the str_starts_with inside is the function itself,
			// which returns the integer it was handed.
			name: "a call of its own name, which a builtin also has",
			src: `let str_starts_with, err = fn(x, n) {
    if (n > 0) { return str_starts_with(x, n - 1); }
    return x;
};
putln(str_starts_with(5, 2));
`,
			card:  0,
			wrong: []string{"- `x` _(used as"},
			right: []string{"- `x` · `INTEGER` _(observed at call sites)_", "- `n` _(used as INTEGER|FLOAT)_"},
		},
		{
			// Prints 5.
			name: "a call of its own name, which a builtin of one argument also has",
			src: `let depth = 0;
let str_upper, err = fn(x) {
    depth = depth + 1;
    if (depth < 3) { return str_upper(x); }
    return x;
};
putln(str_upper(5));
`,
			card:  0,
			wrong: []string{"- `x` _(used as"},
			right: []string{"- `x` · `INTEGER` _(observed at call sites)_"},
		},
		{
			name:  "a call of its own name, which an earlier function also has",
			src:   earlierUp,
			card:  1,
			wrong: []string{"- `x` _(used as"},
			right: []string{"- `x` · `INTEGER` _(observed at call sites)_", "- `n` _(used as INTEGER|FLOAT)_"},
		},
		{
			name:  "what the call passes is observed on the function itself",
			src:   observedUp,
			card:  1,
			right: []string{"- `x` · `STRING|INTEGER` _(observed at call sites)_"},
		},
		{
			name:  "and not on the earlier function",
			src:   observedUp,
			card:  0,
			wrong: []string{"_(observed at call sites)_"},
			right: []string{"**Parameters**\n- `s`\n\n**Returns**"},
		},
		{
			name:  "a function nested in it calls it by that name",
			src:   nestedCaller,
			card:  1,
			wrong: []string{"- `x` _(used as"},
			right: []string{"- `x` · `INTEGER` _(observed at call sites)_"},
		},
		{
			// again hands m to up's n, which the second up compares and
			// subtracts from; y goes to up's x, which it only returns.
			name:  "and the nested caller's own card",
			src:   nestedCaller,
			card:  2,
			wrong: []string{"- `y` _(used as"},
			right: []string{"- `m` _(used as INTEGER|FLOAT)_"},
		},
		{
			// Prints 5 A. The up nested in other is a different declaration
			// of the name, written later; the first up's call of up is a call
			// of itself.
			name: "a later function of that name elsewhere",
			src: `let up, err = fn(x, n) {
    if (n > 0) { return up(x, n - 1); }
    return x;
};
let other = fn() {
    let up = fn(s) { return str_upper(s); };
    return up("a");
};
putln(up(5, 2), other());
`,
			card:  0,
			wrong: []string{"- `x` _(used as"},
			right: []string{"- `x` · `INTEGER` _(observed at call sites)_"},
		},
		{
			// Prints 5, and str("ab", 1) stops with `cannot access field on
			// non-struct: CLOSURE`: str.upper reads a field of the function
			// itself, and str_upper is never called.
			name: "a dotted call through its own name",
			src: `let str, err = fn(x, k) {
    if (k > 0) { return str.upper(x); }
    return x;
};
putln(str(5, 0));
`,
			card:  0,
			wrong: []string{"- `x` _(used as"},
			right: []string{"- `x` · `INTEGER` _(observed at call sites)_", "- `k` _(used as INTEGER|FLOAT)_"},
		},
		{
			// Prints A. A let in the body ahead of the call is nearer than the
			// function's own name, in the compiler as in the graph, so the
			// call is of the inner shout, which upper-cases what it is given.
			name: "a let inside it that takes the name",
			src: `let shout, err = fn(x) {
    let shout = fn(s) { return str_upper(s); };
    return shout(x);
};
putln(shout("a"));
`,
			card:  0,
			right: []string{"- `x` _(used as STRING)_"},
		},
		{
			// Prints 42: a parameter of the function's own name is nearer
			// still, and call(v) calls the function it was handed.
			name: "a parameter that takes the name",
			src: `let call, err = fn(call, v) { return call(v); };
putln(call(fn(n) { return n + 1; }, 41));
`,
			card:  0,
			right: []string{"- `call` _(used as FUNCTION)_", "- `v` · `INTEGER` _(observed at call sites)_"},
		},
	})
}

// The same functions through the real hover path, on the call that runs them.
// Every name of a multi-name let is a value (TestEveryNameOfAMultiNameLetIsAValue),
// so the call shows its name as one, as it did before these lets were looked
// at, and none of the card's lines -- false or true -- reaches the user there.
// The card itself is held to the run in TestAFunctionsOwnNameInsideItIsThatFunction.
func TestAHoverOnAFunctionBoundBesideAnErrorIsTrue(t *testing.T) {
	cases := []struct {
		name, src, at string
		wrong, right  []string
	}{
		{
			// Prints 5.
			name: "a call of its own name, which an earlier function also has",
			src: `let up = fn(s) { return str_upper(s); };
let up, err = fn(x, n) {
    if (n > 0) { return up(x, n - 1); }
    return x;
};
putln(up(5, 2));
`,
			at:    "up(5, 2)",
			wrong: []string{"- `x` _(used as", "function `up(s)"},
			right: []string{"identifier `up`"},
		},
		{
			// Prints 5.
			name: "a call of its own name, which a builtin also has",
			src: `let depth = 0;
let str_upper, err = fn(x) {
    depth = depth + 1;
    if (depth < 3) { return str_upper(x); }
    return x;
};
putln(str_upper(5));
`,
			at:    "str_upper(5)",
			wrong: []string{"- `x` _(used as", "builtin `str_upper"},
			right: []string{"identifier `str_upper`"},
		},
		{
			// Prints 5.
			name: "a function nested in it calls it by that name",
			src: `let up = fn(s) { return str_upper(s); };
let up, err = fn(x, n) {
    let again = fn(y, m) { return up(y, m); };
    if (n > 0) { return again(x, n - 1); }
    return x;
};
putln(up(5, 2));
`,
			at:    "up(5, 2)",
			wrong: []string{"- `x` _(used as", "function `up(s)"},
			right: []string{"identifier `up`"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := New().Analyze(c.src)
			if len(s.ParseErrors) > 0 {
				t.Fatalf("fixture did not parse: %v", s.ParseErrors)
			}
			card, _, ok := s.HoverText(solverPositionOf(t, c.src, c.at))
			if !ok {
				t.Fatalf("no hover on %q", c.at)
			}
			checkSolverCard(t, card, c.wrong, c.right)
		})
	}
}

// TestBuildingTheGraphNeverBuildsTheSolver pins the rule solvedFunctions rests
// on. The solver builds the graph from inside solvedOnce, and the graph's build
// reaches the struct index and the inference pass through structHeldBy; if
// either of those ever asked for the solver, solvedOnce would wait on itself and
// the first card hovered would hang the language server.
func TestBuildingTheGraphNeverBuildsTheSolver(t *testing.T) {
	// p is built by a call rather than a struct literal, so asking which
	// struct it holds -- which the graph does for p.x -- runs inference.
	src := `struct Point { x }
let make = fn(v) { return Point { x: v }; };
let p = make(1);
let read = fn(q) { return str_upper(q); };
putln(p.x, read("a"));
`
	s := New().Analyze(src)
	if len(s.ParseErrors) > 0 {
		t.Fatalf("fixture did not parse: %v", s.ParseErrors)
	}
	// Broken, the rule is a hang rather than a wrong answer, so the build runs
	// where a deadline can report it instead of stalling the package.
	built := make(chan bool, 1)
	go func() {
		s.Graph()
		inferred := s.typeMap != nil
		s.types()
		s.buildStructIndex()
		built <- inferred
	}()
	var inferred bool
	select {
	case inferred = <-built:
	case <-time.After(time.Minute):
		t.Fatal("building the graph or running inference did not return: something on that path asked for the solver, which waits on the graph")
	}
	if !inferred {
		t.Fatal("building the graph did not run inference, so this fixture no longer exercises the path the rule is about")
	}
	if s.solvedFns != nil {
		t.Fatal("building the graph or running inference built the solver; the solver builds the graph inside solvedOnce, so a card would deadlock")
	}

	// And the solver, asked first, builds its graph and returns. It has to
	// build one: resolving names through the graph is the whole of the fix
	// this ordering rule is the price of.
	fresh := New().Analyze(src)
	done := make(chan int, 1)
	go func() { done <- len(fresh.solvedFunctions()) }()
	select {
	case n := <-done:
		if n != 2 {
			t.Fatalf("the solver found %d functions, want make and read", n)
		}
	case <-time.After(time.Minute):
		t.Fatal("solvedFunctions did not return: building the graph inside solvedOnce waited on solvedOnce")
	}
	if fresh.graph == nil {
		t.Fatal("the solver answered without building the graph, so it is not resolving names the way the compiler does")
	}
}
