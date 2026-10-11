package analyzer

import (
	"strings"
	"testing"
)

// argTypeMessages returns the messages of every builtinArgType diagnostic in
// src. The rule's messages are the only lint messages that read "must be".
func argTypeMessages(t *testing.T, src string) []string {
	t.Helper()
	return argTypeMessagesWithConfig(t, src, DefaultLintConfig())
}

func argTypeMessagesWithConfig(t *testing.T, src string, config LintConfig) []string {
	t.Helper()
	snapshot := New().Analyze(src)
	out := make([]string, 0, 2)
	for _, d := range Diagnostics(snapshot, config) {
		if d.Source != nil && *d.Source == "mutant-lint" && strings.Contains(d.Message, "must be") {
			out = append(out, d.Message)
		}
	}
	return out
}

func TestBuiltinArgTypeFiresOnWrongKind(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{"integer for a string parameter", "str_upper(42);\n", "argument 1 to `str_upper` must be STRING, got INTEGER"},
		{"boolean for a union parameter", "len(true);\n", "argument 1 to `len` must be STRING, BYTES, ARRAY, or HASH, got BOOLEAN"},
		{"string for an array parameter", "str_join(\"abc\", \",\");\n", "argument 1 to `str_join` must be ARRAY, got STRING"},
		{"hash for an array parameter", "sort({\"a\": 1});\n", "argument 1 to `sort` must be ARRAY, got HASH"},
		{"wrong kind in a later position", "str_repeat(\"ab\", \"3\");\n", "argument 2 to `str_repeat` must be INTEGER, got STRING"},
		{"string for a numeric union", "abs(\"1\");\n", "argument 1 to `abs` must be INTEGER or FLOAT, got STRING"},
		{"wrong kind in a variadic tail", "min(1, \"2\");\n", "argument 2 to `min` must be INTEGER or FLOAT, got STRING"},
		{"a bound name carries its type", "let n = 42;\nstr_upper(n);\n", "argument 1 to `str_upper` must be STRING, got INTEGER"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			msgs := argTypeMessages(t, tc.src)
			found := false
			for _, m := range msgs {
				if m == tc.want {
					found = true
				}
			}
			if !found {
				t.Fatalf("expected %q for %q, got %+v", tc.want, tc.src, msgs)
			}
		})
	}
}

func TestBuiltinArgTypeSilentOnCorrectCalls(t *testing.T) {
	sources := []string{
		"str_upper(\"abc\");\n",
		"len(\"abc\");\n",
		"len([1, 2]);\n",
		"len({\"k\": 1});\n",
		"str_join([\"a\", \"b\"], \",\");\n",
		"abs(-1);\n",
		"abs(-1.5);\n",
		"sqrt(2);\n", // INTEGER is accepted where the runtime takes either number
		"min(1, 2.5, 3);\n",
		"to_string(true);\n",
		"map([1, 2], fn(x) { x });\n",
	}
	for _, src := range sources {
		if msgs := argTypeMessages(t, src); len(msgs) != 0 {
			t.Fatalf("expected no builtinArgType diagnostics for %q, got %+v", src, msgs)
		}
	}
}

// TestBuiltinArgTypeSilentWhereNothingIsDeclared covers the two ways a
// parameter opts out of checking, and the case where the builtin documents no
// parameters at all.
func TestBuiltinArgTypeSilentWhereNothingIsDeclared(t *testing.T) {
	sources := []string{
		"putln(42, true, [1], {\"k\": 1});\n", // ...values is verified ParamAny
		"type_of(42);\n",                      // ParamAny
		"to_string([1, 2]);\n",                // ParamAny
		"contains([1], true);\n",              // the value compared is ParamAny
		"get({\"k\": 1}, \"k\", false);\n",    // the default is ParamAny
		"security_diagnostics();\n",           // no parameters at all
	}
	for _, src := range sources {
		if msgs := argTypeMessages(t, src); len(msgs) != 0 {
			t.Fatalf("expected no builtinArgType diagnostics for %q, got %+v", src, msgs)
		}
	}
}

func TestBuiltinArgTypeSilentWhenShadowed(t *testing.T) {
	sources := []string{
		"let len = fn(x) { 1 };\nlen(true);\n",
		"let f = fn(str_upper) { str_upper(42) };\n",
	}
	for _, src := range sources {
		if msgs := argTypeMessages(t, src); len(msgs) != 0 {
			t.Fatalf("expected no diagnostic for a shadowed builtin in %q, got %+v", src, msgs)
		}
	}
}

// TestBuiltinArgTypeSilentOnUncertainArguments is the rule's false-positive
// guard: inference is best-effort, so anything whose type is merely inferred —
// rather than read off a literal or a never-reassigned binding — goes
// unchecked.
func TestBuiltinArgTypeSilentOnUncertainArguments(t *testing.T) {
	sources := []string{
		// A name that is assigned to elsewhere may hold a different kind by
		// the time it reaches the call, which the forward walk cannot see.
		"let v = 42;\nv = \"text\";\nstr_upper(v);\n",
		"let v = 42;\nstr_upper(v);\nv = \"text\";\n",
		// Call results, index and field accesses, and arithmetic are inferred
		// rather than certain.
		"str_upper(gets());\n",
		"let a = [1, 2];\nstr_upper(a[0]);\n",
		// A function parameter has no known type at all.
		"let f = fn(x) { str_upper(x) };\n",
	}
	for _, src := range sources {
		if msgs := argTypeMessages(t, src); len(msgs) != 0 {
			t.Fatalf("expected no diagnostic for an uncertain argument in %q, got %+v", src, msgs)
		}
	}
}

// TestBuiltinArgTypeSuppressedByWrongArity pins the one-diagnostic-per-call
// rule: a call the arity check rejects is not also type-checked, because its
// arguments cannot be mapped onto parameters.
func TestBuiltinArgTypeSuppressedByWrongArity(t *testing.T) {
	if msgs := argTypeMessages(t, "len(true, false);\n"); len(msgs) != 0 {
		t.Fatalf("a wrong-arity call must not be type-checked, got %+v", msgs)
	}

	// Even with the arity rule switched off, the type check still declines the
	// call — the count is wrong regardless of who reports it.
	config := DefaultLintConfig()
	config.BuiltinArity = LintSeverityOff
	if msgs := argTypeMessagesWithConfig(t, "len(true, false);\n", config); len(msgs) != 0 {
		t.Fatalf("a wrong-arity call must not be type-checked with builtinArity off, got %+v", msgs)
	}
}

func TestBuiltinArgTypeRespectsOffSeverity(t *testing.T) {
	config := DefaultLintConfig()
	config.BuiltinArgType = LintSeverityOff
	if msgs := argTypeMessagesWithConfig(t, "str_upper(42);\n", config); len(msgs) != 0 {
		t.Fatalf("builtinArgType=off should suppress the diagnostic, got %+v", msgs)
	}

	// Switching the type rule off must leave the arity rule working.
	if msgs := arityMessages(t, "abs(1, 2);\n"); len(msgs) == 0 {
		t.Fatal("builtinArity must still fire when builtinArgType is off")
	}
}

// TestBuiltinArgTypeAnchorsOnTheArgument checks the diagnostic points at the
// offending argument rather than at the callee, so the editor underlines the
// value the author has to change.
func TestBuiltinArgTypeAnchorsOnTheArgument(t *testing.T) {
	snapshot := New().Analyze("str_repeat(\"ab\", \"3\");\n")
	for _, d := range Diagnostics(snapshot, DefaultLintConfig()) {
		if !strings.Contains(d.Message, "must be INTEGER") {
			continue
		}
		if d.Range.Start.Line != 0 {
			t.Fatalf("diagnostic line = %d, want 0", d.Range.Start.Line)
		}
		// `str_repeat("ab", ` is 17 characters, so the second argument starts
		// at character 17.
		if d.Range.Start.Character != 17 {
			t.Fatalf("diagnostic starts at character %d, want the second argument at 17", d.Range.Start.Character)
		}
		return
	}
	t.Fatal("expected a builtinArgType diagnostic for str_repeat")
}

// TestBuiltinArgTypeCatchesStructsAndEnums covers what the two new kinds bought.
//
// STRUCT and ENUM_VALUE did not exist in the kind vocabulary until the last
// fifteen undeclared parameter positions needed them, and while they were
// missing paramKindForType declined both — so a struct handed to a builtin that
// cannot serialise one was simply never judged.
func TestBuiltinArgTypeCatchesStructsAndEnums(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			// builtin/json.go objectToJSONValue handles scalars, null, arrays
			// and hashes, and errors on anything else — structs included.
			name: "a struct where JSON expects a serialisable value",
			src:  "struct Point { x; };\nlet p = Point{x: 1};\njson_stringify(p);\n",
			want: "argument 1 to `json_stringify` must be STRING, BYTES, INTEGER, FLOAT, BOOLEAN, NULL, ARRAY, or HASH, got STRUCT",
		},
		{
			// builtin/db.go dbNodeTypeFromObject: "must be INTEGER or ENUM_VALUE".
			name: "a string where a node type expects an integer or enum",
			src:  "let t = \"data\";\ndb_add_node(1, t);\n",
			want: "argument 2 to `db_add_node` must be INTEGER or ENUM_VALUE, got STRING",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := argTypeMessages(t, tc.src)
			for _, message := range got {
				if message == tc.want {
					return
				}
			}
			t.Errorf("diagnostics = %q, want one of them to be %q", got, tc.want)
		})
	}
}

// TestBuiltinArgTypeAcceptsStructsWhereTheyAreTaken is the other half. The
// positions that gained STRUCT accept one, and must stay silent on it — the
// whole reason they could not be declared before is that declaring only HASH
// would have flagged the struct calls that work.
func TestBuiltinArgTypeAcceptsStructsWhereTheyAreTaken(t *testing.T) {
	cases := []string{
		// mitm_http.go: "must be HASH or STRUCT".
		"struct Req { method; };\nlet r = Req{method: \"GET\"};\nhttp_build_request(r);\n",
		// http.go httpBodyString / httpHeaderMap take either container.
		"struct Body { a; };\nlet b = Body{a: 1};\nhttp_post(\"http://x\", b);\n",
		// secure_net.go requireOptionsArg accepts a struct as an options bag.
		"struct Opts { days; };\nlet o = Opts{days: 30};\ntls_generate_ca(o);\n",
		// An enum variant is what db_add_node's type argument is for.
		"enum NodeKind { Data, File };\ndb_add_node(1, NodeKind.File);\n",
	}

	for _, src := range cases {
		if got := argTypeMessages(t, src); len(got) > 0 {
			t.Errorf("flagged a call that works:\n%s\ndiagnostics: %q", src, got)
		}
	}
}

// TestBuiltinArgTypeIgnoresWhatTheSolverDeduced pins that nothing fn_solver.go
// deduced about a parameter reaches the argument-type rule. Inference used to
// seed the solver's kinds into each function body, and they rode out on aliases
// and return types into names the rule trusts (M26-SEM-003). Every program here
// runs; the comment on each says what it prints.
func TestBuiltinArgTypeIgnoresWhatTheSolverDeduced(t *testing.T) {
	for _, c := range []struct {
		name string
		src  string
	}{
		{
			// Prints `solver: ABC`: the guarded branch returns the string.
			name: "through the return type, past a type guard",
			src: `let low_byte = fn(v) {
    if (type_of(v) == "STRING") { return v; }
    return v & 255;
};
let s = low_byte("abc");
putf("solver: %v\n", str_upper(s));
`,
		},
		{
			// Prints `alias: ABC`: v is "abc", so w is too.
			name: "through an alias in the body",
			src: `let low_byte = fn(v) {
    if (type_of(v) == "STRING") { let w = v; return str_upper(w); }
    return v & 255;
};
putf("alias: %v\n", low_byte("abc"));
`,
		},
		{
			// Prints `abs: 5`: str_upper(5) returns an error value, and the
			// function carries on and returns 5.
			name: "on a straight line, with no guard at all",
			src: `let f = fn(a) {
    let shout = str_upper(a);
    return a;
};
let r = f(5);
putf("abs: %v\n", abs(r));
`,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			if msgs := argTypeMessages(t, c.src); len(msgs) != 0 {
				t.Errorf("a deduction about a parameter reached the diagnostic: %v\n%s", msgs, c.src)
			}
		})
	}
}

// TestBuiltinArgTypeStillChecksWhatIsCertain is the control for the test above:
// a name bound from a literal, or from a builtin's fixed pair contract, is
// still checked.
func TestBuiltinArgTypeStillChecksWhatIsCertain(t *testing.T) {
	if msgs := argTypeMessages(t, "let s = 42;\nstr_upper(s);\n"); len(msgs) != 1 {
		t.Errorf("a literal-bound name must still be checked, got %v", msgs)
	}
	if msgs := argTypeMessages(t, "let data, err = fs_read_bytes(\"disk.img\");\nstr_upper(data);\n"); len(msgs) != 1 {
		t.Errorf("a builtin-pair-bound name must still be checked, got %v", msgs)
	}
}

// TestBuiltinArgTypeKnowsAFunctionsOwnName pins that inside a function literal
// its own name is the literal, as the compiler's DefineFunctionName makes it,
// whatever an outer binding of that name holds. Inference used to see the outer
// `let walk = 0` there and called map's second argument an INTEGER
// (M26-SEM-020). The program prints [] and then 0.
func TestBuiltinArgTypeKnowsAFunctionsOwnName(t *testing.T) {
	src := `let walk = 0;
if (true) {
    let walk = fn(xs) {
        if (len(xs) == 0) { return []; }
        return map(xs, walk);
    };
    putf("%v\n", walk([]));
}
putf("%v\n", walk);
`
	if msgs := argTypeMessages(t, src); len(msgs) != 0 {
		t.Errorf("a self-named function shadowing an outer binding was flagged: %v", msgs)
	}
}
