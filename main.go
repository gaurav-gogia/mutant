package main

import (
	"fmt"
	"mutant/repl"
	"os"
	"os/user"
)

func main() {
	user, err := user.Current()
	if err != nil {
		panic(err)
	}
	fmt.Printf("Hello %s! Welcome to mutant programming language!\n", user.Username)
	fmt.Printf("Please get started by using this REPL")
	repl.Start(os.Stdin, os.Stdout)
}
