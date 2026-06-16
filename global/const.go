package global

// Initial capacities for VM storage. The VM grows these slices dynamically at runtime.
const (
	StackSize  = 2048
	GlobalSize = 65536
	MaxFrames  = 2048

	MutantSourceCodeFileExtention       = ".mut"
	MutantByteCodeCompiledFileExtension = ".mu"
	WindowsPE32ExecutableExtension      = ".exe"
)

const (
	DARWIN  = "darwin"
	LINUX   = "linux"
	WINDOWS = "windows"
)
