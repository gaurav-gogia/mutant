package generator

import (
	"errors"
)

const EXPRESSION = `(\|)(\#)(\|)(.+)(\|)(\#)(\|)`

func writeBinary(dstPath, goos, goarch string, bytecode []byte) error {
	_, err := getBinaryFormat(goos, goarch)
	if err != nil {
		return err
	}

	return nil
}

func getBinaryFormat(goos, goarch string) (string, error) {
	switch goos {
	}

	return "", errors.New("this platform is not supported at the moment, please create an issue or look into existing ones for more details")
}
