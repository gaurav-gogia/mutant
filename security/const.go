package security

import "errors"

// Header and footer constant strings for file identification
const (
	HEADER    = "MUT"
	FOOTER    = "ANT"
	ENCSIG    = "MUTANT"
	SEPERATOR = "|"
)

// ErrWrongSignature error is returned if signature doesn't match
var ErrWrongSignature = errors.New("Wrong signature, compromised or wrong file")
