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

const BAN1 = `
                    __              __ 
   ____ ___  __  __/ /_____ _____  / /_
  / __ 
  `

const BAN2 = "`"
const BAN3 = `__ \/ / / / __/ __ `
const BAN4 = "`"
const BAN5 = `/ __ \/ __/
 / / / / / / /_/ / /_/ /_/ / / / / /_  
/_/ /_/ /_/\__,_/\__/\__,_/_/ /_/\__/  
                                       
`

const BANNER = BAN1 + BAN2 + BAN3 + BAN4 + BAN5

// Start function is the entrypoint of our repl
func Start(in io.Reader, out io.Writer) {
	scanner := bufio.NewScanner(in)
	io.WriteString(out, BANNER)
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
	io.WriteString(out, "Mutation gone wrong 😕. Below error messages may help!\n\n")
	io.WriteString(out, " parser errors:\n")
	for _, msg := range msgs {
		io.WriteString(out, "\t"+msg+"\t\n")
	}
}
