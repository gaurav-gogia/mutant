package runner

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mutant/errrs"
	"mutant/security"
)

func TestRunSecureModeRejectsMalformedPayload(t *testing.T) {
	keyPair, err := security.GenerateKeyPair()
	if err != nil {
		t.Fatalf("failed to generate key pair: %v", err)
	}

	t.Setenv(security.TrustedPublicKeyEnv, toHex(t, keyPair.PublicKey))

	path := writeTempPayload(t, []byte("legacy-format-payload"))
	err, errType := Run(path, "", true, true)
	if err == nil {
		t.Fatalf("expected secure mode to reject malformed payload")
	}
	if errType != errrs.ERROR {
		t.Fatalf("expected errrs.ERROR, got %q", errType)
	}
	if !errors.Is(err, security.ErrWrongSignature) {
		t.Fatalf("expected ErrWrongSignature, got: %v", err)
	}
}

func TestRunSecureModeBootstrapsLocalKeysWhenTrustedEnvMissing(t *testing.T) {
	keyDir := t.TempDir()
	t.Setenv(security.LocalKeyStoreDirEnv, keyDir)
	t.Setenv(security.TrustedPublicKeyEnv, "")

	path := writeTempPayload(t, []byte("legacy-format-payload"))
	err, errType := Run(path, "", true, true)
	if err == nil {
		t.Fatalf("expected malformed payload to fail")
	}
	if errType != errrs.ERROR {
		t.Fatalf("expected errrs.ERROR, got %q", errType)
	}
	if !errors.Is(err, security.ErrWrongSignature) {
		t.Fatalf("expected ErrWrongSignature, got: %v", err)
	}

	privatePath := filepath.Join(keyDir, security.LocalSigningPrivateKeyFileName)
	publicPath := filepath.Join(keyDir, security.LocalSigningPublicKeyFileName)

	if _, statErr := os.Stat(privatePath); statErr != nil {
		t.Fatalf("expected bootstrapped private key file: %v", statErr)
	}
	if _, statErr := os.Stat(publicPath); statErr != nil {
		t.Fatalf("expected bootstrapped public key file: %v", statErr)
	}
}

func TestRunCompatModeWarnsAndContinuesOnMalformedPayload(t *testing.T) {
	t.Setenv(security.TamperResponseEnv, security.TamperResponseWarn)

	path := writeTempPayload(t, []byte("legacy-format-payload"))
	err, errType := Run(path, "", false, false)
	if err == nil {
		t.Fatalf("expected compat mode to fail later during decode")
	}
	if errType != errrs.ERROR {
		t.Fatalf("expected errrs.ERROR, got %q", errType)
	}
	if errors.Is(err, security.ErrWrongSignature) {
		t.Fatalf("expected compatibility mode to continue past signature failure")
	}
}

func TestRunSecureModeRejectsTamperedSignedPayload(t *testing.T) {
	keyPair, err := security.GenerateKeyPair()
	if err != nil {
		t.Fatalf("failed to generate key pair: %v", err)
	}

	t.Setenv(security.TrustedPublicKeyEnv, toHex(t, keyPair.PublicKey))

	signed, err := security.SignCode("not-valid-metadata", keyPair.PrivateKey)
	if err != nil {
		t.Fatalf("failed to sign payload: %v", err)
	}

	tampered := tamperSignedPublicKey(t, string(signed))
	path := writeTempPayload(t, []byte(tampered))

	err, errType := Run(path, "", true, true)
	if err == nil {
		t.Fatalf("expected secure mode to reject tampered signed payload")
	}
	if errType != errrs.ERROR {
		t.Fatalf("expected errrs.ERROR, got %q", errType)
	}
	if !errors.Is(err, security.ErrUntrustedSigner) {
		t.Fatalf("expected ErrUntrustedSigner, got: %v", err)
	}
}

func TestRunSecureModeAcceptsSignatureThenFailsDecode(t *testing.T) {
	keyPair, err := security.GenerateKeyPair()
	if err != nil {
		t.Fatalf("failed to generate key pair: %v", err)
	}

	t.Setenv(security.TrustedPublicKeyEnv, toHex(t, keyPair.PublicKey))

	signed, err := security.SignCode("not-valid-metadata", keyPair.PrivateKey)
	if err != nil {
		t.Fatalf("failed to sign payload: %v", err)
	}

	path := writeTempPayload(t, signed)
	err, errType := Run(path, "", true, true)
	if err == nil {
		t.Fatalf("expected decode failure after signature verification")
	}
	if errType != errrs.ERROR {
		t.Fatalf("expected errrs.ERROR, got %q", errType)
	}
	if errors.Is(err, security.ErrUntrustedSigner) || errors.Is(err, security.ErrWrongSignature) {
		t.Fatalf("expected non-signature decode error after successful signature verification, got: %v", err)
	}
}

func TestRunSecureModeWithoutSignerAuthFlagSkipsSignatureVerification(t *testing.T) {
	t.Setenv(security.TamperResponseEnv, "")

	path := writeTempPayload(t, []byte("legacy-format-payload"))
	err, errType := Run(path, "", true, false)
	if err == nil {
		t.Fatalf("expected malformed payload to fail decode")
	}
	if errType != errrs.ERROR {
		t.Fatalf("expected errrs.ERROR, got %q", errType)
	}
	if errors.Is(err, security.ErrWrongSignature) || errors.Is(err, security.ErrUntrustedSigner) {
		t.Fatalf("expected secure mode to skip signer verification by default, got: %v", err)
	}
}

func TestEnforceAntiSandboxSecureModeTerminates(t *testing.T) {
	originalSandbox := isSandboxed
	isSandboxed = func() bool { return true }
	defer func() {
		isSandboxed = originalSandbox
	}()

	t.Setenv(security.TamperResponseEnv, "")
	err := enforceAntiSandbox(true, "test-stage")
	if err == nil {
		t.Fatalf("expected secure mode to terminate on sandbox detection")
	}
	if !errors.Is(err, security.ErrSandboxDetected) {
		t.Fatalf("expected ErrSandboxDetected, got: %v", err)
	}
}

func TestEnforceAntiSandboxCompatModeWarnsAndContinues(t *testing.T) {
	originalSandbox := isSandboxed
	isSandboxed = func() bool { return true }
	defer func() {
		isSandboxed = originalSandbox
	}()

	t.Setenv(security.TamperResponseEnv, security.TamperResponseWarn)
	err := enforceAntiSandbox(false, "test-stage")
	if err != nil {
		t.Fatalf("expected compat warn mode to continue, got: %v", err)
	}
}

func writeTempPayload(t *testing.T, data []byte) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "payload-*.mu")
	if err != nil {
		t.Fatalf("failed creating temp payload: %v", err)
	}
	defer f.Close()
	if _, err := f.Write(data); err != nil {
		t.Fatalf("failed writing temp payload: %v", err)
	}
	return f.Name()
}

func tamperSignedPublicKey(t *testing.T, signed string) string {
	t.Helper()
	parts := strings.Split(signed, security.OUTER_SEPERATOR)
	if len(parts) < 5 {
		t.Fatalf("unexpected signed payload format")
	}
	pub := parts[len(parts)-2]
	if len(pub) == 0 {
		t.Fatalf("unexpected empty public key")
	}
	if pub[0] == 'a' {
		pub = "b" + pub[1:]
	} else {
		pub = "a" + pub[1:]
	}
	parts[len(parts)-2] = pub
	return strings.Join(parts, security.OUTER_SEPERATOR)
}

func toHex(t *testing.T, b []byte) string {
	t.Helper()
	const hex = "0123456789abcdef"
	out := make([]byte, len(b)*2)
	for i, v := range b {
		out[i*2] = hex[v>>4]
		out[i*2+1] = hex[v&0x0f]
	}
	return string(out)
}
