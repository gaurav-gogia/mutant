package builtin

import (
	"reflect"
	"runtime"
	"testing"

	"mutant/object"
)

// An error's context names the builtin that raised it -- "builtin.fs_read" --
// and the language reference tells programs to read it. It used to be worked
// out from the Go function's name, snake-cased with any "Bin" prefix dropped,
// which names a builtin only when the two happen to agree. Where they did not,
// the context named something no program can call: reverse said
// builtin.reverse_array, uuid_v7 said builtin.uui_dv7, bin_is_go said
// builtin.is_go, and the spawn ceiling said builtin.register_task (M26-BLT-017).

// contextNamesErrorIn returns the error a builtin's result carries, bare or as
// the second half of a (value, err) pair.
func contextNamesErrorIn(result object.Object) *object.Error {
	switch value := result.(type) {
	case *object.Error:
		return value
	case *object.MultiValue:
		for _, inner := range value.Values {
			if failure, ok := inner.(*object.Error); ok {
				return failure
			}
		}
	}
	return nil
}

// Every builtin that refuses an argument count its signature forbids is
// probed, the way TestSignatureArityMatchesImplementation probes them: the
// refusal comes from the prologue, so nothing is opened, dialled or killed.
func TestErrorContextNamesEveryBuiltinByItsRegisteredName(t *testing.T) {
	probed := 0
	for _, def := range Builtins {
		name := def.Name
		if _, skipped := arityProbeSkip[name]; skipped {
			continue
		}
		signature, _, _, ok := TeachingDoc(name)
		if !ok {
			continue
		}
		_, params, ok := ParseSignature(signature)
		if !ok {
			continue
		}
		minArgs, maxArgs := SignatureArity(params)
		var illegal []int
		if minArgs > 0 {
			illegal = append(illegal, minArgs-1)
		}
		if maxArgs >= 0 {
			illegal = append(illegal, maxArgs+1)
		}
		for _, n := range illegal {
			result, recovered := callWithNulls(def.Builtin.Fn, n)
			if recovered != nil {
				continue // TestSignatureArityMatchesImplementation reports a panic
			}
			failure := contextNamesErrorIn(result)
			if failure == nil {
				continue // and an accepted count
			}
			probed++
			if want := "builtin." + name; failure.Context != want {
				t.Errorf("%s called with %d arguments: err.context = %q, want %q", name, n, failure.Context, want)
			}
		}
	}
	if probed < 600 {
		t.Fatalf("probed only %d refusals; the registry or its signatures changed shape and this test proves too little", probed)
	}
}

// A builtin may be another one behind a check: db_query is DbQueryNodes after
// an argument count, and net_syn_scan is net_connect_scan under its old name.
// An error raised inside the reused function belongs to the builtin the
// program called.
func TestErrorContextNamesTheBuiltinTheProgramCalledNotTheOneItReuses(t *testing.T) {
	notAHandle := &object.String{Value: "not a handle"}
	_, failure := unwrapPair(t, GetBuiltinByName(BuiltinNameDbQuery).Fn(notAHandle))
	if failure == nil {
		t.Fatal("db_query accepted a STRING handle")
	}
	if want := "builtin." + BuiltinNameDbQuery; failure.Context != want {
		t.Errorf("db_query(STRING): err.context = %q, want %q", failure.Context, want)
	}

	// The host is not a STRING, so both refuse before anything is dialled.
	scanArgs := []object.Object{&object.Integer{Value: 1}, &object.Integer{Value: 1},
		&object.Integer{Value: 2}, &object.Integer{Value: 3}}
	for _, name := range []string{BuiltinNameNetSynScan, BuiltinNameNetConnectScan} {
		_, failure := unwrapPair(t, GetBuiltinByName(name).Fn(scanArgs...))
		if failure == nil {
			t.Fatalf("%s accepted an INTEGER host", name)
		}
		if want := "builtin." + name; failure.Context != want {
			t.Errorf("%s(INTEGER, ...): err.context = %q, want %q", name, failure.Context, want)
		}
	}
}

// contextNamesEngineCall is an argument whose Type method calls db_query the way
// an engine calls a builtin -- from a function outside this package -- while
// the builtin it was handed to is still on the stack beneath, waiting.
type contextNamesEngineCall struct{ raised *object.Error }

func (c *contextNamesEngineCall) Type() object.ObjectType {
	engine := reflect.ValueOf(GetBuiltinByName(BuiltinNameDbQuery).Fn)
	results := engine.Call([]reflect.Value{reflect.ValueOf(&object.String{Value: "not a handle"})})
	c.raised = contextNamesErrorIn(results[0].Interface().(object.Object))
	return object.STRING_OBJ
}

func (c *contextNamesEngineCall) Inspect() string { return "engine call" }

// The error belongs to the builtin the engine called last, not to one further
// out that is merely still running: type_of did not fail, db_query did.
func TestErrorContextStopsAtTheBuiltinAnEngineCalled(t *testing.T) {
	probe := &contextNamesEngineCall{}
	GetBuiltinByName(BuiltinNameTypeOf).Fn(probe)
	if probe.raised == nil {
		t.Fatal("db_query accepted a STRING handle")
	}
	if want := "builtin." + BuiltinNameDbQuery; probe.raised.Context != want {
		t.Errorf("db_query called from inside type_of: err.context = %q, want %q", probe.raised.Context, want)
	}
}

// The engine runs spawn itself and asks this package for a task slot, so the
// ceiling is raised with no registered function on the stack at all.
func TestSpawnCeilingErrorContextNamesSpawn(t *testing.T) {
	taskRegistry.Lock()
	taskRegistry.live += maxLiveTasks
	taskRegistry.Unlock()
	defer func() {
		taskRegistry.Lock()
		taskRegistry.live -= maxLiveTasks
		taskRegistry.Unlock()
	}()

	handle, failure := RegisterTask()
	if failure == nil {
		CompleteTask(handle, nil, nil)
		t.Fatal("RegisterTask handed out a slot above the ceiling")
	}
	if want := "builtin." + BuiltinNameSpawn; failure.Context != want {
		t.Errorf("the spawn ceiling: err.context = %q, want %q", failure.Context, want)
	}
}

// A stack shows which Go function ran, not which registry entry the program
// went through, so two names on one function could not be told apart. The
// executor-native builtins are left out: an engine runs them itself and their
// registered Fn is never what it calls.
func TestEveryDispatchedBuiltinHasAGoFunctionOfItsOwn(t *testing.T) {
	owner := map[string]string{}
	for _, def := range Builtins {
		if def.Builtin == nil || ExecutorNativeKind(def.Builtin) != "" {
			continue
		}
		goName := runtime.FuncForPC(reflect.ValueOf(def.Builtin.Fn).Pointer()).Name()
		if other, taken := owner[goName]; taken && other != def.Name {
			t.Errorf("%s and %s are both registered as %s; give one a function of its own", other, def.Name, goName)
			continue
		}
		owner[goName] = def.Name
	}
}

// With no registered function on the stack there is no builtin to name, and
// the context says so rather than naming whatever Go function raised it.
func TestErrorContextNamesNoBuiltinWhenNoneIsOnTheStack(t *testing.T) {
	if got := newError("raised by no builtin").Context; got != "builtin" {
		t.Errorf("err.context = %q, want %q", got, "builtin")
	}
}
