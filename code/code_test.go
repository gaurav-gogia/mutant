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
		{OpClosure, []int{65534, 255}, []byte{byte(OpClosure), 255, 254, 255}},
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
		Make(OpClosure, 65535, 255),
	}

	ex1 := "0000 OpAdd\n"
	ex2 := "0001 OpGetLocal 1\n"
	ex3 := "0003 OpConstant 2\n"
	ex4 := "0006 OpConstant 65535\n"
	ex5 := "0009 OpClosure 65535 255\n"

	expected := ex1 + ex2 + ex3 + ex4 + ex5

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
		{OpClosure, []int{65535, 255}, 3},
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
				t.Errorf("operand wrong. want = %d, got = %d, number = %d", want, operandsRead[i], i)
			}
		}
	}
}
