package lua

import (
	"fmt"

	"mutant/object"

	lua "github.com/Shopify/go-lua"
)

// APIContext holds Mutant runtime data passed to Lua environment.
// This is the only way Lua patches can interact with Mutant runtime.
type APIContext struct {
	// Globals map holds read-only access to Mutant global variables
	Globals map[string]object.Object

	// BuiltinCapabilities describes which builtins the patch can access
	BuiltinCapabilities []string

	// PatchName is the name of the executing patch (for logging/debugging)
	PatchName string
}

// RegisterMutantAPI installs the Mutant-specific API into the Lua environment.
// This is the only place where Mutant runtime data is exposed to Lua.
func RegisterMutantAPI(vm *SandboxedVM, ctx *APIContext) error {
	if vm == nil || ctx == nil {
		return fmt.Errorf("vm and context cannot be nil")
	}

	state := vm.GetState()
	if state == nil {
		return fmt.Errorf("sandbox not initialized")
	}

	// Create "mutant" table in Lua
	state.NewTable()

	// Register mutant.get_global(name) - read-only access to Mutant globals
	state.PushGoFunction(func(l *lua.State) int {
		name := lua.CheckString(l, 1)
		if obj, ok := ctx.Globals[name]; ok {
			// Convert object to Lua value
			luaValue := objectToLua(l, obj)
			if luaValue {
				return 1
			}
		}
		l.PushNil()
		return 1
	})
	state.SetField(-2, "get_global")

	// Register mutant.patch_name() - returns current patch name
	state.PushGoFunction(func(l *lua.State) int {
		l.PushString(ctx.PatchName)
		return 1
	})
	state.SetField(-2, "patch_name")

	// Register mutant.can_use_builtin(name) - check if patch has capability
	state.PushGoFunction(func(l *lua.State) int {
		name := lua.CheckString(l, 1)
		hasCapability := false
		for _, cap := range ctx.BuiltinCapabilities {
			if cap == name {
				hasCapability = true
				break
			}
		}
		l.PushBoolean(hasCapability)
		return 1
	})
	state.SetField(-2, "can_use_builtin")

	// Register mutant.version() - returns Mutant version for compatibility checks
	state.PushGoFunction(func(l *lua.State) int {
		l.PushString("2.1.0")
		return 1
	})
	state.SetField(-2, "version")

	// Set mutant table as global
	state.SetGlobal("mutant")

	return nil
}

// objectToLua converts a Mutant object.Object to a Lua value on the stack.
// Returns true if conversion succeeded, false otherwise.
func objectToLua(l *lua.State, obj object.Object) bool {
	if obj == nil {
		l.PushNil()
		return true
	}

	switch obj.Type() {
	case object.INTEGER_OBJ:
		intObj := obj.(*object.Integer)
		l.PushInteger(int(intObj.Value))
		return true

	case object.FLOAT_OBJ:
		floatObj := obj.(*object.Float)
		l.PushNumber(floatObj.Value)
		return true

	case object.BOOLEAN_OBJ:
		boolObj := obj.(*object.Boolean)
		l.PushBoolean(boolObj.Value)
		return true

	case object.STRING_OBJ:
		strObj := obj.(*object.String)
		l.PushString(strObj.Value)
		return true

	case object.NULL_OBJ:
		l.PushNil()
		return true

	case object.ARRAY_OBJ:
		arrObj := obj.(*object.Array)
		l.NewTable()
		for i, elem := range arrObj.Elements {
			l.PushInteger(i + 1) // Lua arrays are 1-indexed
			if !objectToLua(l, elem) {
				l.PushNil()
			}
			l.SetTable(-3)
		}
		return true

	case object.HASH_OBJ:
		hashObj := obj.(*object.Hash)
		l.NewTable()
		for _, pair := range hashObj.Pairs {
			// Push key
			if !objectToLua(l, pair.Key) {
				l.PushNil()
			}
			// Push value
			if !objectToLua(l, pair.Value) {
				l.PushNil()
			}
			l.SetTable(-3)
		}
		return true

	case object.STRUCT_OBJ:
		structObj := obj.(*object.Struct)
		l.NewTable()
		for name, value := range structObj.Fields {
			l.PushString(name)
			if !objectToLua(l, value) {
				l.PushNil()
			}
			l.SetTable(-3)
		}
		return true

	case object.ENUM_VALUE_OBJ:
		enumObj := obj.(*object.EnumValue)
		l.NewTable()
		l.PushString("_type_name")
		l.PushString(enumObj.TypeName)
		l.SetTable(-3)
		l.PushString("_tag")
		l.PushString(enumObj.Tag)
		l.SetTable(-3)
		if enumObj.Value != nil {
			l.PushString("_value")
			objectToLua(l, enumObj.Value)
			l.SetTable(-3)
		}
		return true

	default:
		// Unsupported types (functions, closures, etc.) cannot be converted
		return false
	}
}

// luaToObject converts a Lua value to a Mutant object.Object.
// Only safe conversions are performed; complex types are rejected.
func luaToObject(l *lua.State, index int) object.Object {
	switch l.TypeOf(index) {
	case lua.TypeNil:
		return &object.Null{}

	case lua.TypeBoolean:
		return &object.Boolean{Value: l.ToBoolean(index)}

	case lua.TypeNumber:
		if n, ok := l.ToInteger(index); ok {
			return &object.Integer{Value: int64(n)}
		}
		if f, ok := l.ToNumber(index); ok {
			return &object.Float{Value: f}
		}
		return &object.Null{}

	case lua.TypeString:
		if s, ok := l.ToString(index); ok {
			return &object.String{Value: s}
		}
		return &object.Null{}

	case lua.TypeTable:
		// Convert table to either Array or Hash depending on structure
		// For now, convert to Hash for safety
		hashObj := &object.Hash{Pairs: make(map[object.HashKey]object.HashPair)}

		absIndex := l.AbsIndex(index)
		l.PushNil()
		for l.Next(absIndex) {
			// Stack: table, key, value
			key := luaToObject(l, -2)
			value := luaToObject(l, -1)

			hashKey, ok := key.(object.Hashable)
			if ok {
				pair := object.HashPair{Key: key, Value: value}
				hashObj.Pairs[hashKey.HashKey()] = pair
			}
			l.Pop(1) // Remove value, keep key for next iteration
		}
		return hashObj

	default:
		// Unsupported types return nil
		return &object.Null{}
	}
}
