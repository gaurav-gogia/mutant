package code

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

type Instructions []byte
type Opcode byte

const (
	OpConstant Opcode = iota
	OpPop
	OpAdd
	OpSub
	OpMul
	OpDiv
	OpTrue
	OpFalse
	OpEqual
	OpUnEqual
	OpGreater
	OpMinus
	OpBang
	OpJumpFalse
	OpJump
	OpNull
	OpGetGlobal
	OpSetGlobal
	OpGetLocal
	OpSetLocal
	OpArray
	OpHash
	OpIndex
	OpCall
	OpReturnValue
	OpReturn
)

type Definition struct {
	Name          string
	OperandWidths []int
}

var definitions = map[Opcode]*Definition{
	OpConstant:    {"OpConstant", []int{2}},
	OpPop:         {"OpPop", []int{}},
	OpAdd:         {"OpAdd", []int{}},
	OpSub:         {"OpSub", []int{}},
	OpMul:         {"OpMul", []int{}},
	OpDiv:         {"OpDiv", []int{}},
	OpTrue:        {"OpTrue", []int{}},
	OpFalse:       {"OpFalse", []int{}},
	OpEqual:       {"OpEqual", []int{}},
	OpUnEqual:     {"OpUnEqual", []int{}},
	OpGreater:     {"OpGreater", []int{}},
	OpMinus:       {"OpMinus", []int{}},
	OpBang:        {"OpBang", []int{}},
	OpJumpFalse:   {"OpJumpFalse", []int{2}},
	OpJump:        {"OpJump", []int{2}},
	OpNull:        {"OpNull", []int{}},
	OpGetGlobal:   {"OpGetGlobal", []int{2}},
	OpSetGlobal:   {"OpSetGlobal", []int{2}},
	OpGetLocal:    {"OpGetLocal", []int{1}},
	OpSetLocal:    {"OpSetLocal", []int{1}},
	OpArray:       {"OpArray", []int{2}},
	OpHash:        {"OpHash", []int{2}},
	OpIndex:       {"OpIndex", []int{}},
	OpCall:        {"Opcall", []int{}},
	OpReturnValue: {"OpReturnValue", []int{}},
	OpReturn:      {"OpReturn", []int{}},
}

func Lookup(op byte) (*Definition, error) {
	def, ok := definitions[Opcode(op)]
	if !ok {
		return nil, fmt.Errorf("opcode %d undefined", op)
	}
	return def, nil
}

func Make(op Opcode, operands ...int) []byte {
	def, ok := definitions[op]
	if !ok {
		return []byte{}
	}

	instLen := 1

	for _, w := range def.OperandWidths {
		instLen += w
	}

	inst := make([]byte, instLen)
	inst[0] = byte(op)

	offset := 1

	for i, o := range operands {
		width := def.OperandWidths[i]
		switch width {
		case 1:
			inst[offset] = byte(o)
		case 2:
			binary.BigEndian.PutUint16(inst[offset:], uint16(o))
		}
		offset += width
	}

	return inst
}

func (ins Instructions) String() string {
	var out bytes.Buffer
	i := 0
	for i < len(ins) {
		def, err := Lookup(ins[i])
		if err != nil {
			fmt.Fprintf(&out, "ERROR: %s\n", err)
			continue
		}
		operands, read := ReadOperands(def, ins[i+1:])
		fmt.Fprintf(&out, "%04d %s\n", i, ins.fmtInstruction(def, operands))
		i += 1 + read
	}

	return out.String()
}

func (i Instructions) fmtInstruction(def *Definition, operands []int) string {
	operandCount := len(def.OperandWidths)
	if len(operands) != operandCount {
		return fmt.Sprintf("ERROR: operand len %d does not match defined %d\n", len(operands), operandCount)
	}

	switch operandCount {
	case 0:
		return def.Name
	case 1:
		return fmt.Sprintf("%s %d", def.Name, operands[0])
	}

	return fmt.Sprintf("ERROR: unhandled operandCount for %s\n", def.Name)
}

func ReadOperands(def *Definition, ins Instructions) ([]int, int) {
	var offset int
	operands := make([]int, len(def.OperandWidths))

	for i, width := range def.OperandWidths {
		switch width {
		case 1:
			operands[i] = int(ReadUint8(ins[offset:]))
		case 2:
			operands[i] = int(ReadUint16(ins[offset:]))
		}
		offset += width
	}

	return operands, offset
}

func ReadUint16(ins Instructions) uint16 {
	return binary.BigEndian.Uint16(ins)
}
func ReadUint8(ins Instructions) uint8 {
	return uint8(ins[0])
}
