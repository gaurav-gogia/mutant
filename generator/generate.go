package generator

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"mutant/builtin"
	"mutant/compiler"
	"mutant/errrs"
	"mutant/global"
	"mutant/lexer"
	"mutant/mutil"
	"mutant/object"
	"mutant/parser"
	"mutant/security"
	"os"
)

// Generate function takes a `string`, it's the path for the source code
// password: optional password for encryption (empty string for deterministic encryption)
// privateKey: Ed25519 private key for signing (if nil, a temporary key is generated)
func Generate(srcpath, dstpath, goos, goarch string, release bool, password string, privateKey []byte) (error, errrs.ErrorType, []string) {
	data, err := os.ReadFile(srcpath)
	if err != nil {
		return err, errrs.ERROR, nil
	}

	// Generate signing key if not provided
	if privateKey == nil {
		keyPair, err := security.GenerateKeyPair()
		if err != nil {
			return err, errrs.ERROR, nil
		}
		privateKey = keyPair.PrivateKey

		// In production, you'd want to save/reuse keys
		// For now, we generate a new key each time
	}

	bytecode, err, errtype, errors := compile(data, password, privateKey)
	if err != nil {
		return err, errtype, errors
	}

	if release {
		if goos == WINDOWS {
			dstpath += global.WindowsPE32ExecutableExtension
		}

		if err := writeBinary(dstpath, goos, goarch, bytecode); err != nil {
			return err, errrs.ERROR, nil
		}

		return nil, "", nil
	}

	if err := os.WriteFile(dstpath+global.MutantByteCodeCompiledFileExtension, bytecode, 0644); err != nil {
		return err, errrs.ERROR, nil
	}

	return nil, "", nil
}

func compile(data []byte, password string, privateKey []byte) ([]byte, error, errrs.ErrorType, []string) {
	constants := []object.Object{}
	symbolTable := compiler.NewSymbolTable()
	for i, v := range builtin.Builtins {
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

	encodedByteCode, err := encode(comp.ByteCode(), data, password, privateKey)
	if err != nil {
		return nil, err, errrs.ERROR, nil
	}

	return encodedByteCode, nil, "", nil
}

func encode(compByteCode *compiler.ByteCode, sourceCode []byte, password string, privateKey []byte) ([]byte, error) {
	var content bytes.Buffer

	compByteCode = mutil.EncryptByteCode(compByteCode)

	registerTypes()
	enc := gob.NewEncoder(&content)
	if err := enc.Encode(compByteCode); err != nil {
		return nil, err
	}

	byteCode := content.Bytes()
	return encryptCode(byteCode, sourceCode, password, privateKey)
}

func encryptCode(b64ByteCode []byte, sourceCode []byte, password string, privateKey []byte) ([]byte, error) {
	// Apply secure XOR (replaces insecure math/rand-based XOR)
	xorByteCode, err := security.SecureXOREncrypt(b64ByteCode)
	if err != nil {
		return nil, err
	}

	// Encrypt using new secure method (no key storage)
	var encodedByteCode string
	if password != "" {
		// Password-based encryption
		encodedByteCode, err = security.AESEncryptWithPassword(xorByteCode, password)
	} else {
		// Deterministic encryption (derives key from source code hash)
		encodedByteCode, err = security.AESEncrypt(xorByteCode, sourceCode)
	}
	if err != nil {
		return nil, err
	}

	// Sign with Ed25519 (replaces insecure MD5)
	signedCode, err := security.SignCode(encodedByteCode, privateKey)
	if err != nil {
		return nil, err
	}

	return signedCode, nil
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
