package main

import (
	"fmt"
	"mutant/repl"
	"os"
	"os/user"
)

const banner = `
=======================================
                  _              _   
                 | |            | |  
  _ __ ___  _   _| |_ __ _ _ __ | |_ 
 | '_ ' _ \| | | | __/ _' | '_ \| __|
 | | | | | | |_| | || (_| | | | | |_ 
 |_| |_| |_|\__,_|\__\__,_|_| |_|\__|
                                     
                                     
=======================================
`

func main() {
	user, err := user.Current()
	if err != nil {
		panic(err)
	}
	fmt.Println(banner)
	fmt.Printf("Hello %s! Welcome to mutant, a programming language!\n", user.Username)
	fmt.Printf("Please get started by using this REPL")
	repl.Start(os.Stdin, os.Stdout)
}
