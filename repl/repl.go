package repl

import (
	"bufio"
	"fmt"
	"io"
	"mutant/compiler"
	"mutant/errrs"
	"mutant/lexer"
	"mutant/object"
	"mutant/parser"
	"mutant/vm"
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

// PROMPT is the constant for showing REPL prompt
const PROMPT = ">> "

// Start function is the entrypoint of our repl
func Start(in io.Reader, out io.Writer) {
	welcome()
	scanner := bufio.NewScanner(in)
	// env := object.NewEnvironment()
	// macroEnv := object.NewEnvironment()

	constants := []object.Object{}
	globals := make([]object.Object, vm.GlobalSize)
	symbolTable := compiler.NewSymbolTable()
	for i, v := range object.Builtins {
		symbolTable.DefineBuiltin(i, v.Name)
	}

	for {
		fmt.Printf("\n\n%s", PROMPT)
		scanned := scanner.Scan()
		if !scanned {
			return
		}

		line := scanner.Text()
		l := lexer.New(line)
		p := parser.New(l)
		program := p.ParseProgram()

		if len(p.Errors()) != 0 {
			errrs.PrintParseErrors(out, p.Errors())
			continue
		}

		comp := compiler.NewWithState(symbolTable, constants)
		if err := comp.Compile(program); err != nil {
			errrs.PrintCompilerError(out, err.Error())
			continue
		}

		machine := vm.NewWithGlobalStore(comp.ByteCode(), globals)
		if err := machine.Run(); err != nil {
			errrs.PrintMachineError(out, err.Error())
			continue
		}

		last := machine.LastPoppedStackElement()
		io.WriteString(out, last.Inspect())
		io.WriteString(out, "\n")
	}
}

func welcome() {
	fmt.Println(banner)

	user, err := user.Current()
	if err != nil {
		panic(err)
	}
	fmt.Printf("Hello %s! Welcome to mutant, a programming language!\n", user.Name)
	fmt.Printf("Please get started by using this REPL")
}
