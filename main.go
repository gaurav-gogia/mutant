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

const (
	RELEASECMD = "release"
	GENCMD     = "gen"
	RUNCMD     = "run"
	VERSION    = "Version: 2.1.0_dev"
)

func main() {
	if len(os.Args) == 1 {
		cli.RunRepl(VERSION, false)
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

			fmt.Println("\tmutant -em, --enableMacros")
			fmt.Println("\t\tRun mutant in REPL mode with experimental macros support.")
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
			fmt.Println("\tmutant gen -src <FILENAME>.mut [-password|-pwd]")
			fmt.Println("\t\tCompile mutant source code into bytecode with optional password.")
			fmt.Println()
			fmt.Println("\tmutant release -src <FILENAME>.mut [-os | -arch]")
			fmt.Println("\t\tCompile mutant source code into standalone, independent binary executable.")
			fmt.Println("")
			fmt.Println("\t\tOptional: -password|-pwd <STRING> to encrypt output with a password.")
			fmt.Println("\t\tIf omitted, deterministic encryption (no password) is used.")
			fmt.Println("")
			fmt.Println("\t\tPossible values for -os: darwin | linux | windows.")
			fmt.Println("\t\tPossible values for -arch: amd64 | arm64 | arm | 386 | x86. (386 & x86 have same meaning here)")

			fmt.Println("")
			fmt.Println("Examples:")
			fmt.Println("\tmutant gen -src hello.mut -pwd \"My$tr0ngPass!\"")

			return
		}

		if os.Args[1] == "-em" || os.Args[1] == "enableMacros" {
			cli.RunRepl(VERSION, true)
			return
		}

		if os.Args[1] == "-v" || os.Args[1] == "--version" {
			fmt.Println(VERSION)
			return
		}

		if strings.HasSuffix(os.Args[1], global.MutantSourceCodeFileExtention) {
			cli.CompileCode(os.Args[1], "", "", false, "")
			return
		}

		if strings.HasSuffix(os.Args[1], global.MutantByteCodeCompiledFileExtension) {
			cli.RunCode(os.Args[1], "")
			return
		}
	}

	// General CLI: support password for compile/run (non-release, non-gen)
	if len(os.Args) >= 2 && os.Args[1] != RELEASECMD && os.Args[1] != GENCMD {
		// Try to find a file argument anywhere in the args
		var fileArg string
		for i := 1; i < len(os.Args); i++ {
			if strings.HasSuffix(os.Args[i], global.MutantSourceCodeFileExtention) ||
				strings.HasSuffix(os.Args[i], global.MutantByteCodeCompiledFileExtension) {
				fileArg = os.Args[i]
				break
			}
		}

		if fileArg != "" {
			password := extractPasswordArg(os.Args)
			if strings.HasSuffix(fileArg, global.MutantSourceCodeFileExtention) {
				cli.CompileCode(fileArg, "", "", false, password)
				return
			}
			if strings.HasSuffix(fileArg, global.MutantByteCodeCompiledFileExtension) {
				cli.RunCode(fileArg, password)
				return
			}
		}
	}

	if len(os.Args) >= 2 && (os.Args[1] == GENCMD || os.Args[1] == RUNCMD) {
		src, password, err := prepareGenRun(os.Args)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		fmt.Println("Generating Bytecode....")
		cli.CompileCode(src, "", "", false, password)
		return
	}

	if len(os.Args) >= 2 && os.Args[1] == RELEASECMD {
		src, goos, goarch, password, err := prepareRelease(os.Args)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		fmt.Println("Compiling Release Build....")
		cli.CompileCode(src, goos, goarch, true, password)
		return
	}
}

// extractPasswordArg scans args for -password|-pwd or --password=|--pwd=<value>
func extractPasswordArg(args []string) string {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "-password" || args[i] == "-pwd" {
			return args[i+1]
		}
	}
	for i := 0; i < len(args); i++ {
		if strings.HasPrefix(args[i], "--password=") {
			return strings.TrimPrefix(args[i], "--password=")
		}
		if strings.HasPrefix(args[i], "--pwd=") {
			return strings.TrimPrefix(args[i], "--pwd=")
		}
	}
	return ""
}

func prepareRelease(args []string) (string, string, string, string, error) {
	var goos, goarch, src, password string

	releasecmd := flag.NewFlagSet(RELEASECMD, flag.ExitOnError)

	releasecmd.StringVar(&src, "src", "", "Mutant Source Code File Path by using -src flag")
	releasecmd.StringVar(&goos, "os", runtime.GOOS, "Use thie flag to specify target OS for cross-compilation by using -os flag")
	releasecmd.StringVar(&goarch, "arch", runtime.GOARCH, "Use thie flag to specify target Architecture for cross-compilation by using -arch flag")
	releasecmd.StringVar(&password, "password", "", "Optional password for encryption (leave empty for deterministic encryption)")
	releasecmd.StringVar(&password, "pwd", "", "Short for -password")

	if err := releasecmd.Parse(args[2:]); err != nil {
		return "", "", "", "", err
	}

	if releasecmd.Parsed() {
		if src == "" {
			return "", "", "", "", errors.New("mutant source code file path is required, please use -src flag")
		}

		if !strings.HasSuffix(src, global.MutantSourceCodeFileExtention) {
			return "", "", "", "", errors.New("incorrect file extension, this program only works for mutant source code files")
		}

		absSrc, err := filepath.Abs(src)
		if err != nil {
			return "", "", "", "", err
		}

		return absSrc, goos, goarch, password, nil
	}

	return "", "", "", "", errors.New("could not parse values")
}

func prepareGenRun(args []string) (string, string, error) {
	var src, password string

	gencmd := flag.NewFlagSet(GENCMD, flag.ExitOnError)

	gencmd.StringVar(&src, "src", "", "Mutant Source Code File Path by using -src flag")
	gencmd.StringVar(&password, "password", "", "Optional password for encryption (leave empty for deterministic encryption)")
	gencmd.StringVar(&password, "pwd", "", "Short for -password")

	if err := gencmd.Parse(args[2:]); err != nil {
		return "", "", err
	}

	if gencmd.Parsed() {
		if src == "" {
			return "", "", errors.New("mutant source code file path is required, please use -src flag")
		}

		if !strings.HasSuffix(src, global.MutantSourceCodeFileExtention) {
			return "", "", errors.New("incorrect file extension, this program only works for mutant source code files")
		}

		absSrc, err := filepath.Abs(src)
		if err != nil {
			return "", "", err
		}

		return absSrc, password, nil
	}

	return "", "", errors.New("could not parse values")
}
