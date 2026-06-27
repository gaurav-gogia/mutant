package main

import (
	"errors"
	"flag"
	"fmt"
	"mutant/cli"
	"mutant/global"
	"mutant/mutil"
	"mutant/runner"
	"mutant/security"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const defaultPolymorphicLevel = 3

const (
	RELEASECMD = "release"
	GENCMD     = "gen"
	RUNCMD     = "run"
	VERSION    = "Version: 2.1.0_dev"
)

func main() {
	if shouldAttemptEmbeddedRun(os.Args) {
		executablePath, err := os.Executable()
		if err == nil {
			hasStandalonePayload, payloadErr := runner.HasStandalonePayload(executablePath)
			if payloadErr != nil {
				fmt.Println(payloadErr)
				os.Exit(1)
			}

			if hasStandalonePayload {
				password := extractPasswordArg(os.Args)
				devMode := hasDevModeArg(os.Args)
				secureMode := extractSecurityModeArg(os.Args)
				enforceSignerAuth := extractSignerAuthArg(os.Args)

				if devMode {
					secureMode = false
				}
				configureSecurityLogging(os.Args, devMode)
				if password == "" && devMode {
					password = mutil.GetPwd()
				}

				cli.RunCode(executablePath, password, secureMode, enforceSignerAuth)
				return
			}
		}
	}

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
			fmt.Println("\t\tOptional: --compat to allow compatibility mode (weaker security checks).")
			fmt.Println("\t\tOptional: --dev for developer mode (compat mode + default local password fallback).")
			fmt.Println("\t\tOptional: --security-log-level <none|error|info|debug|trace> (active in --dev mode).")
			fmt.Println("\t\tAlias: --log-level <none|error|info|debug|trace> (active in --dev mode).")
			fmt.Println("\t\tOptional: --signer-auth to enforce trusted signer key verification in secure mode.")
			fmt.Println("\t\tDefault is --secure (fail-closed security behavior).")
			fmt.Println()
			fmt.Println("\tmutant gen <FILENAME>.mut [-password|-pwd]")
			fmt.Println("\t\tCompile mutant source code into bytecode with optional password.")
			fmt.Println("\t\tOptional: -mutation <0-10> to control polymorphism level (default: 3).")
			fmt.Println("\t\tOptional: -seed <INT64> to set polymorphism seed (default: current timestamp).")
			fmt.Println("\tmutant gen --release-assets [-out <DIR>]")
			fmt.Println("\t\tGenerate embedded release runtime assets files (index + data/*.bin).")
			fmt.Println("\t\tAlso supported: mutant gen assets [-out <DIR>].")
			fmt.Println()
			fmt.Println("\tmutant release <FILENAME>.mut [-os | -arch]")
			fmt.Println("\t\tCompile mutant source code into standalone, independent binary executable.")
			fmt.Println("\t\tRelease requires embedded runtime assets for target OS/ARCH.")
			fmt.Println("")
			fmt.Println("\t\tOptional: -password|-pwd <STRING> to encrypt output with a password.")
			fmt.Println("\t\tOptional: -mutation <0-10> to control polymorphism level (default: 3).")
			fmt.Println("\t\tOptional: -seed <INT64> to set polymorphism seed (default: current timestamp).")
			fmt.Println("\t\tIf omitted, deterministic compatibility mode (weaker obfuscation) is used.")
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
			pwd := mutil.GetPwd()
			cli.CompileCode(os.Args[1], "", "", false, pwd, defaultPolymorphicLevel, time.Now().UnixNano())
			return
		}

		if strings.HasSuffix(os.Args[1], global.MutantByteCodeCompiledFileExtension) {
			pwd := mutil.GetPwd()
			cli.RunCode(os.Args[1], pwd, true, false)
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
			devMode := hasDevModeArg(os.Args)
			secureMode := extractSecurityModeArg(os.Args)
			enforceSignerAuth := extractSignerAuthArg(os.Args)
			if devMode {
				secureMode = false
			}
			configureSecurityLogging(os.Args, devMode)
			if strings.HasSuffix(fileArg, global.MutantSourceCodeFileExtention) {
				cli.CompileCode(fileArg, "", "", false, password, defaultPolymorphicLevel, time.Now().UnixNano())
				return
			}
			if strings.HasSuffix(fileArg, global.MutantByteCodeCompiledFileExtension) {
				if password == "" && devMode {
					password = mutil.GetPwd()
				}
				cli.RunCode(fileArg, password, secureMode, enforceSignerAuth)
				return
			}
		}
	}

	if len(os.Args) >= 2 && os.Args[1] == GENCMD && hasReleaseAssetsArg(os.Args) {
		out, err := prepareReleaseAssetsGeneration(os.Args)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		fmt.Println("Generating embedded release runtime assets....")
		cli.GenerateReleaseAssets(out)
		return
	}

	if len(os.Args) >= 2 && (os.Args[1] == GENCMD || os.Args[1] == RUNCMD) {
		src, password, mutationLevel, mutationSeed, err := prepareGenRun(os.Args)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		fmt.Println("Generating Bytecode....")
		cli.CompileCode(src, "", "", false, password, mutationLevel, mutationSeed)
		return
	}

	if len(os.Args) >= 2 && os.Args[1] == RELEASECMD {
		src, goos, goarch, password, mutationLevel, mutationSeed, err := prepareRelease(os.Args)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		fmt.Println("Compiling Release Build....")
		cli.CompileCode(src, goos, goarch, true, password, mutationLevel, mutationSeed)
		return
	}
}

func shouldAttemptEmbeddedRun(args []string) bool {
	if len(args) == 1 {
		return true
	}

	for _, arg := range args[1:] {
		switch arg {
		case RELEASECMD, GENCMD, RUNCMD, "-h", "--help", "-v", "--version", "-em", "--enableMacros":
			return false
		}

		if strings.HasSuffix(arg, global.MutantSourceCodeFileExtention) ||
			strings.HasSuffix(arg, global.MutantByteCodeCompiledFileExtension) {
			return false
		}
	}

	return true
}

// extractSecurityModeArg scans args for explicit mode flags.
// Defaults to secure mode unless --compat is supplied.
func extractSecurityModeArg(args []string) bool {
	secureMode := true
	for _, arg := range args {
		switch arg {
		case "--dev", "-dev":
			secureMode = false
		case "--compat", "-compat":
			secureMode = false
		case "--secure", "-secure":
			secureMode = true
		}
	}
	return secureMode
}

func hasDevModeArg(args []string) bool {
	for _, arg := range args {
		if arg == "--dev" || arg == "-dev" {
			return true
		}
	}
	return false
}

func extractSecurityLogLevelArg(args []string) string {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "--security-log-level" || args[i] == "-security-log-level" || args[i] == "--log-level" || args[i] == "-log-level" {
			return strings.TrimSpace(args[i+1])
		}
	}
	for i := 0; i < len(args); i++ {
		if strings.HasPrefix(args[i], "--security-log-level=") {
			return strings.TrimSpace(strings.TrimPrefix(args[i], "--security-log-level="))
		}
		if strings.HasPrefix(args[i], "--log-level=") {
			return strings.TrimSpace(strings.TrimPrefix(args[i], "--log-level="))
		}
	}
	return ""
}

func configureSecurityLogging(args []string, devMode bool) {
	if devMode {
		_ = os.Setenv(security.SecurityDevModeEnv, "1")
	} else {
		_ = os.Unsetenv(security.SecurityDevModeEnv)
	}

	level := extractSecurityLogLevelArg(args)
	if level != "" {
		_ = os.Setenv(security.SecurityLogLevelEnv, level)
	}
}

// extractSignerAuthArg scans args for explicit signer-auth flags.
// Defaults to disabled unless --signer-auth is supplied.
func extractSignerAuthArg(args []string) bool {
	enforceSignerAuth := false
	for _, arg := range args {
		switch arg {
		case "--signer-auth", "-signer-auth":
			enforceSignerAuth = true
		case "--no-signer-auth", "-no-signer-auth":
			enforceSignerAuth = false
		}
	}
	return enforceSignerAuth
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

func prepareRelease(args []string) (string, string, string, string, int, int64, error) {
	var goos, goarch, src, password string
	var mutationLevel int
	var mutationSeed int64

	releasecmd := flag.NewFlagSet(RELEASECMD, flag.ExitOnError)

	releasecmd.StringVar(&src, "src", "", "Mutant Source Code File Path by using -src flag")
	releasecmd.StringVar(&goos, "os", runtime.GOOS, "Use thie flag to specify target OS for cross-compilation by using -os flag")
	releasecmd.StringVar(&goarch, "arch", runtime.GOARCH, "Use thie flag to specify target Architecture for cross-compilation by using -arch flag")
	releasecmd.StringVar(&password, "password", "", "Optional password for encryption (leave empty for deterministic encryption)")
	releasecmd.StringVar(&password, "pwd", "", "Short for -password")
	releasecmd.IntVar(&mutationLevel, "mutation", defaultPolymorphicLevel, "Polymorphic mutation level (0-10)")
	releasecmd.Int64Var(&mutationSeed, "seed", 0, "Polymorphic seed (default: current timestamp)")

	if err := releasecmd.Parse(args[2:]); err != nil {
		return "", "", "", "", 0, 0, err
	}

	if src == "" {
		src = findSourceArg(args[2:])
	}

	if password == "" {
		password = extractPasswordArg(args)
	}

	if releasecmd.Parsed() {
		if src == "" {
			return "", "", "", "", 0, 0, errors.New("mutant source code file path is required, please use -src flag")
		}

		if !strings.HasSuffix(src, global.MutantSourceCodeFileExtention) {
			return "", "", "", "", 0, 0, errors.New("incorrect file extension, this program only works for mutant source code files")
		}

		absSrc, err := filepath.Abs(src)
		if err != nil {
			return "", "", "", "", 0, 0, err
		}

		return absSrc, goos, goarch, password, mutationLevel, mutationSeed, nil
	}

	return "", "", "", "", 0, 0, errors.New("could not parse values")
}

func prepareGenRun(args []string) (string, string, int, int64, error) {
	var src, password string
	var mutationLevel int
	var mutationSeed int64

	gencmd := flag.NewFlagSet(GENCMD, flag.ExitOnError)

	gencmd.StringVar(&src, "src", "", "Mutant Source Code File Path by using -src flag")
	gencmd.StringVar(&password, "password", "", "Optional password for encryption (leave empty for deterministic encryption)")
	gencmd.StringVar(&password, "pwd", "", "Short for -password")
	gencmd.IntVar(&mutationLevel, "mutation", defaultPolymorphicLevel, "Polymorphic mutation level (0-10)")
	gencmd.Int64Var(&mutationSeed, "seed", 0, "Polymorphic seed (default: current timestamp)")

	if err := gencmd.Parse(args[2:]); err != nil {
		return "", "", 0, 0, err
	}

	if src == "" {
		src = findSourceArg(args[2:])
	}

	if password == "" {
		password = extractPasswordArg(args)
	}

	if gencmd.Parsed() {
		if src == "" {
			return "", "", 0, 0, errors.New("mutant source code file path is required, please use -src flag")
		}

		if !strings.HasSuffix(src, global.MutantSourceCodeFileExtention) {
			return "", "", 0, 0, errors.New("incorrect file extension, this program only works for mutant source code files")
		}

		absSrc, err := filepath.Abs(src)
		if err != nil {
			return "", "", 0, 0, err
		}

		return absSrc, password, mutationLevel, mutationSeed, nil
	}

	return "", "", 0, 0, errors.New("could not parse values")
}

func hasReleaseAssetsArg(args []string) bool {
	if len(args) >= 3 && strings.EqualFold(args[2], "assets") {
		return true
	}

	for _, arg := range args {
		if arg == "--release-assets" || arg == "-release-assets" {
			return true
		}
	}

	return false
}

func prepareReleaseAssetsGeneration(args []string) (string, error) {
	var out string

	gencmd := flag.NewFlagSet(GENCMD, flag.ExitOnError)
	gencmd.Bool("release-assets", false, "Generate embedded release runtime assets")
	gencmd.StringVar(&out, "out", "releaseassets", "Directory for generated release assets")

	if err := gencmd.Parse(args[2:]); err != nil {
		return "", err
	}

	if out == "releaseassets" {
		for _, arg := range gencmd.Args() {
			if strings.EqualFold(arg, "assets") {
				continue
			}
			out = arg
			break
		}
	}

	absOut, err := filepath.Abs(out)
	if err != nil {
		return "", err
	}

	return absOut, nil
}

func findSourceArg(args []string) string {
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			continue
		}

		if strings.HasSuffix(arg, global.MutantSourceCodeFileExtention) {
			return arg
		}
	}

	return ""
}
