package vm

import (
	"crypto/sha256"
	"fmt"
	"math"
	"mutant/builtin"
	"mutant/code"
	"mutant/compiler"
	"mutant/global"
	"mutant/mutil"
	"mutant/object"
	"mutant/security"
	"os"
)

// VM structure defines virtual machine
type VM struct {
	constants       []object.Object
	stack           []object.Object
	stackPointer    int // top of stack is stack[stackPointer-1]
	globals         []object.Object
	frames          []*Frame
	frameIndex      int
	inslen          int
	password        string // password for instruction decryption
	stepCount       uint64
	integrityEvery  uint64
	integrityJitter uint64
	frameIntegrity  map[*object.CompiledFunction][32]byte
	secureMode      bool

	enforceSecurityCheckOpcodes bool
}

var (
	isDebuggerPresent  = security.IsDebuggerPresent
	isSandboxed        = security.IsSandboxed
	logSecurityWarning = func(event, stage string) {
		fmt.Fprintf(os.Stderr, "[security] event=%s stage=%s action=warn\n", event, stage)
	}
)

const (
	initialStackCapacity   = global.StackSize
	initialGlobalsCapacity = global.GlobalSize
	initialFrameCapacity   = global.MaxFrames
)

func New(bc *compiler.ByteCode) *VM {
	mainfn := &object.CompiledFunction{Instructions: bc.Instructions}
	frames := make([]*Frame, initialFrameCapacity)

	mainClosure := &object.Closure{Fn: mainfn}
	mainFrame := NewFrame(mainClosure, 0)
	frames[0] = mainFrame

	frameIntegrity := make(map[*object.CompiledFunction][32]byte)
	frameIntegrity[mainfn] = sha256.Sum256(mainfn.Instructions)

	return &VM{
		constants:       bc.Constants,
		stack:           make([]object.Object, initialStackCapacity),
		stackPointer:    0,
		globals:         make([]object.Object, initialGlobalsCapacity),
		frames:          frames,
		frameIndex:      1,
		inslen:          len(bc.Instructions),
		password:        "",
		stepCount:       0,
		integrityEvery:  64,
		integrityJitter: uint64(len(bc.Instructions)%31 + 7),
		frameIntegrity:  frameIntegrity,
		secureMode:      true,

		enforceSecurityCheckOpcodes: false,
	}
}

func growSize(current, required, fallback int) int {
	if required <= current {
		return current
	}

	newSize := current
	if newSize == 0 {
		newSize = fallback
	}
	if newSize == 0 {
		newSize = 1
	}

	for newSize < required {
		newSize *= 2
	}

	return newSize
}

func (vm *VM) ensureStackCapacity(required int) {
	if required <= len(vm.stack) {
		return
	}

	newSize := growSize(len(vm.stack), required, initialStackCapacity)
	resized := make([]object.Object, newSize)
	copy(resized, vm.stack)
	vm.stack = resized
}

func (vm *VM) ensureGlobalCapacity(index int) {
	required := index + 1
	if required <= len(vm.globals) {
		return
	}

	newSize := growSize(len(vm.globals), required, initialGlobalsCapacity)
	resized := make([]object.Object, newSize)
	copy(resized, vm.globals)
	vm.globals = resized
}

func (vm *VM) ensureFrameCapacity(required int) {
	if required <= len(vm.frames) {
		return
	}

	newSize := growSize(len(vm.frames), required, initialFrameCapacity)
	resized := make([]*Frame, newSize)
	copy(resized, vm.frames)
	vm.frames = resized
}

func (vm *VM) encryptForStorage(obj object.Object) object.Object {
	encObj, err := mutil.EncryptObject(obj, vm.inslen, vm.password)
	if err == nil {
		return encObj
	}
	return obj
}

func (vm *VM) decryptForUse(obj object.Object) object.Object {
	decObj, err := mutil.DecryptObject(obj, vm.inslen, vm.password)
	if err == nil {
		return decObj
	}
	return obj
}

func (vm *VM) clearObjectSensitiveData(obj object.Object) {
	if obj == nil {
		return
	}

	switch o := obj.(type) {
	case *object.Encrypted:
		security.SecureZero(o.Value)
		o.Value = nil
	case *object.Array:
		for i := range o.Elements {
			vm.clearObjectSensitiveData(o.Elements[i])
			o.Elements[i] = nil
		}
	case *object.Hash:
		for key, pair := range o.Pairs {
			vm.clearObjectSensitiveData(pair.Key)
			vm.clearObjectSensitiveData(pair.Value)
			delete(o.Pairs, key)
		}
	case *object.Closure:
		for i := range o.Free {
			vm.clearObjectSensitiveData(o.Free[i])
			o.Free[i] = nil
		}
	}
}

// CleanupSensitiveData clears encrypted runtime data buffers after execution.
// If clearGlobals is false, globals are preserved (useful for REPL stateful sessions).
func (vm *VM) CleanupSensitiveData(clearGlobals bool) {
	for i := range vm.stack {
		vm.clearObjectSensitiveData(vm.stack[i])
		vm.stack[i] = nil
	}
	vm.stackPointer = 0

	if clearGlobals {
		for i := range vm.globals {
			vm.clearObjectSensitiveData(vm.globals[i])
			vm.globals[i] = nil
		}
	}

	for i := range vm.constants {
		if compiledFn, ok := vm.constants[i].(*object.CompiledFunction); ok {
			security.SecureZero(compiledFn.Instructions)
			compiledFn.Instructions = nil
		}
		vm.clearObjectSensitiveData(vm.constants[i])
		vm.constants[i] = nil
	}

	for i := range vm.frames {
		vm.frames[i] = nil
	}
	vm.frameIndex = 0
	vm.password = ""
}

func NewWithPassword(bc *compiler.ByteCode, password string) *VM {
	return NewWithPasswordMode(bc, password, true)
}

func NewWithPasswordMode(bc *compiler.ByteCode, password string, secureMode bool) *VM {
	vm := New(bc)
	vm.password = password
	vm.secureMode = secureMode
	vm.enforceSecurityCheckOpcodes = true
	return vm
}

func NewWithGlobalStore(bc *compiler.ByteCode, globals []object.Object) *VM {
	return NewWithGlobalStoreMode(bc, globals, true)
}

func NewWithGlobalStoreAndPassword(bc *compiler.ByteCode, globals []object.Object, password string) *VM {
	vm := NewWithGlobalStore(bc, globals)
	vm.password = password
	return vm
}

func NewWithPasswordAndGlobalStore(bc *compiler.ByteCode, password string, globals []object.Object) *VM {
	return NewWithPasswordAndGlobalStoreMode(bc, password, globals, true)
}

func NewWithGlobalStoreMode(bc *compiler.ByteCode, globals []object.Object, secureMode bool) *VM {
	vm := New(bc)
	if globals != nil {
		vm.globals = globals
	}
	vm.secureMode = secureMode
	return vm
}

func NewWithPasswordAndGlobalStoreMode(bc *compiler.ByteCode, password string, globals []object.Object, secureMode bool) *VM {
	vm := NewWithPasswordMode(bc, password, secureMode)
	if globals != nil {
		vm.globals = globals
	}
	return vm
}

func (vm *VM) Run() error {
	var ip int
	var ins code.Instructions
	var op code.Opcode

	if err := vm.validateSecurityCheckOpcodes("before-execution"); err != nil {
		return err
	}

	for vm.currentFrame().ip < len(vm.currentFrame().Instructions())-1 {
		if err := vm.runIntegrityProbes(); err != nil {
			return err
		}

		vm.currentFrame().ip++
		vm.stepCount++

		ip = vm.currentFrame().ip
		ins = vm.currentFrame().Instructions()

		opcodeByte, err := security.SecureXOROneAt(ins[ip], int64(vm.inslen), vm.password, int64(ip))
		if err != nil {
			return err
		}
		op = code.Opcode(opcodeByte)

		switch op {
		case code.OpChkDbg:
			if isDebuggerPresent() {
				security.RecordDebuggerDetected("vm-run")
				if !vm.secureMode {
					logSecurityWarning("debugger_detected", "vm-run")
					continue
				}
				return security.ErrDebuggerDetected
			}
		case code.OpChkSnd:
			if isSandboxed() {
				security.RecordSandboxDetected("vm-run")
				if !vm.secureMode {
					logSecurityWarning("sandbox_detected", "vm-run")
					continue
				}
				return security.ErrSandboxDetected
			}
		case code.OpConstant:
			if ip+2 >= len(ins) {
				return fmt.Errorf("OpConstant: not enough bytes for operand at ip=%d, len=%d", ip, len(ins))
			}
			constIndex, err := code.ReadUint16(ins[ip+1:], int64(vm.inslen), vm.password, int64(ip+1))
			if err != nil {
				return err
			}
			vm.currentFrame().ip += 2

			if err := vm.push(vm.constants[constIndex]); err != nil {
				return err
			}
		case code.OpBang:
			if err := vm.execBangOperation(); err != nil {
				return err
			}
		case code.OpMinus:
			if err := vm.execMinusOperation(); err != nil {
				return err
			}
		case code.OpAdd, code.OpSub, code.OpMul, code.OpDiv:
			if err := vm.execBinaryOperation(op); err != nil {
				return err
			}
		case code.OpTrue:
			if err := vm.push(global.True); err != nil {
				return err
			}
		case code.OpFalse:
			if err := vm.push(global.False); err != nil {
				return err
			}
		case code.OpArray:
			if ip+2 >= len(ins) {
				return fmt.Errorf("OpArray: not enough bytes for operand at ip=%d, len=%d", ip, len(ins))
			}
			res, err := code.ReadUint16(ins[ip+1:], int64(vm.inslen), vm.password, int64(ip+1))
			if err != nil {
				return err
			}
			numElements := int(res)
			vm.currentFrame().ip += 2
			array := vm.buildArray(vm.stackPointer-numElements, vm.stackPointer)
			if err := vm.push(array); err != nil {
				return err
			}
		case code.OpHash:
			if ip+2 >= len(ins) {
				return fmt.Errorf("OpHash: not enough bytes for operand at ip=%d, len=%d", ip, len(ins))
			}
			res, err := code.ReadUint16(ins[ip+1:], int64(vm.inslen), vm.password, int64(ip+1))
			if err != nil {
				return err
			}
			numElements := int(res)
			vm.currentFrame().ip += 2
			hash, err := vm.buildHash(vm.stackPointer-numElements, vm.stackPointer)
			if err != nil {
				return err
			}
			vm.stackPointer = vm.stackPointer - numElements
			if err := vm.push(hash); err != nil {
				return err
			}
		case code.OpEqual, code.OpUnEqual, code.OpGreater:
			if err := vm.execComparison(op); err != nil {
				return err
			}
		case code.OpJump:
			if ip+2 >= len(ins) {
				return fmt.Errorf("OpJump: not enough bytes for operand at ip=%d, len=%d", ip, len(ins))
			}
			res, err := code.ReadUint16(ins[ip+1:], int64(vm.inslen), vm.password, int64(ip+1))
			if err != nil {
				return err
			}
			pos := int(res)
			vm.currentFrame().ip = pos - 1
		case code.OpJumpFalse:
			if ip+2 >= len(ins) {
				return fmt.Errorf("OpJumpFalse: not enough bytes for operand at ip=%d, len=%d", ip, len(ins))
			}
			res, err := code.ReadUint16(ins[ip+1:], int64(vm.inslen), vm.password, int64(ip+1))
			if err != nil {
				return err
			}
			pos := int(res)
			vm.currentFrame().ip += 2
			condition := vm.pop()
			if !isTruthy(condition) {
				vm.currentFrame().ip = pos - 1
			}
		case code.OpSetGlobal:
			if ip+2 >= len(ins) {
				return fmt.Errorf("OpSetGlobal: not enough bytes for operand at ip=%d, len=%d", ip, len(ins))
			}
			globalIndex, err := code.ReadUint16(ins[ip+1:], int64(vm.inslen), vm.password, int64(ip+1))
			if err != nil {
				return err
			}
			vm.currentFrame().ip += 2
			vm.ensureGlobalCapacity(int(globalIndex))
			vm.globals[globalIndex] = vm.encryptForStorage(vm.pop())
		case code.OpGetGlobal:
			if ip+2 >= len(ins) {
				return fmt.Errorf("OpGetGlobal: not enough bytes for operand at ip=%d, len=%d", ip, len(ins))
			}
			globalIndex, err := code.ReadUint16(ins[ip+1:], int64(vm.inslen), vm.password, int64(ip+1))
			if err != nil {
				return err
			}
			vm.currentFrame().ip += 2
			vm.ensureGlobalCapacity(int(globalIndex))
			if err := vm.push(vm.decryptForUse(vm.globals[globalIndex])); err != nil {
				return err
			}
		case code.OpSetLocal:
			if ip+1 >= len(ins) {
				return fmt.Errorf("OpSetLocal: not enough bytes for operand at ip=%d, len=%d", ip, len(ins))
			}
			localIndex, err := code.ReadUint8(ins[ip+1:], int64(vm.inslen), vm.password, int64(ip+1))
			if err != nil {
				return err
			}
			vm.currentFrame().ip++
			frame := vm.currentFrame()
			obj := vm.pop()
			vm.stack[frame.bp+int(localIndex)] = vm.encryptForStorage(obj)
		case code.OpGetLocal:
			if ip+1 >= len(ins) {
				return fmt.Errorf("OpGetLocal: not enough bytes for operand at ip=%d, len=%d", ip, len(ins))
			}
			localIndex, err := code.ReadUint8(ins[ip+1:], int64(vm.inslen), vm.password, int64(ip+1))
			if err != nil {
				return err
			}
			vm.currentFrame().ip++
			frame := vm.currentFrame()
			if err := vm.push(vm.decryptForUse(vm.stack[frame.bp+int(localIndex)])); err != nil {
				return err
			}
		case code.OpGetBuiltin:
			if ip+1 >= len(ins) {
				return fmt.Errorf("OpGetBuiltin: not enough bytes for operand at ip=%d, len=%d", ip, len(ins))
			}
			builtinIndex, err := code.ReadUint8(ins[ip+1:], int64(vm.inslen), vm.password, int64(ip+1))
			if err != nil {
				return err
			}
			vm.currentFrame().ip++
			if int(builtinIndex) >= len(builtin.Builtins) {
				return fmt.Errorf("OpGetBuiltin: invalid builtin index=%d, len=%d", builtinIndex, len(builtin.Builtins))
			}
			definition := builtin.Builtins[builtinIndex]
			if err := vm.push(definition.Builtin); err != nil {
				return err
			}
		case code.OpGetFree:
			if ip+1 >= len(ins) {
				return fmt.Errorf("OpGetFree: not enough bytes for operand at ip=%d, len=%d", ip, len(ins))
			}
			freeIndex, err := code.ReadUint8(ins[ip+1:], int64(vm.inslen), vm.password, int64(ip+1))
			if err != nil {
				return err
			}
			vm.currentFrame().ip++
			currentClosure := vm.currentFrame().cl
			if err := vm.push(vm.decryptForUse(currentClosure.Free[freeIndex])); err != nil {
				return err
			}
		case code.OpIndex:
			index := vm.pop()
			left := vm.pop()
			if err := vm.execIndexOperation(left, index); err != nil {
				return err
			}
		case code.OpClosure:
			if ip+3 >= len(ins) {
				return fmt.Errorf("OpClosure: not enough bytes for operands at ip=%d, len=%d", ip, len(ins))
			}
			constIndex, err := code.ReadUint16(ins[ip+1:], int64(vm.inslen), vm.password, int64(ip+1))
			if err != nil {
				return err
			}
			numFree, err := code.ReadUint8(ins[ip+3:], int64(vm.inslen), vm.password, int64(ip+3))
			if err != nil {
				return err
			}
			vm.currentFrame().ip += 3
			if err := vm.pushClosure(int(constIndex), int(numFree)); err != nil {
				return err
			}
		case code.OpCurrentClosure:
			currentClosure := vm.currentFrame().cl
			if err := vm.push(vm.decryptForUse(currentClosure)); err != nil {
				return err
			}
		case code.OpCall:
			if ip+1 >= len(ins) {
				return fmt.Errorf("OpCall: not enough bytes for operand at ip=%d, len=%d", ip, len(ins))
			}
			numArgs, err := code.ReadUint8(ins[ip+1:], int64(vm.inslen), vm.password, int64(ip+1))
			if err != nil {
				return err
			}
			vm.currentFrame().ip++
			if err := vm.execCall(int(numArgs)); err != nil {
				return err
			}
		case code.OpReturnValue:
			returnValue := vm.pop()
			frame := vm.popFrame()
			vm.stackPointer = frame.bp - 1
			if err := vm.push(returnValue); err != nil {
				return err
			}
		case code.OpReturn:
			frame := vm.popFrame()
			vm.stackPointer = frame.bp - 1
			if err := vm.push(global.Null); err != nil {
				return err
			}
		case code.OpNull:
			if err := vm.push(global.Null); err != nil {
				return err
			}
		case code.OpPop:
			vm.pop()
		}
	}

	if err := vm.validateSecurityCheckOpcodes("after-execution"); err != nil {
		return err
	}

	return nil
}

func (vm *VM) validateSecurityCheckOpcodes(stage string) error {
	if !vm.enforceSecurityCheckOpcodes {
		return nil
	}

	foundDbg, foundSnd, err := vm.scanSecurityCheckOpcodes()
	if err != nil {
		return err
	}

	if !foundDbg || !foundSnd {
		return fmt.Errorf("required security check opcodes missing %s: OpChkDbg=%t OpChkSnd=%t", stage, foundDbg, foundSnd)
	}

	return nil
}

func (vm *VM) scanSecurityCheckOpcodes() (bool, bool, error) {
	foundDbg := false
	foundSnd := false

	mainFoundDbg, mainFoundSnd, err := vm.scanInstructionsForSecurityCheckOpcodes(vm.currentFrame().Instructions())
	if err != nil {
		return false, false, err
	}
	foundDbg = foundDbg || mainFoundDbg
	foundSnd = foundSnd || mainFoundSnd

	for _, constant := range vm.constants {
		compiledFn, ok := constant.(*object.CompiledFunction)
		if !ok {
			continue
		}

		fnFoundDbg, fnFoundSnd, scanErr := vm.scanInstructionsForSecurityCheckOpcodes(compiledFn.Instructions)
		if scanErr != nil {
			return false, false, scanErr
		}
		foundDbg = foundDbg || fnFoundDbg
		foundSnd = foundSnd || fnFoundSnd
	}

	return foundDbg, foundSnd, nil
}

func (vm *VM) scanInstructionsForSecurityCheckOpcodes(ins code.Instructions) (bool, bool, error) {
	foundDbg := false
	foundSnd := false

	for i := 0; i < len(ins); {
		opcodeByte, err := security.SecureXOROneAt(ins[i], int64(vm.inslen), vm.password, int64(i))
		if err != nil {
			return false, false, err
		}

		op := code.Opcode(opcodeByte)
		if op == code.OpChkDbg {
			foundDbg = true
		}
		if op == code.OpChkSnd {
			foundSnd = true
		}

		def, err := code.Lookup(byte(op))
		if err != nil {
			return false, false, err
		}

		next := 1
		for _, width := range def.OperandWidths {
			next += width
		}
		i += next
	}

	return foundDbg, foundSnd, nil
}

func (vm *VM) runIntegrityProbes() error {
	if vm.integrityEvery == 0 {
		return nil
	}

	if vm.stepCount%vm.integrityEvery == 0 || vm.stepCount%97 == vm.integrityJitter%97 {
		if err := vm.verifyCurrentFrameIntegrity(); err != nil {
			return err
		}
	}

	if vm.stepCount > 0 && vm.stepCount%251 == 0 {
		if err := vm.verifyActiveFramesIntegrity(); err != nil {
			return err
		}
	}

	return nil
}

func (vm *VM) StackTop() object.Object {
	if vm.stackPointer == 0 {
		return nil
	}

	return vm.decryptForUse(vm.stack[vm.stackPointer-1])
}

func (vm *VM) LastPoppedStackElement() object.Object {
	return vm.decryptForUse(vm.stack[vm.stackPointer])
}

func (vm *VM) pushClosure(constIndex, numFree int) error {
	constant := vm.constants[constIndex]
	fun, ok := constant.(*object.CompiledFunction)
	if !ok {
		return fmt.Errorf("not a function: %+v", constant)
	}

	free := make([]object.Object, numFree)
	for i := 0; i < numFree; i++ {
		free[i] = vm.stack[vm.stackPointer-numFree+i]
	}
	vm.stackPointer = vm.stackPointer - numFree

	closure := &object.Closure{Fn: fun, Free: free}
	return vm.push(closure)
}

func (vm *VM) push(obj object.Object) error {
	vm.ensureStackCapacity(vm.stackPointer + 1)
	obj = vm.encryptForStorage(obj)

	vm.stack[vm.stackPointer] = obj
	vm.stackPointer++

	return nil
}

func (vm *VM) pop() object.Object {
	obj := vm.decryptForUse(vm.stack[vm.stackPointer-1])

	vm.stackPointer--
	return obj
}

func (vm *VM) execBinaryOperation(op code.Opcode) error {
	right := vm.pop()
	left := vm.pop()

	rtype := right.Type()
	ltype := left.Type()

	if rtype == object.INTEGER_OBJ && ltype == object.INTEGER_OBJ {
		return vm.execBinaryIntegerOperation(op, left, right)
	}

	switch {
	case rtype == object.INTEGER_OBJ && ltype == object.INTEGER_OBJ:
		return vm.execBinaryIntegerOperation(op, left, right)
	case rtype == object.STRING_OBJ && ltype == object.STRING_OBJ:
		return vm.execBinaryStringOperation(op, left, right)
	}

	ans1 := mutil.AssertObjectTypes(string(rtype), object.INTEGER_OBJ, object.FLOAT_OBJ)
	ans2 := mutil.AssertObjectTypes(string(ltype), object.INTEGER_OBJ, object.FLOAT_OBJ)
	if ans1 && ans2 {
		return vm.execBinaryFloatOperation(op, left, right)
	}

	return fmt.Errorf("Unsupported types for binary operation: %s, %s", ltype, rtype)
}

func (vm *VM) execBinaryIntegerOperation(op code.Opcode, left, right object.Object) error {
	rval := right.(*object.Integer).Value
	lval := left.(*object.Integer).Value
	var result int64

	switch op {
	case code.OpAdd:
		result = lval + rval
	case code.OpSub:
		result = lval - rval
	case code.OpMul:
		result = lval * rval
	case code.OpDiv:
		result = lval / rval
	default:
		return fmt.Errorf("Unknown integer operator: %d", op)
	}

	return vm.push(&object.Integer{Value: result})
}

func getFloatVal(obj object.Object) float64 {
	if obj.Type() == object.INTEGER_OBJ {
		val := obj.(*object.Integer).Value
		return float64(val)
	}
	return obj.(*object.Float).Value
}
func (vm *VM) execBinaryFloatOperation(op code.Opcode, left, right object.Object) error {
	rval := getFloatVal(right)
	lval := getFloatVal(left)
	var result float64

	switch op {
	case code.OpAdd:
		result = lval + rval
	case code.OpSub:
		result = lval - rval
	case code.OpMul:
		result = lval * rval
	case code.OpDiv:
		result = lval / rval
	case code.OpMod:
		result = math.Mod(lval, rval)
	default:
		return fmt.Errorf("Unknown float operator: %d", op)
	}

	return vm.push(&object.Float{Value: result})
}

func (vm *VM) execBinaryStringOperation(op code.Opcode, left, right object.Object) error {
	rval := right.(*object.String).Value
	lval := left.(*object.String).Value

	if op != code.OpAdd {
		return fmt.Errorf("Unknown string operator: %d", op)
	}

	return vm.push(&object.String{Value: lval + rval})
}

func (vm *VM) execIndexOperation(left, index object.Object) error {
	switch {
	case left.Type() == object.ARRAY_OBJ && index.Type() == object.INTEGER_OBJ:
		return vm.execArrayIndex(left, index)
	case left.Type() == object.STRING_OBJ && index.Type() == object.INTEGER_OBJ:
		return vm.execStringIndex(left, index)
	case left.Type() == object.HASH_OBJ:
		return vm.execHashIndex(left, index)
	default:
		return fmt.Errorf("index operator not supported: %s", left.Type())
	}
}

func (vm *VM) execStringIndex(str, index object.Object) error {
	strVal := str.(*object.String).Value
	i := index.(*object.Integer).Value
	max := int64(len(strVal) - 1)
	if i > max {
		return vm.push(global.Null)
	} else if i < 0 {
		strObj := &object.String{Value: string(strVal[max+i+1])}
		return vm.push(strObj)
	}
	strObj := &object.String{Value: string(strVal[i])}
	return vm.push(strObj)
}

func (vm *VM) execArrayIndex(array, index object.Object) error {
	arrayObj := array.(*object.Array)
	i := index.(*object.Integer).Value
	max := int64(len(arrayObj.Elements) - 1)
	if i > max {
		return vm.push(global.Null)
	} else if i < 0 {
		return vm.push(arrayObj.Elements[max+i+1])
	}
	return vm.push(arrayObj.Elements[i])
}

func (vm *VM) execHashIndex(hash, index object.Object) error {
	hashObj := hash.(*object.Hash)

	key, ok := index.(object.Hashable)
	if !ok {
		return fmt.Errorf("unusable as hash key: %s", index.Type())
	}

	pair, ok := hashObj.Pairs[key.HashKey()]
	if !ok {
		return vm.push(global.Null)
	}

	return vm.push(pair.Value)
}

func (vm *VM) execBangOperation() error {
	operand := vm.pop()

	switch operand {
	case global.True:
		return vm.push(global.False)
	case global.False:
		return vm.push(global.True)
	case global.Null:
		return vm.push(global.True)
	default:
		return vm.push(global.False)
	}
}

func (vm *VM) execMinusOperation() error {
	operand := vm.pop()
	assertion := mutil.AssertObjectTypes(string(operand.Type()), object.INTEGER_OBJ, object.FLOAT_OBJ)
	if !assertion {
		return fmt.Errorf("unsupported object type for negation: %s", operand.Type())
	}

	switch operand.Type() {
	case object.INTEGER_OBJ:
		value := operand.(*object.Integer).Value
		return vm.push(&object.Integer{Value: -value})
	case object.FLOAT_OBJ:
		value := operand.(*object.Float).Value
		return vm.push(&object.Float{Value: -value})
	}

	return fmt.Errorf("unknown object: %s", operand.Type())
}

func (vm *VM) execComparison(op code.Opcode) error {
	right := vm.pop()
	left := vm.pop()

	if left.Type() == object.INTEGER_OBJ && right.Type() == object.INTEGER_OBJ {
		return vm.execIntegerComparison(op, left, right)
	}

	rtype := right.Type()
	ltype := left.Type()
	ans1 := mutil.AssertObjectTypes(string(rtype), object.INTEGER_OBJ, object.FLOAT_OBJ)
	ans2 := mutil.AssertObjectTypes(string(ltype), object.INTEGER_OBJ, object.FLOAT_OBJ)
	if ans1 && ans2 {
		return vm.execFloatComparison(op, left, right)
	}

	switch op {
	case code.OpEqual:
		return vm.push(nativeBoolToBooleanObject(right.Inspect() == left.Inspect()))
	case code.OpUnEqual:
		return vm.push(nativeBoolToBooleanObject(right.Inspect() != left.Inspect()))
	default:
		return fmt.Errorf("unknown operator: %d (%s %s)", op, left.Type(), right.Type())
	}
}

func (vm *VM) execFloatComparison(op code.Opcode, left, right object.Object) error {
	leftValue := getFloatVal(left)
	rightValue := getFloatVal(right)
	switch op {
	case code.OpEqual:
		return vm.push(nativeBoolToBooleanObject(rightValue == leftValue))
	case code.OpUnEqual:
		return vm.push(nativeBoolToBooleanObject(rightValue != leftValue))
	case code.OpGreater:
		return vm.push(nativeBoolToBooleanObject(leftValue > rightValue))
	default:
		return fmt.Errorf("unknown operator: %d", op)
	}
}
func (vm *VM) execIntegerComparison(op code.Opcode, left, right object.Object) error {
	leftValue := left.(*object.Integer).Value
	rightValue := right.(*object.Integer).Value
	switch op {
	case code.OpEqual:
		return vm.push(nativeBoolToBooleanObject(rightValue == leftValue))
	case code.OpUnEqual:
		return vm.push(nativeBoolToBooleanObject(rightValue != leftValue))
	case code.OpGreater:
		return vm.push(nativeBoolToBooleanObject(leftValue > rightValue))
	default:
		return fmt.Errorf("unknown operator: %d", op)
	}
}

func (vm *VM) buildArray(startIndex, endIndex int) object.Object {
	elements := make([]object.Object, endIndex-startIndex)
	for i := startIndex; i < endIndex; i++ {
		elements[i-startIndex] = vm.decryptForUse(vm.stack[i])
	}
	return &object.Array{Elements: elements}
}

func (vm *VM) buildHash(startIndex, endIndex int) (object.Object, error) {
	hashedPairs := make(map[object.HashKey]object.HashPair)
	for i := startIndex; i < endIndex; i += 2 {
		key := vm.decryptForUse(vm.stack[i])
		value := vm.decryptForUse(vm.stack[i+1])

		pair := object.HashPair{Key: key, Value: value}
		hashKey, ok := key.(object.Hashable)
		if !ok {
			return nil, fmt.Errorf("unusable as a hashkey: %s", key.Type())
		}
		hashedPairs[hashKey.HashKey()] = pair
	}
	return &object.Hash{Pairs: hashedPairs}, nil
}

func (vm *VM) currentFrame() *Frame { return vm.frames[vm.frameIndex-1] }
func (vm *VM) pushFrame(f *Frame) {
	vm.ensureFrameCapacity(vm.frameIndex + 1)
	vm.frames[vm.frameIndex] = f
	vm.frameIndex++
	if f != nil && f.cl != nil && f.cl.Fn != nil {
		vm.registerFrameIntegrity(f.cl.Fn)
	}
}
func (vm *VM) popFrame() *Frame {
	vm.frameIndex--
	return vm.frames[vm.frameIndex]
}

func (vm *VM) execCall(numArgs int) error {
	var callee object.Object
	if vm.stack[vm.stackPointer-1-numArgs].Type() == object.CLOSURE_OBJ || vm.stack[vm.stackPointer-1-numArgs].Type() == object.BUILTIN_OBJ {
		callee = vm.stack[vm.stackPointer-1-numArgs]
	} else {
		callee = vm.stack[0]
	}

	switch calleeType := callee.(type) {
	case *object.Closure:
		return vm.callClosure(calleeType, numArgs)
	case *builtin.BuiltIn:
		return vm.callBuiltin(calleeType, numArgs)

	default:
		return fmt.Errorf("calling non-function and non-built-in")
	}
}

func (vm *VM) callClosure(cl *object.Closure, numArgs int) error {
	if numArgs != cl.Fn.NumParams {
		return fmt.Errorf("wrong number of arguments. want=%d, got=%d", cl.Fn.NumParams, numArgs)
	}

	frame := NewFrame(cl, vm.stackPointer-numArgs)
	vm.pushFrame(frame)
	vm.ensureStackCapacity(frame.bp + cl.Fn.NumLocals)
	vm.stackPointer = frame.bp + cl.Fn.NumLocals
	return nil
}

func (vm *VM) registerFrameIntegrity(fn *object.CompiledFunction) {
	if vm.frameIntegrity == nil {
		vm.frameIntegrity = make(map[*object.CompiledFunction][32]byte)
	}
	if _, exists := vm.frameIntegrity[fn]; !exists {
		vm.frameIntegrity[fn] = sha256.Sum256(fn.Instructions)
	}
}

func (vm *VM) verifyCurrentFrameIntegrity() error {
	frame := vm.currentFrame()
	return vm.verifyFrameIntegrity(frame, "vm-frame")
}

func (vm *VM) verifyActiveFramesIntegrity() error {
	for i := 0; i < vm.frameIndex; i++ {
		if err := vm.verifyFrameIntegrity(vm.frames[i], "vm-frame-sweep"); err != nil {
			return err
		}
	}

	return nil
}

func (vm *VM) verifyFrameIntegrity(frame *Frame, stage string) error {
	if frame == nil || frame.cl == nil || frame.cl.Fn == nil {
		return nil
	}

	fn := frame.cl.Fn
	expected, exists := vm.frameIntegrity[fn]
	if !exists {
		vm.registerFrameIntegrity(fn)
		expected = vm.frameIntegrity[fn]
	}

	actual := sha256.Sum256(fn.Instructions)
	if !security.SecureCompare(expected[:], actual[:]) {
		security.RecordIntegrityFailure(stage)
		return security.ApplyTamperResponse("integrity_failed", stage, true, fmt.Errorf("runtime integrity check failed"))
	}

	return nil
}

func (vm *VM) callBuiltin(builtin *builtin.BuiltIn, numArgs int) error {
	storedArgs := vm.stack[vm.stackPointer-numArgs : vm.stackPointer]
	args := make([]object.Object, len(storedArgs))
	for i, arg := range storedArgs {
		args[i] = vm.decryptForUse(arg)
	}
	result := builtin.Fn(args...)

	vm.stackPointer = vm.stackPointer - numArgs - 1

	if result != nil {
		vm.push(result)
	} else {
		vm.push(global.Null)
	}

	return nil
}

func nativeBoolToBooleanObject(native bool) *object.Boolean {
	if native {
		return global.True
	}
	return global.False
}

func isTruthy(obj object.Object) bool {
	switch obj := obj.(type) {
	case *object.Boolean:
		return obj.Value
	case *object.Null:
		return false
	default:
		return true
	}
}
