package runner

// A program that ends on a function prints which function it is, the same way
// on every run and at every mutation level. It printed Closure[0x...] instead,
// a heap address that changed from run to run, so the sweep classed
// examples/modules/lib/stats.mut and report.mut "output varies between runs"
// and could never give them a golden (M26-EVL-002).

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mutant/generator"
)

// runFileCapturingStdoutEVL002 compiles src the way `mutant gen` does, at the
// given mutation level, runs the .mu the way `mutant prog.mu` does, and returns
// what the run wrote to standard output.
func runFileCapturingStdoutEVL002(t *testing.T, src string, level int) string {
	t.Helper()
	const password = "a password for the test's own artifact"
	dst := filepath.Join(t.TempDir(), "prog")
	if err, _, _ := generator.Generate(src, dst, "", "", false, password, level, 1, nil, nil); err != nil {
		t.Fatalf("%s did not compile at level %d: %v", src, level, err)
	}
	data, err := os.ReadFile(dst + ".mu")
	if err != nil {
		t.Fatal(err)
	}
	bytecode, err := decode(data, password)
	if err != nil {
		t.Fatalf("%s did not decode: %v", src, err)
	}

	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	saved := os.Stdout
	os.Stdout = writer
	captured := make(chan string, 1)
	go func() {
		out, _ := io.ReadAll(reader)
		captured <- string(out)
	}()

	runErr, _ := runvm(bytecode, password, false)
	os.Stdout = saved
	writer.Close()
	out := <-captured
	reader.Close()

	if runErr != nil {
		t.Fatalf("%s at level %d: run failed: %v", src, level, runErr)
	}
	return out
}

func TestAFunctionValuePrintsItsNameNotAnAddressEVL002(t *testing.T) {
	dir := t.TempDir()
	for _, c := range []struct {
		name   string
		source string
		want   string
	}{
		{"named", "let mean = fn(values) { return values[0]; };\nmean\n", "fn mean(values)\n"},
		{"anonymous", "fn(a, b) { a + b }\n", "fn(a, b)\n"},
		{"returned closure", "let make = fn(x) { fn(y) { x + y } };\nmake(1)\n", "fn(y)\n"},
		// The same variable read twice is two closures on the VM.
		{"read twice", "let f = fn(n) { n };\nputln(f);\nf\n", "fn f(n)\nfn f(n)\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			src := filepath.Join(dir, c.name+".mut")
			if err := os.WriteFile(src, []byte(c.source), 0o600); err != nil {
				t.Fatal(err)
			}
			for _, level := range []int{0, 5} {
				// Two runs of each build: an address differed between them.
				for run := 1; run <= 2; run++ {
					if got := runFileCapturingStdoutEVL002(t, src, level); got != c.want {
						t.Errorf("level %d run %d printed %q, want %q", level, run, got, c.want)
					}
				}
			}
		})
	}
}

// The two library examples print the function they end on. A sweep run with
// --golden checks the same thing through the CLI against the .golden beside
// each one; this checks it without a binary, and checks that the golden, when
// it is on disk, records the same line.
func TestModuleLibraryExamplesPrintTheirLastFunctionEVL002(t *testing.T) {
	for _, c := range []struct{ name, want string }{
		{"stats", "fn largest(values)\n"},
		{"report", "fn line(name, values)\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			lib := filepath.Join("..", "examples", "modules", "lib")
			for _, level := range []int{0, 5} {
				if got := runFileCapturingStdoutEVL002(t, filepath.Join(lib, c.name+".mut"), level); got != c.want {
					t.Errorf("level %d printed %q, want %q", level, got, c.want)
				}
			}
			golden, err := os.ReadFile(filepath.Join(lib, c.name+".golden"))
			if err != nil {
				return
			}
			if got := strings.ReplaceAll(string(golden), "\r\n", "\n"); got != c.want {
				t.Errorf("%s.golden records %q, want %q", c.name, got, c.want)
			}
		})
	}
}
