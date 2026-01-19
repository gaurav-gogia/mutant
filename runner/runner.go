package runner

import (
	"bytes"
	"encoding/gob"
	"errors"
	"fmt"
	"io"
	"mutant/builtin"
	"mutant/compiler"
	"mutant/errrs"
	"mutant/global"
	"mutant/object"
	"mutant/security"
	"mutant/vm"
	"os"
)

func Run(srcpath string, password string) (error, errrs.ErrorType) {
	signedCode, err := os.ReadFile(srcpath)
	if err != nil {
		return err, errrs.ERROR
	}

	if err := security.VerifyCode(signedCode); err != nil {
		return err, errrs.ERROR
	}

	bytecode, err := decode(signedCode, password)
	if err != nil {
		return err, errrs.ERROR
	}

	if security.IsDebuggerPresent() {
		fmt.Println("debugger found, exiting")
		return errors.New("debugger found"), errrs.ERROR
	}

	return runvm(bytecode, password)
}

func decode(data []byte, password string) (*compiler.ByteCode, error) {
	decodedData, err := decryptCode(data, password)
	if err != nil {
		return nil, err
	}
	reader := bytes.NewReader(decodedData)

	var bytecode *compiler.ByteCode
	registerTypes()
	dec := gob.NewDecoder(reader)
	if err := dec.Decode(&bytecode); err != nil {
		return nil, err
	}

	return bytecode, nil
}

func decryptCode(signedCode []byte, password string) ([]byte, error) {
	encryptedMetadata := security.GetEncryptedCode(signedCode)

	// Decrypt using the new secure method
	var xorEncryptedData []byte
	var err error

	xorEncryptedData, err = security.AESDecrypt(encryptedMetadata, password)
	if err != nil {
		return nil, err
	}

	// Decrypt the XOR layer (key is embedded in the data)
	decodedData, err := security.SecureXORDecrypt(xorEncryptedData)
	if err != nil {
		return nil, err
	}

	return decodedData, nil
}

func runvm(bytecode *compiler.ByteCode, password string) (error, errrs.ErrorType) {
	globals := make([]object.Object, global.GlobalSize)
	machine := vm.NewWithPasswordAndGlobalStore(bytecode, password, globals)

	if err := machine.Run(); err != nil {
		return err, errrs.VM_ERROR
	}

	last := machine.LastPoppedStackElement()
	io.WriteString(os.Stdout, last.Inspect())
	io.WriteString(os.Stdout, "\n")

	return nil, ""
}

func registerTypes() {
	gob.Register(&object.Integer{})
	gob.Register(&object.Boolean{})
	gob.Register(&object.Null{})
	gob.Register(&object.ReturnValue{})
	gob.Register(&object.Error{})
	gob.Register(&object.Function{})
	gob.Register(&object.String{})
	gob.Register(&builtin.BuiltIn{})
	gob.Register(&object.Array{})
	gob.Register(&object.Hash{})
	gob.Register(&object.Quote{})
	gob.Register(&object.Macro{})
	gob.Register(&object.CompiledFunction{})
	gob.Register(&object.Closure{})
	gob.Register(&object.Encrypted{})
}
