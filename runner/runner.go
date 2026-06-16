package runner

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
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
	"path/filepath"
)

var (
	isDebuggerPresent = security.IsDebuggerPresent
	isSandboxed       = security.IsSandboxed
)

func Run(srcpath string, password string, secureMode bool, enforceSignerAuth bool) (error, errrs.ErrorType) {
	telemetryPath := os.Getenv(security.SecurityTelemetryFileEnv)
	if telemetryPath != "" {
		defer func() {
			_ = security.ExportSecurityTelemetry(telemetryPath)
		}()
	}

	signedCode, err := os.ReadFile(srcpath)
	if err != nil {
		return err, errrs.ERROR
	}

	signedCode, err = extractStandaloneSignedCode(signedCode)
	if err != nil {
		return err, errrs.ERROR
	}

	if secureMode && enforceSignerAuth {
		trustedPublicKey, generated, keyDir, keyErr := security.ResolveTrustedPublicKeyHex()
		if keyErr != nil {
			return fmt.Errorf("failed to resolve trusted public key: %w", keyErr), errrs.ERROR
		}

		if generated {
			privatePath, publicPath := security.LocalKeyPairPaths(keyDir)
			fmt.Fprintf(os.Stderr,
				"[security] generated local keypair for secure mode bootstrap\n[security] private=%s\n[security] public=%s\n[security] optional: set %s to the public key for explicit pinning\n",
				filepath.Clean(privatePath),
				filepath.Clean(publicPath),
				security.TrustedPublicKeyEnv,
			)
		}

		if err := security.VerifyCodeWithTrustedPublicKey(signedCode, trustedPublicKey); err != nil {
			security.RecordSignatureFailure("secure-mode-verify")
			if responseErr := security.ApplyTamperResponse("signature_failed", "secure-mode-verify", secureMode, err); responseErr != nil {
				return responseErr, errrs.ERROR
			}
		}
	} else if !secureMode {
		if err := security.VerifyCode(signedCode); err != nil {
			security.RecordSignatureFailure("compat-mode-verify")
			if responseErr := security.ApplyTamperResponse("signature_failed", "compat-mode-verify", secureMode, err); responseErr != nil {
				return responseErr, errrs.ERROR
			}
		}
	}

	if err := enforceAntiRev(secureMode, "pre-decode"); err != nil {
		return err, errrs.ERROR
	}

	bytecode, err := decode(signedCode, password)
	if err != nil {
		return err, errrs.ERROR
	}

	if err := enforceAntiRev(secureMode, "pre-execution"); err != nil {
		return err, errrs.ERROR
	}

	return runvm(bytecode, password, secureMode)
}

func enforceAntiRev(secureMode bool, stage string) error {
	if err := enforceAntiDebug(secureMode, stage); err != nil {
		return err
	}
	if err := enforceAntiSandbox(secureMode, stage); err != nil {
		return err
	}
	return nil
}

func enforceAntiDebug(secureMode bool, stage string) error {
	if !isDebuggerPresent() {
		return nil
	}
	security.RecordDebuggerDetected(stage)

	return security.ApplyTamperResponse("debugger_detected", stage, secureMode, security.ErrDebuggerDetected)
}

func enforceAntiSandbox(secureMode bool, stage string) error {
	if !isSandboxed() {
		return nil
	}
	security.RecordSandboxDetected(stage)

	return security.ApplyTamperResponse("sandbox_detected", stage, secureMode, security.ErrSandboxDetected)
}

func decode(data []byte, password string) (*compiler.ByteCode, error) {
	decodedData, err := decryptCode(data, password)
	if err != nil {
		return nil, err
	}
	defer security.SecureZero(decodedData)
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
	defer security.SecureZero(xorEncryptedData)

	// Decrypt the XOR layer (key is embedded in the data)
	decodedData, err := security.SecureXORDecrypt(xorEncryptedData)
	if err != nil {
		return nil, err
	}

	return decodedData, nil
}

func extractStandaloneSignedCode(binaryData []byte) ([]byte, error) {
	if len(binaryData) < security.StandaloneTailSize {
		return binaryData, nil
	}

	trailerStart := len(binaryData) - security.StandaloneTailSize
	trailer := binaryData[trailerStart:]

	if !bytes.Equal(trailer[:len(security.StandaloneTailMarker)], []byte(security.StandaloneTailMarker)) {
		return binaryData, nil
	}

	versionOffset := len(security.StandaloneTailMarker)
	if trailer[versionOffset] != security.StandaloneTailV1 {
		return nil, fmt.Errorf("unsupported standalone trailer version: %d", trailer[versionOffset])
	}

	lengthStart := versionOffset + 1
	lengthEnd := lengthStart + 8
	payloadLength := binary.BigEndian.Uint64(trailer[lengthStart:lengthEnd])

	if payloadLength == 0 {
		return nil, errors.New("invalid standalone payload length: 0")
	}

	if payloadLength > uint64(trailerStart) {
		return nil, fmt.Errorf("invalid standalone payload length: %d", payloadLength)
	}

	payloadStart := trailerStart - int(payloadLength)
	payload := binaryData[payloadStart:trailerStart]

	expectedChecksum := trailer[lengthEnd:]
	actualChecksum := sha256.Sum256(payload)
	if !bytes.Equal(expectedChecksum, actualChecksum[:]) {
		return nil, errors.New("standalone payload checksum mismatch")
	}

	return payload, nil
}

func HasStandalonePayload(srcpath string) (bool, error) {
	binaryData, err := os.ReadFile(srcpath)
	if err != nil {
		return false, err
	}

	if len(binaryData) < security.StandaloneTailSize {
		return false, nil
	}

	trailerStart := len(binaryData) - security.StandaloneTailSize
	trailer := binaryData[trailerStart:]

	if !bytes.Equal(trailer[:len(security.StandaloneTailMarker)], []byte(security.StandaloneTailMarker)) {
		return false, nil
	}

	versionOffset := len(security.StandaloneTailMarker)
	if trailer[versionOffset] != security.StandaloneTailV1 {
		return false, fmt.Errorf("unsupported standalone trailer version: %d", trailer[versionOffset])
	}

	return true, nil
}

func runvm(bytecode *compiler.ByteCode, password string, secureMode bool) (error, errrs.ErrorType) {
	globals := make([]object.Object, global.GlobalSize)
	machine := vm.NewWithPasswordAndGlobalStoreMode(bytecode, password, globals, secureMode)
	defer machine.CleanupSensitiveData(true)

	if err := machine.Run(); err != nil {
		return err, errrs.VM_ERROR
	}

	last := machine.LastPoppedStackElement()
	io.WriteString(os.Stdout, last.Inspect())
	io.WriteString(os.Stdout, "\n")

	return nil, ""
}

func registerTypes() {
	gob.Register(&object.Float{})
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
