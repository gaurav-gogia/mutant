package generator

import (
	"encoding/base64"
	"errors"
	"mutant/binformat"
	"os"
	"regexp"
)

const (
	EXPRESSION = `(\|\#\|)(.+)(\|\#\|)`
	DARWIN     = "darwin"
	LINUX      = "linux"
	WINDOWS    = "windows"

	AMD64 = "amd64"
	X86   = "386"
	X862  = "x86"
	ARM64 = "arm64"
	ARM   = "arm"
)

func writeBinary(dstpath, goos, goarch string, bytecode []byte) error {
	format, err := getBinaryFormat(goos, goarch)
	if err != nil {
		return err
	}

	_, err = base64.StdEncoding.Decode(format, format)
	if err != nil {
		return err
	}

	re := regexp.MustCompile(EXPRESSION)
	matches := re.FindAll(format, -1)

	replacement := make([]byte, len(matches[0]))

	for i := range replacement {
		if i < len(bytecode) {
			replacement[i] = bytecode[i]
			continue
		}
		replacement[i] = 'x'
	}
	replacement[len(bytecode)] = '#'

	replaced := re.ReplaceAllLiteral(format, replacement)
	return os.WriteFile(dstpath, replaced, 0755)
}

func getBinaryFormat(goos, goarch string) ([]byte, error) {
	switch goos {
	case DARWIN:
		if goarch == AMD64 {
			return []byte(binformat.DarwinAmd64), nil
		}

	case LINUX:
		if goarch == AMD64 {
			return []byte(binformat.LinuxAmd64), nil
		}
		if goarch == X86 || goarch == X862 {
			return []byte(binformat.Linux386), nil
		}
		if goarch == ARM64 {
			return []byte(binformat.LinuxArm64), nil
		}
		if goarch == ARM {
			return []byte(binformat.LinuxArm386), nil
		}

	case WINDOWS:
		if goarch == AMD64 {
			return []byte(binformat.WindowsAmd64), nil
		}
		if goarch == X86 || goarch == X862 {
			return []byte(binformat.Windows386), nil
		}
		if goarch == ARM {
			return []byte(binformat.WindowsArm386), nil
		}
	}

	return nil, errors.New("this platform is not supported at the moment, please create an issue or look into existing ones for more details")
}
