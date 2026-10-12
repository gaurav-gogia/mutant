package builtin

import (
	"math"
	"strings"
	"testing"

	"mutant/object"
)

// floatShapeStringify is json_stringify's text for value, failing the test on
// an error.
func floatShapeStringify(t *testing.T, value object.Object) string {
	t.Helper()
	payload, errObj := unwrapPair(t, JsonStringify(value))
	if errObj != nil {
		t.Fatalf("json_stringify(%s) failed: %s", value.Inspect(), errObj.Message)
	}
	text, ok := payload.(*object.String)
	if !ok {
		t.Fatalf("json_stringify returned %T, not a STRING", payload)
	}
	return text.Value
}

// floatShapeParse is json_parse's value for text, failing the test on an error.
func floatShapeParse(t *testing.T, text string) object.Object {
	t.Helper()
	payload, errObj := unwrapPair(t, JsonParse(str(text)))
	if errObj != nil {
		t.Fatalf("json_parse(%s) failed: %s", text, errObj.Message)
	}
	return payload
}

// floatShapeWantTypes fails the test unless each element of the array under key
// in hash is of type want.
func floatShapeWantTypes(t *testing.T, hash *object.Hash, key string, want object.ObjectType) {
	t.Helper()
	list, ok := hashValueByKey(hash, key).(*object.Array)
	if !ok {
		t.Fatalf("%q is not an ARRAY", key)
	}
	for i, element := range list.Elements {
		if element.Type() != want {
			t.Errorf("%s[%d] came back as %s %s, want a %s", key, i, element.Type(), element.Inspect(), want)
		}
	}
}

// json_parse types a number by its literal, so json_stringify has to write a
// FLOAT as text that reads back as one. A whole FLOAT lost its point: 5.0 came
// back as the INTEGER 5, and 1e20 as a STRING (M26-DOC1-004). The last three
// rows were read back as FLOAT already, and are written as they always were.
func TestJsonStringifyKeepsAWholeFloatAFloat(t *testing.T) {
	cases := []struct {
		name  string
		value float64
		want  string
	}{
		{"five", 5, "5.0"},
		{"zero", 0, "0.0"},
		{"negative zero keeps its sign", math.Copysign(0, -1), "-0.0"},
		{"negative", -42, "-42.0"},
		{"past 2^53", 9007199254740994, "9007199254740994.0"},
		{"past INTEGER", 1e20, "100000000000000000000.0"},
		{"the largest written without an exponent", math.Nextafter(1e21, 0), "999999999999999900000.0"},
		{"from 1e21 the exponent stays", 1e21, "1e+21"},
		{"a fraction", 2.5, "2.5"},
		{"a fraction below 1e-6", 1e-7, "1e-7"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			text := floatShapeStringify(t, &object.Float{Value: c.value})
			if text != c.want {
				t.Fatalf("json_stringify(%v) = %s, want %s", c.value, text, c.want)
			}
			back := floatShapeParse(t, text)
			got, ok := back.(*object.Float)
			if !ok {
				t.Fatalf("json_parse(%s) = %s %s, want the FLOAT %v", text, back.Type(), back.Inspect(), c.value)
			}
			if math.Float64bits(got.Value) != math.Float64bits(c.value) {
				t.Fatalf("json_parse(%s) = %v, want %v", text, got.Value, c.value)
			}
		})
	}
}

// STRUCTURED_DATA.md: "5 decodes to INTEGER, 5.0 decodes to FLOAT.
// Round-tripping through json_stringify preserves that shape." Its own example
// 6.1 carries bob's score as 42.0. Arrays, hashes and structs are all walked.
func TestJsonRoundTripKeepsEveryNumbersShape(t *testing.T) {
	first := floatShapeParse(t, `{"meta":{"active":false,"score":42.0},"ints":[5,0,-7],"floats":[5.0,0.0,-0.0,0.5]}`)
	text := floatShapeStringify(t, first)
	const want = `{"floats":[5.0,0.0,-0.0,0.5],"ints":[5,0,-7],"meta":{"active":false,"score":42.0}}`
	if text != want {
		t.Fatalf("json_stringify wrote\n%s\nwant\n%s", text, want)
	}

	second, ok := floatShapeParse(t, text).(*object.Hash)
	if !ok {
		t.Fatalf("json_parse(%s) is not a HASH", text)
	}
	floatShapeWantTypes(t, second, "floats", object.FLOAT_OBJ)
	floatShapeWantTypes(t, second, "ints", object.INTEGER_OBJ)
	meta, ok := hashValueByKey(second, "meta").(*object.Hash)
	if !ok {
		t.Fatal(`"meta" is not a HASH`)
	}
	if score := hashValueByKey(meta, "score"); score == nil || score.Type() != object.FLOAT_OBJ {
		t.Errorf("bob's score came back as %v, want a FLOAT", score)
	}

	record := &object.Struct{
		TypeName:   "Record",
		FieldOrder: []string{"score"},
		Fields:     map[string]object.Object{"score": &object.Float{Value: 42}},
	}
	if got := floatShapeStringify(t, record); got != `{"score":42.0}` {
		t.Errorf("a struct's FLOAT field was written as %s, want {\"score\":42.0}", got)
	}
}

// ndjson_parse reads each line as json_parse reads a document, so
// ndjson_stringify has to write each value the way json_stringify does.
func TestNdjsonStringifyKeepsAWholeFloatAFloat(t *testing.T) {
	values := &object.Array{Elements: []object.Object{
		&object.Float{Value: 5},
		makeHashObject(map[string]object.Object{"n": intObj(3), "x": &object.Float{Value: 3}}),
	}}
	text := mustCall(t, NdjsonStringify, values).(*object.String).Value
	if want := "5.0\n{\"n\":3,\"x\":3.0}\n"; text != want {
		t.Fatalf("ndjson_stringify wrote %q, want %q", text, want)
	}

	back := mustArray(t, mustCall(t, NdjsonParse, str(text)))
	if len(back.Elements) != 2 {
		t.Fatalf("ndjson_parse gave back %d values, want 2", len(back.Elements))
	}
	if got := back.Elements[0]; got.Type() != object.FLOAT_OBJ {
		t.Errorf("line 1 came back as %s %s, want a FLOAT", got.Type(), got.Inspect())
	}
	record, ok := back.Elements[1].(*object.Hash)
	if !ok {
		t.Fatalf("line 2 came back as %s, want a HASH", back.Elements[1].Type())
	}
	if x := hashValueByKey(record, "x"); x == nil || x.Type() != object.FLOAT_OBJ {
		t.Errorf("x came back as %v, want a FLOAT", x)
	}
	if n := hashValueByKey(record, "n"); n == nil || n.Type() != object.INTEGER_OBJ {
		t.Errorf("n came back as %v, want an INTEGER", n)
	}
}

// Only a whole FLOAT changes: an INTEGER is still written as its digits, and
// NaN and the infinities, which JSON has no way to write, are still refused.
func TestJsonStringifyFloatShapeLeavesTheRestAlone(t *testing.T) {
	if got := floatShapeStringify(t, intObj(5)); got != "5" {
		t.Errorf("json_stringify(5) = %s, want 5", got)
	}
	for _, value := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		_, errObj := unwrapPair(t, JsonStringify(&object.Float{Value: value}))
		if errObj == nil || !strings.Contains(errObj.Message, "unsupported value") {
			t.Errorf("json_stringify(%v) should be refused as an unsupported value; got %v", value, errObj)
		}
	}
}
