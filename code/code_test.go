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
		{OpAdd, []int{}, []byte{byte(OpAdd)}},
		{OpGetLocal, []int{255}, []byte{byte(OpGetLocal), 255}},
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

func TestInstructionsString(t *testing.T) {
	instructions := []Instructions{
		Make(OpAdd),
		Make(OpGetLocal, 1),
		Make(OpConstant, 2),
		Make(OpConstant, 65535),
	}

	expected := "0000 OpAdd\n0001 OpGetLocal 1\n0003 OpConstant 2\n0006 OpConstant 65535\n"
	concatted := Instructions{}

	for _, ins := range instructions {
		concatted = append(concatted, ins...)
	}

	if concatted.String() != expected {
		t.Errorf("instructions wrongly formatted.\nwant=%q\ngot=%q", expected, concatted.String())
	}
}

func TestReadOperands(t *testing.T) {
	tests := []struct {
		op        Opcode
		operands  []int
		bytesRead int
	}{
		{OpConstant, []int{65535}, 2},
		{OpGetLocal, []int{255}, 1},
	}

	for _, tt := range tests {
		instruction := Make(tt.op, tt.operands...)
		def, err := Lookup(byte(tt.op))
		if err != nil {
			t.Fatalf("definition not found: %q\n", err)
		}

		operandsRead, n := ReadOperands(def, instruction[1:])
		if n != tt.bytesRead {
			t.Fatalf("n wrong. want = %d, got = %d", tt.bytesRead, n)
		}

		for i, want := range tt.operands {
			if operandsRead[i] != want {
				t.Errorf("operand wrong. want = %d, got = %d", want, operandsRead[i])
			}
		}
	}
}
