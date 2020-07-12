package runner

import (
	"bytes"
	"encoding/base64"
	"encoding/gob"
	"io"
	"io/ioutil"
	"mutant/compiler"
	"mutant/errrs"
	"mutant/object"
	"mutant/vm"
	"os"
)

func Run(srcpath string) (error, errrs.ErrorType) {
	data, err := ioutil.ReadFile(srcpath)
	if err != nil {
		return err, errrs.ERROR
	}

	bytecode, err := decode(data)
	if err != nil {
		return err, errrs.ERROR
	}

	return runvm(bytecode)
}

func decode(data []byte) (*compiler.ByteCode, error) {
	encodedLen := base64.StdEncoding.EncodedLen(len(data))
	decodedData := make([]byte, encodedLen)
	if _, err := base64.StdEncoding.Decode(decodedData, data); err != nil {
		return nil, err
	}

	var bytecode *compiler.ByteCode

	gob.Register(&object.Integer{})
	gob.Register(&object.Boolean{})
	gob.Register(&object.Null{})
	gob.Register(&object.ReturnValue{})
	gob.Register(&object.Error{})
	gob.Register(&object.Function{})
	gob.Register(&object.String{})
	gob.Register(&object.BuiltIn{})
	gob.Register(&object.Array{})
	gob.Register(&object.Hash{})
	gob.Register(&object.Quote{})
	gob.Register(&object.Macro{})
	gob.Register(&object.CompiledFunction{})
	gob.Register(&object.Closure{})

	dec := gob.NewDecoder(bytes.NewReader(decodedData))
	if err := dec.Decode(&bytecode); err != nil {
		return nil, err
	}

	return bytecode, nil
}

func runvm(bytecode *compiler.ByteCode) (error, errrs.ErrorType) {
	globals := make([]object.Object, vm.GlobalSize)
	machine := vm.NewWithGlobalStore(bytecode, globals)

	if err := machine.Run(); err != nil {
		return err, errrs.VM_ERROR
	}

	last := machine.LastPoppedStackElement()
	io.WriteString(os.Stdout, last.Inspect())
	io.WriteString(os.Stdout, "\n")

	return nil, ""
}
