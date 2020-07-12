package errrs

type ErrorType string

const (
	ERROR          = "ERROR"
	PARSER_ERROR   = "PARSER ERROR"
	COMPILER_ERROR = "COMPILER ERROR"
	VM_ERROR       = "VM ERROR"
)
