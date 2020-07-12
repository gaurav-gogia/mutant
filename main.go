package main

import (
	"mutant/cli"
	"os"
	"strings"
)

func main() {
	if len(os.Args) == 1 {
		cli.RunRepl()
	} else if len(os.Args) == 2 {

		if strings.HasSuffix(os.Args[1], "mut") {
			cli.CompileCode(os.Args[1])
		} else if strings.HasSuffix(os.Args[1], "mu") {
			cli.RunCode(os.Args[1])
		}
	}
}
