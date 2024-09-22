package mutil

import (
	"encoding/binary"
	"errors"
	"mutant/compiler"
	"mutant/global"
	"mutant/object"
	"mutant/security"
	"strconv"
	"strings"
)

func EncryptByteCode(byteCode *compiler.ByteCode) *compiler.ByteCode {
	byteCode.Instructions = security.XOR(byteCode.Instructions, len(byteCode.Instructions))
	insLen := len(byteCode.Instructions)

	for i := range byteCode.Constants {
		if byteCode.Constants[i].Type() == object.COMPILED_FN_OBJ {
			ins := byteCode.Constants[i].(*object.CompiledFunction).Instructions
			ins = security.XOR(ins, insLen)
			byteCode.Constants[i].(*object.CompiledFunction).Instructions = ins
			continue
		}

		if encConst, err := EncryptObject(byteCode.Constants[i], insLen); err == nil {
			byteCode.Constants[i] = encConst
		}
	}

	return byteCode
}

func EncryptObject(obj object.Object, length int) (object.Object, error) {
	var encObj object.Object
	var err error

	switch obj.Type() {
	case object.INTEGER_OBJ:
		val := obj.(*object.Integer).Value
		bite := make([]byte, 8)
		binary.LittleEndian.PutUint64(bite, uint64(val))
		bite = security.XOR(bite, length)

		encObj = &object.Encrypted{
			EncType: object.INTEGER_OBJ,
			Value:   bite,
		}

	case object.STRING_OBJ:
		val := obj.(*object.String).Value
		bite := security.XOR([]byte(val), length)

		encObj = &object.Encrypted{
			EncType: object.STRING_OBJ,
			Value:   bite,
		}

	case object.BOOLEAN_OBJ:
		val := obj.(*object.Boolean).Value
		str := strconv.FormatBool(val)
		bite := security.XOR([]byte(str), length)

		encObj = &object.Encrypted{
			EncType: object.BOOLEAN_OBJ,
			Value:   bite,
		}

	default:
		err = errors.New("wrong obj type")
	}

	return encObj, err
}

func DecryptObject(obj object.Object, length int) (object.Object, error) {
	decObj := obj
	var err error

	if decObj.Type() == object.ENCRYPTED_OBJ {
		biteVal := decObj.(*object.Encrypted).Value
		bite := make([]byte, len(biteVal))
		copy(bite, biteVal)
		bite = security.XOR(bite, length)

		switch decObj.(*object.Encrypted).EncType {
		case object.INTEGER_OBJ:
			val := binary.LittleEndian.Uint64(bite)
			decObj = &object.Integer{Value: int64(val)}

		case object.STRING_OBJ:
			decObj = &object.String{Value: string(bite)}

		case object.BOOLEAN_OBJ:
			str := strings.ToLower(string(bite))
			if str == "true" {
				decObj = global.True
			} else {
				decObj = global.False
			}
		}

		return decObj, nil
	}

	err = errors.New("wrong obj type")
	return obj, err
}
