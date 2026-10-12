package object

import (
	"strings"
	"testing"
)

// A function value renders as the name and parameters its source gave it, and
// never as an address. Closure.Inspect was fmt.Sprintf("Closure[%p]", c), so a
// program whose last value was a function printed a pointer that differed on
// every run, and on the VM between two reads of one variable, since storage
// rebuilds the closure on every load (M26-EVL-002).
func TestFunctionInspectNamesTheFunctionEVL002(t *testing.T) {
	for _, tt := range []struct {
		name string
		fn   *CompiledFunction
		want string
	}{
		{"bound to a name", &CompiledFunction{Name: "mean", NumParams: 1, Params: []string{"values"}}, "fn mean(values)"},
		{"several parameters", &CompiledFunction{Name: "line", NumParams: 2, Params: []string{"name", "values"}}, "fn line(name, values)"},
		{"no parameters", &CompiledFunction{Name: "tick"}, "fn tick()"},
		{"anonymous", &CompiledFunction{NumParams: 2, Params: []string{"a", "b"}}, "fn(a, b)"},
		{"anonymous, no parameters", &CompiledFunction{}, "fn()"},
		// A release build strips Name and Params; only the count is left.
		{"stripped, one parameter", &CompiledFunction{NumParams: 1}, "fn(<1 parameter>)"},
		{"stripped, three parameters", &CompiledFunction{NumParams: 3}, "fn(<3 parameters>)"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.fn.Inspect(); got != tt.want {
				t.Errorf("CompiledFunction.Inspect() = %q, want %q", got, tt.want)
			}

			// Two closures over one function are what the VM makes of one
			// variable read twice. Each must render the function, and so
			// render alike.
			first := &Closure{Fn: tt.fn}
			second := &Closure{Fn: tt.fn}
			if got := first.Inspect(); got != tt.want {
				t.Errorf("Closure.Inspect() = %q, want %q", got, tt.want)
			}
			if a, b := first.Inspect(), second.Inspect(); a != b {
				t.Errorf("two closures over one function render differently: %q and %q", a, b)
			}
		})
	}
}

// The nil receivers render without a panic and without an address.
func TestFunctionInspectNilReceiversEVL002(t *testing.T) {
	var cl *Closure
	var fn *CompiledFunction
	for _, got := range []string{cl.Inspect(), fn.Inspect(), (&Closure{}).Inspect()} {
		if got != "fn()" {
			t.Errorf("nil function rendered %q, want %q", got, "fn()")
		}
		if strings.Contains(got, "0x") {
			t.Errorf("nil function rendered an address: %q", got)
		}
	}
}
