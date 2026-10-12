package vm

import (
	"strings"
	"testing"
)

// A refused comparison names the operator as the program spells it. It printed
// the opcode's number instead, so `1 < h["x"]` on a missing field failed as
// "unknown operator: 59 (INTEGER NULL)".
func TestARefusedComparisonNamesItsOperator(t *testing.T) {
	for _, c := range []struct{ input, want string }{
		{`let h = {}; 1 < h["x"];`, "unknown operator: INTEGER < NULL"},
		{`let h = {}; 1 >= h["x"];`, "unknown operator: INTEGER >= NULL"},
		{`"a" <= "b";`, "unknown operator: STRING <= STRING"},
		{`"a" > "b";`, "unknown operator: STRING > STRING"},
		{`let a, e = string_to_bytes("a", "raw"); let b, f = string_to_bytes("b", "raw"); a < b;`, "unknown operator: BYTES < BYTES"},
	} {
		_, err := runEncryptedVM(c.input)
		if err == nil {
			t.Errorf("%s was answered; want %q", c.input, c.want)
		} else if !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s failed with %q, want %q", c.input, err, c.want)
		}
	}
}
