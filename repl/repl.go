package repl

import (
	"bufio"
	"fmt"
	"io"
	"mutant/compiler"
	"mutant/lexer"
	"mutant/object"
	"mutant/parser"
	"mutant/vm"
)

// PROMPT is the constant for showing REPL prompt
const PROMPT = ">> "

// Start function is the entrypoint of our repl
func Start(in io.Reader, out io.Writer) {
	scanner := bufio.NewScanner(in)
	// env := object.NewEnvironment()
	// macroEnv := object.NewEnvironment()

	constants := []object.Object{}
	globals := make([]object.Object, vm.GlobalSize)
	symbolTable := compiler.NewSymbolTable()

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
			printParseErrors(out, p.Errors())
			continue
		}

		comp := compiler.NewWithState(symbolTable, constants)
		if err := comp.Compile(program); err != nil {
			printCompilerError(out, err.Error())
			continue
		}

		machine := vm.NewWithGlobalStore(comp.ByteCode(), globals)
		if err := machine.Run(); err != nil {
			printMachineError(out, err.Error())
			continue
		}

		last := machine.LastPoppedStackElement()
		io.WriteString(out, last.Inspect())
		io.WriteString(out, "\n")
	}
}

func printParseErrors(out io.Writer, msgs []string) {
	io.WriteString(out, "\nMutation gone wrong 😕. Below error messages may help!\n\n")
	io.WriteString(out, "parser errors:")
	for _, msg := range msgs {
		io.WriteString(out, "\n\t"+msg+"\t\n")
	}
}

func printCompilerError(out io.Writer, msg string) {
	io.WriteString(out, "\nBytes are small but confusing 😕. Below error messages may help!\n\n")
	io.WriteString(out, "compiler error:")
	io.WriteString(out, "\n\t"+msg+"\t\n")
}

func printMachineError(out io.Writer, msg string) {
	io.WriteString(out, "\nEven machines aren't perfect 😕. Below error messages may help!\n\n")
	io.WriteString(out, "vm error:")
	io.WriteString(out, "\n\t"+msg+"\t\n")
}
