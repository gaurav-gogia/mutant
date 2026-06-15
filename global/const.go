package global

// Constants for VM
const (
	StackSize  = 2048 * 10
	GlobalSize = 65536 * 10
	MaxFrames  = 2048 * 10

	MutantSourceCodeFileExtention       = ".mut"
	MutantByteCodeCompiledFileExtension = ".mu"
	WindowsPE32ExecutableExtension      = ".exe"
)

const (
	DARWIN  = "darwin"
	LINUX   = "linux"
	WINDOWS = "windows"
)
