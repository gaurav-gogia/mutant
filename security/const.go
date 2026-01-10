package security

import "errors"

// Header and footer constant strings for file identification
const (
	HEADER          = "MUT"
	FOOTER          = "ANT"
	ENCSIG          = "MUTANT"
	SEPERATOR       = "|"
	OUTER_SEPERATOR = "|-|"
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
)
