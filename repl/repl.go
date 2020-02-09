package repl

import (
	"bufio"
	"fmt"
	"io"
	"mutant/lexer"
	"mutant/token"
)

// PROMPT is the constant for showing REPL prompt
const PROMPT = ">> "

// Start function is the entrypoint of our repl
func Start(in io.Reader, out io.Writer) {
	scanner := bufio.NewScanner(in)
	fmt.Printf("\n\n%s", PROMPT)
	for {
		scanned := scanner.Scan()
		if !scanned {
			return
		}
		line := scanner.Text()
		l := lexer.New(line)

		for tok := l.NextToken(); tok.Type != token.EOF; tok = l.NextToken() {
			fmt.Printf("%+v\n", tok)
		}
		fmt.Printf(PROMPT)
	}
}
