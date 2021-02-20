package generator

import (
	"bytes"
	"encoding/base64"
	"encoding/gob"
	"fmt"
	"io/ioutil"
	"mutant/compiler"
	"mutant/errrs"
	"mutant/lexer"
	"mutant/object"
	"mutant/parser"
	"mutant/security"
)

// Generate function takes a `string`, it's the path for the source code
func Generate(srcpath, dstpath string) (error, errrs.ErrorType, []string) {
	data, err := ioutil.ReadFile(srcpath)
	if err != nil {
		return err, errrs.ERROR, nil
	}

	bytecode, err, errtype, errors := compile(data)
	if err != nil {
		return err, errtype, errors
	}
	if err := ioutil.WriteFile(dstpath, bytecode, 0644); err != nil {
		return err, errrs.ERROR, nil
	}

	return nil, "", nil
}

func compile(data []byte) ([]byte, error, errrs.ErrorType, []string) {
	constants := []object.Object{}
	symbolTable := compiler.NewSymbolTable()
	for i, v := range object.Builtins {
		symbolTable.DefineBuiltin(i, v.Name)
	}

	l := lexer.New(string(data))
	p := parser.New(l)
	program := p.ParseProgram()

	if len(p.Errors()) != 0 {
		return nil, fmt.Errorf("pareser error"), errrs.PARSER_ERROR, p.Errors()
	}

	comp := compiler.NewWithState(symbolTable, constants)
	if err := comp.Compile(program); err != nil {
		return nil, err, errrs.COMPILER_ERROR, nil
	}

	encodedByteCode, err := encode(comp.ByteCode())
	if err != nil {
		return nil, err, errrs.ERROR, nil
	}

	return encodedByteCode, nil, "", nil
}

func encode(compByteCode *compiler.ByteCode) ([]byte, error) {
	var content bytes.Buffer
	compByteCode.Instructions = security.XOR(compByteCode.Instructions, len(compByteCode.Instructions))

	registerTypes()

	enc := gob.NewEncoder(&content)
	if err := enc.Encode(compByteCode); err != nil {
		return nil, err
	}

	byteCode := content.Bytes()
	encodedLen := base64.StdEncoding.EncodedLen(len(byteCode))
	encodedByteCode := make([]byte, encodedLen)
	base64.StdEncoding.Encode(encodedByteCode, byteCode)

	return encodedByteCode, nil
}

func registerTypes() {
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
}
