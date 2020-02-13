package parser

import (
	"fmt"
	"strings"
)

var traceLevel int = 0

const traceIdentPlaceholder string = "\t"

func identLevel() string {
	return strings.Repeat(traceIdentPlaceholder, traceLevel-1)
}

func tracePrint(fs string) {
	fmt.Printf("%s%s\n", identLevel(), fs)
}

func incIdent() { traceLevel++ }
func decIdent() { traceLevel-- }

func untrace(msg string) {
	tracePrint("END " + msg)
	decIdent()
}

func trace(msg string) string {
	incIdent()
	tracePrint("BEGIN " + msg)
	return msg
}
