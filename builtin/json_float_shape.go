package builtin

import (
	"encoding/json"
	"math"
	"strconv"
)

// jsonFloatsStayFloat makes every finite FLOAT in a value built by
// objectToJSONValue come out of encoding/json as text that json_parse reads
// back as the same FLOAT. It rewrites slices and maps in place and returns the
// result.
//
// json_parse types a number by its literal: 5 is an INTEGER and 5.0 a FLOAT,
// and STRUCTURED_DATA.md promises that a round trip through json_stringify
// keeps that shape. encoding/json writes a float64 the way JavaScript prints a
// number, so a FLOAT with a whole value lost its point: 5.0 came back from
// json_parse as the INTEGER 5, and 1e20 as the STRING "100000000000000000000",
// because no INTEGER holds it (M26-DOC1-004).
//
// encoding/json writes neither a point nor an exponent only for a whole value
// whose magnitude is below 1e21, so those are the only values rewritten: to
// the digits encoding/json would write for them, followed by ".0", and -0.0
// keeps its sign. Every other finite value already has a point or an exponent
// and is written as before, and NaN and the infinities are left for
// encoding/json to refuse, as before. A float64 here can only be a FLOAT:
// objectToJSONValue hands over an INTEGER as an int64.
//
// json_stringify and ndjson_stringify call this, but objectToJSONValue does
// not, because the policy engine hands that function's result to OPA, which
// writes it out with encoding/json and reads the numbers back from that text.
// A FLOAT 5.0 reaches a policy as 5; arriving as 5.0 it would be a number that
// substring refuses as not an integer and that sprintf's %d prints as
// %!d(float64=5).
func jsonFloatsStayFloat(value any) any {
	switch v := value.(type) {
	case float64:
		if v == math.Trunc(v) && math.Abs(v) < 1e21 {
			return json.Number(strconv.FormatFloat(v, 'f', -1, 64) + ".0")
		}
	case []any:
		for i, item := range v {
			v[i] = jsonFloatsStayFloat(item)
		}
	case map[string]any:
		for key, item := range v {
			v[key] = jsonFloatsStayFloat(item)
		}
	}
	return value
}
