package code

import (
	"testing"
)

func TestMake(t *testing.T) {
	tests := []struct {
		Op       Opcode
		operands []int
		expected []byte
	}{
		{OpConstant, []int{65534}, []byte{byte(OpConstant), 255, 254}},
	}

	for _, tt := range tests {
		inst := Make(tt.Op, tt.operands...)
		if len(inst) != len(tt.expected) {
			t.Errorf("instruction and expected lengths don't match, want = %d, got = %d", len(tt.expected), len(inst))
		}

		for i, b := range tt.expected {
			if inst[i] != b {
				t.Errorf("wrong byte at position %d, wanted = %d, got  = %d", i, b, inst[i])
			}
		}
	}
}
