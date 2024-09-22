package cli

import (
	"fmt"
	"mutant/errrs"
	"mutant/generator"
	"mutant/global"
	"mutant/repl"
	"mutant/runner"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"
)

func RunRepl(version string, enableMacros bool) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		<-c
		repl.GracefulExit()
	}()
	repl.Start(os.Stdin, os.Stdout, version, enableMacros)
}

func CompileCode(src, goos, goarch string, release bool) {
	start := time.Now()
	srcpath, err := filepath.Abs(src)
	if err != nil {
		fmt.Println(err)
		return
	}
	dstpath := strings.TrimSuffix(srcpath, global.MutantSourceCodeFileExtention)

	if err, errtype, errors := generator.Generate(srcpath, dstpath, goos, goarch, release); err != nil {
		switch errtype {
		case errrs.ERROR:
			fmt.Println(err)
		case errrs.PARSER_ERROR:
			errrs.PrintParseErrors(os.Stdout, errors)
		case errrs.COMPILER_ERROR:
			errrs.PrintCompilerError(os.Stdout, err.Error())
		}
		return
	}

	fmt.Println("Compiled in:", time.Since(start))
}

func RunCode(src string) {
	srcpath, err := filepath.Abs(src)
	if err != nil {
		fmt.Println(err)
		return
	}

	if err, errtype := runner.Run(srcpath); err != nil {
		switch errtype {
		case errrs.ERROR:
			fmt.Println(err)
		case errrs.VM_ERROR:
			errrs.PrintMachineError(os.Stdout, err.Error())
		}
	}
}
