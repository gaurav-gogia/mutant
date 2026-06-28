package builtin

import (
	"fmt"
	"mutant/object"
	"strconv"
)

func getType(in string) (string, any) {
	if val, err := strconv.ParseInt(in, 10, 64); err == nil {
		return "int", val
	}
	if val, err := strconv.ParseFloat(in, 64); err == nil {
		return "float", val
	}
	if val, err := strconv.ParseBool(in); err == nil {
		return "bool", val
	}
	return "str", in
}

func Gets(args ...object.Object) object.Object {
	if len(args) != 0 {
		return newError("wrong number of arguments. got=%d, want=0", len(args))
	}

	var in string
	_, err := fmt.Scanln(&in)
	if err != nil {
		return newError("something went wrong :/")
	}
	inType, inVal := getType(in)

	switch inType {
	case "bool":
		inVal := inVal.(bool)
		return &object.Boolean{Value: inVal}
	case "int":
		inVal := inVal.(int64)
		return &object.Integer{Value: inVal}
	case "float":
		inVal := inVal.(float64)
		return &object.Float{Value: inVal}
	case "str":
		inVal := inVal.(string)
		return &object.String{Value: inVal}
	default:
		return nil
	}
}
