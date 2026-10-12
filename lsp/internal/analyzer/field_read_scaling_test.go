package analyzer

import (
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	mast "mutant/ast"
)

// A field read must not cost a walk of the whole file (M26-SEM-004). Four
// shapes, because four different per-read costs made Diagnostics quadratic:
//
//   - parameter receivers, `r.size` where r is a parameter: the graph build
//     asked structHeldBy once per read, and structTypeNameForDeclaration walked
//     every statement looking for a let that declares r, and found none. Then
//     StructFieldDiagnostics asked again, once per read, for the same answer.
//   - struct literal receivers, `p.x` where p is a struct literal's binding:
//     the same two walks, which succeed.
//   - unbound receivers, `str.upper`: nothing binds the receiver, so the field
//     rule's lookup fell back to a scan of every field name in the file, once
//     per read, for an answer it could never use.
//   - a struct declared late: structFieldNames scanned the top level from the
//     start, once per read.

func fieldReadsOnParameters(n int) string {
	var sb strings.Builder
	for i := 0; i < n; i++ {
		sb.WriteString("let f" + strconv.Itoa(i) + " = fn(r) { return r.size + r.offset; };\n")
	}
	return sb.String()
}

func fieldReadsOnStructBindings(n int) string {
	var sb strings.Builder
	sb.WriteString("struct P { x; y; };\n")
	for i := 0; i < n; i++ {
		k := strconv.Itoa(i)
		sb.WriteString("let p" + k + " = P{x: 1, y: 2};\nlet q" + k + " = p" + k + ".x + p" + k + ".y;\n")
	}
	return sb.String()
}

func fieldReadsOnUnboundReceivers(n int) string {
	var sb strings.Builder
	for i := 0; i < n; i++ {
		sb.WriteString("let s" + strconv.Itoa(i) + " = [str.upper, str.lower, str.trim, str.upper, str.lower, str.trim, str.upper, str.lower];\n")
	}
	return sb.String()
}

func fieldReadsOnALateStruct(n int) string {
	var sb strings.Builder
	for i := 0; i < n; i++ {
		sb.WriteString("let z" + strconv.Itoa(i) + " = " + strconv.Itoa(i) + ";\n")
	}
	sb.WriteString("struct P { x; y; };\nlet p = P{x: 1, y: 2};\n")
	for i := 0; i < n; i++ {
		sb.WriteString("let q" + strconv.Itoa(i) + " = [p.x, p.y, p.x, p.y, p.x, p.y, p.x, p.y];\n")
	}
	return sb.String()
}

// TestFieldReadsDoNotCostTheSquareOfTheFile is wall-clock on a shared machine.
// It times what M26-SEM-004 is about and nothing else: building the name graph
// and running the field rule, which is where the four per-read walks were, and
// not the rest of Diagnostics. Each run analyses a fresh snapshot, because the
// graph and the struct index are built once per snapshot and a second call
// would time a lookup. The fastest of three runs is what is compared, so that
// one run slowed by other work on the box cannot fail the test, and a run
// inside the budget ends it early, since no later run could change the verdict.
//
// Measured both ways on 2026-10-11 through go test -overlay, in several
// sittings on a machine that was running other work, so every figure here
// moved with that load and none of them is a bound. Over more than sixty runs
// of each shape, the fixed code took 50-159 ms for parameter receivers,
// 87-253 ms for struct literal receivers, 75-191 ms for unbound receivers and
// 121-325 ms for a struct declared late, the upper ends with other packages'
// tests running beside it. The 3 s budget is therefore about nine times the
// slowest run seen and over twenty times the slowest shape's quietest one. The
// code before the fix took 5.3 s to 50 s a run on the same fixtures. At the
// fastest of three runs, which is what the test compares, it took 7.4-9.5 s,
// 7.8-23 s, 20-25 s and 5.3-7.5 s for the four shapes in the order above:
// under twice the budget for the struct declared late in its quietest sitting,
// and over six times it for the unbound receivers in theirs. One shape over
// the budget fails the test, so it is the unbound receivers that keep it
// failing on the old code with room to spare.
//
// The budget is absolute, as in
// TestManyHolesOnOneLineDoNotCostTheSquareOfTheirNumber, rather than a ratio
// between two sizes: Diagnostics on the fix's prototype measured x2.5 from 2000
// to 4000 parameter-receiver functions on this machine (36 ms to 91 ms), so a
// ratio tight enough to tell linear from quadratic sits inside the noise of the
// fixed code itself.
func TestFieldReadsDoNotCostTheSquareOfTheFile(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
	}{
		{"parameter receivers", fieldReadsOnParameters(6000)},
		{"struct literal receivers", fieldReadsOnStructBindings(8000)},
		{"unbound receivers", fieldReadsOnUnboundReceivers(5000)},
		{"a struct declared late", fieldReadsOnALateStruct(6000)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const budget = 3 * time.Second
			const runs = 3
			var fastest time.Duration
			for run := 1; run <= runs; run++ {
				snapshot := New().Analyze(tc.src)
				if len(snapshot.ParseErrors) != 0 {
					t.Fatalf("the fixture does not parse: %v", snapshot.ParseErrors[0])
				}
				// What an earlier run left behind is collected here, outside
				// the timed section, so that no run pays for another's garbage.
				runtime.GC()
				began := time.Now()
				snapshot.Graph()
				snapshot.StructFieldDiagnostics()
				elapsed := time.Since(began)
				t.Logf("run %d over %d bytes: %s", run, len(tc.src), elapsed)
				if run == 1 || elapsed < fastest {
					fastest = elapsed
				}
				if fastest <= budget {
					return
				}
			}
			t.Errorf("building the graph and running the field rule over %d bytes took %s at the fastest of %d runs, over the %s budget: a field read is walking the whole file again",
				len(tc.src), fastest, runs, budget)
		})
	}
}

// TestStructQuestionsAreAnsweredFromOneWalk is the deterministic half. After the
// first answer the program's statements are removed: a walk per question then
// finds nothing, and an index built on first use still answers.
func TestStructQuestionsAreAnsweredFromOneWalk(t *testing.T) {
	s := New().Analyze("struct P { x; y; };\nlet p = P{x: 1, y: 2};\np.x;\n")
	let, ok := s.Program.Statements[1].(*mast.LetStatement)
	if !ok || let.Name == nil {
		t.Fatalf("statement 1 is %T, want the let", s.Program.Statements[1])
	}
	if name, ok := s.structTypeNameForDeclaration(let.Name); !ok || name != "P" {
		t.Fatalf("structTypeNameForDeclaration(p) = %q, %v; want P", name, ok)
	}
	if names, ok := s.structFieldNames("P"); !ok || strings.Join(names, ",") != "x,y" {
		t.Fatalf("structFieldNames(P) = %v, %v; want [x y]", names, ok)
	}

	s.Program.Statements = nil
	if name, ok := s.structTypeNameForDeclaration(let.Name); !ok || name != "P" {
		t.Errorf("structTypeNameForDeclaration walked the program again: got %q, %v after the statements were removed", name, ok)
	}
	if names, ok := s.structFieldNames("P"); !ok || strings.Join(names, ",") != "x,y" {
		t.Errorf("structFieldNames scanned the program again: got %v, %v after the statements were removed", names, ok)
	}
}
