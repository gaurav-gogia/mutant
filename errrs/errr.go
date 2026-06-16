package errrs

import "io"

func PrintParseErrors(out io.Writer, msgs []string) {
	io.WriteString(out, "\n"+nextParserComfortMessage()+"\n\n")
	io.WriteString(out, "parser errors:")
	for _, msg := range msgs {
		io.WriteString(out, "\n\t"+msg+"\t\n")
	}
}

func PrintCompilerError(out io.Writer, msg string) {
	io.WriteString(out, "\n"+nextCompilerComfortMessage()+"\n\n")
	io.WriteString(out, "compiler error:")
	io.WriteString(out, "\n\t"+msg+"\t\n")
}

func PrintMachineError(out io.Writer, msg string) {
	io.WriteString(out, "\n"+nextMachineComfortMessage()+"\n\n")
	io.WriteString(out, "vm error:")
	io.WriteString(out, "\n\t"+msg+"\t\n")
}
