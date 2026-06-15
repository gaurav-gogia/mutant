package security

import "errors"

// Header and footer constant strings for file identification
const (
	HEADER               = "MUT"
	FOOTER               = "ANT"
	StandaloneTailMarker = "MUTANTBC"
	StandaloneTailV1     = byte(1)
	StandaloneTailSize   = 49
	ENCSIG               = "MUTANT"
	SEPERATOR            = "|"
	OUTER_SEPERATOR      = "|-|"
	TrustedPublicKeyEnv  = "MUTANT_TRUSTED_PUBLIC_KEY_HEX"
	SigningPrivateKeyEnv = "MUTANT_SIGNING_PRIVATE_KEY_HEX"
	LocalKeyStoreDirEnv  = "MUTANT_KEYSTORE_DIR"

	LocalSigningPrivateKeyFileName = "ed25519_private_key.hex"
	LocalSigningPublicKeyFileName  = "ed25519_public_key.hex"

	DOCKER = "Docker"
)

// Linux sandbox detection files
const (
	LNX_DCKR_ENV_0 = "/.dockerenv"
	LNX_DCKR_ENV_1 = "linux:file:/.dockerenv"
)

// Error definitions
var (
	// ErrWrongSignature error is returned if signature doesn't match
	ErrWrongSignature = errors.New("wrong signature, compromised or wrong file")

	// ErrPasswordRequired error is returned when password is needed but not provided
	ErrPasswordRequired = errors.New("password required for decryption")

	// ErrInvalidMetadata error is returned when encryption metadata is malformed
	ErrInvalidMetadata = errors.New("invalid encryption metadata")

	// ErrDebuggerDetected error is returned when a debugger is detected
	ErrDebuggerDetected = errors.New("debugger detected, execution halted for security")

	// ErrSandboxDetected error is returned when a sandbox/container/vm is detected
	ErrSandboxDetected = errors.New("sandbox detected, execution halted for security")

	// ErrUntrustedSigner error is returned when signed payload key does not match trusted key
	ErrUntrustedSigner = errors.New("untrusted signer public key")
)
