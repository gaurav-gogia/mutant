package repl

import (
	"bufio"
	"fmt"
	"io"
	"mutant/lexer"
	"mutant/parser"
)

// PROMPT is the constant for showing REPL prompt
const PROMPT = ">> "

// Start function is the entrypoint of our repl
func Start(in io.Reader, out io.Writer) {
	scanner := bufio.NewScanner(in)
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

		io.WriteString(out, program.String())
		io.WriteString(out, "\n")
	}
}

func printParseErrors(out io.Writer, msgs []string) {
	for _, msg := range msgs {
		io.WriteString(out, "\t"+msg+"\t")
	}
}
