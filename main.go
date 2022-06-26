package main

import (
	"errors"
	"flag"
	"fmt"
	"mutant/cli"
	"mutant/global"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const RELEASECMD = "release"

func main() {
	if len(os.Args) == 1 {
		cli.RunRepl()
		return
	}

	if len(os.Args) == 2 {
		if os.Args[1] == "-h" || os.Args[1] == "--help" {
			fmt.Println("mutant - an open source, secure by default programming language")
			fmt.Println()
			fmt.Println("USAGE: mutant COMMAND [OPTIONS...]")

			fmt.Println("Options:")

			fmt.Println("\tmutant")
			fmt.Println("\t\tRun mutant in REPL mode.")
			fmt.Println()

			fmt.Println("\tmutant -h, --help")
			fmt.Println("\t\tShow this help message.")
			fmt.Println()

			fmt.Println("\tmutant -v, --version")
			fmt.Println("\t\tShow version information.")
			fmt.Println()

			fmt.Println("\tmutant <FILENAME>.mut")
			fmt.Println("\t\tCompile mutant source code into mutant bytecode.")
			fmt.Println()

			fmt.Println("\tmutant <FILENAME>.mu")
			fmt.Println("\t\tRun mutant bytecode using mutant VM.")
			fmt.Println()

			fmt.Println("\tmutant release -src <FILENAME>.mut [-os | -arch]")
			fmt.Println("\t\tCompile mutant source code into standalone, independent binary executable.")
			fmt.Println("")
			fmt.Println("\t\tPossible values for -os: darwin | linux | windows.")
			fmt.Println("\t\tPossible values for -arch: amd64 | arm64 | arm | 386 | x86. (386 & x86 have same meaning here)")

			return
		}

		if os.Args[1] == "-v" || os.Args[1] == "--version" {
			fmt.Println("Version: 2.0.1")
			return
		}

		if strings.HasSuffix(os.Args[1], global.MutantSourceCodeFileExtention) {
			cli.CompileCode(os.Args[1], "", "", false)
			return
		}

		if strings.HasSuffix(os.Args[1], global.MutantByteCodeCompiledFileExtension) {
			cli.RunCode(os.Args[1])
			return
		}

	}

	if len(os.Args) >= 2 && os.Args[1] == RELEASECMD {
		src, goos, goarch, err := prepareRelease(os.Args)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		fmt.Println("Compiling Release Build....")
		cli.CompileCode(src, goos, goarch, true)
		return
	}
}

func prepareRelease(args []string) (string, string, string, error) {
	var goos, goarch, src string

	releasecmd := flag.NewFlagSet(RELEASECMD, flag.ExitOnError)

	releasecmd.StringVar(&src, "src", "", "Mutant Source Code File Path by using -src flag")
	releasecmd.StringVar(&goos, "os", runtime.GOOS, "Use thie flag to specify target OS for cross-compilation by using -os flag")
	releasecmd.StringVar(&goarch, "arch", runtime.GOARCH, "Use thie flag to specify target Architecture for cross-compilation by using -arch flag")

	if err := releasecmd.Parse(os.Args[2:]); err != nil {
		return "", "", "", err
	}

	if releasecmd.Parsed() {
		if src == "" {
			return "", "", "", errors.New("mutant source code file path is required, please use -src flag")
		}

		if !strings.HasSuffix(src, global.MutantSourceCodeFileExtention) {
			return "", "", "", errors.New("incorrect file extension, this program only works for mutant source code files")
		}

		absSrc, err := filepath.Abs(src)
		if err != nil {
			return "", "", "", err
		}

		return absSrc, goos, goarch, nil
	}

	return "", "", "", errors.New("could not parse values")
}
