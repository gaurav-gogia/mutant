package builtin

import (
	"bytes"
	"math"
	"slices"
	"strings"
	"testing"

	"mutant/object"
)

// M26-BLT-010. The cache kept a value as JSON, and JSON has one kind of number
// and no binary type, so cache_get handed back something other than what
// cache_put was given: INTEGER 42 came back as FLOAT 42.000000, INTEGER
// 9007199254740993 as FLOAT 9007199254740992.000000, BYTES de ad as the STRING
// "dead", a STRUCT as a HASH, and a STRING that was not UTF-8 with U+FFFD
// where its stray byte had been. cache_get's metadata promises "the
// stored value", and these tests hold it to that: the same type, the same
// value, at every level of a value.

// cacheBlt010RoundTrip stores value under key and reads it back. It reports a
// refused put and answers false, so one refusal does not hide the rest.
func cacheBlt010RoundTrip(t *testing.T, cacheName, key, name string, value object.Object) (object.Object, bool) {
	t.Helper()
	if _, errObj := unwrapPair(t, CachePut(stringObj(cacheName), stringObj(key), value)); errObj != nil {
		t.Errorf("%s: cache_put refused %s %s: %s", name, value.Type(), value.Inspect(), errObj.Inspect())
		return nil, false
	}
	return cacheBlt010Get(t, cacheName, key), true
}

func cacheBlt010Get(t *testing.T, cacheName, key string) object.Object {
	t.Helper()
	payload, errObj := unwrapPair(t, CacheGet(stringObj(cacheName), stringObj(key)))
	if errObj != nil {
		t.Fatalf("cache_get: %s", errObj.Inspect())
	}
	hash, ok := payload.(*object.Hash)
	if !ok {
		t.Fatalf("cache_get payload is %T, want a HASH", payload)
	}
	found, ok := cacheTestMustHashValue(t, hash, "found").(*object.Boolean)
	if !ok || !found.Value {
		t.Fatalf("cache_get did not find %q", key)
	}
	return cacheTestMustHashValue(t, hash, "value")
}

func cacheBlt010Open(t *testing.T, cacheName string) {
	t.Helper()
	if _, errObj := unwrapPair(t, CacheOpen(stringObj(cacheName))); errObj != nil {
		t.Fatalf("cache_open: %s", errObj.Inspect())
	}
	t.Cleanup(func() { CacheClose(stringObj(cacheName)) })
}

// cacheBlt010Same reports, at path, every way got differs from want. It checks
// the type at every level rather than comparing renders, because a render is
// where FLOAT 42 and INTEGER 42 can look alike, and it compares a FLOAT by its
// bits so that -0 and NaN are held to what was stored too.
func cacheBlt010Same(t *testing.T, path string, want, got object.Object) {
	t.Helper()
	if want == nil || got == nil {
		if want != got {
			t.Errorf("%s: got %v, want %v", path, got, want)
		}
		return
	}
	if got.Type() != want.Type() {
		t.Errorf("%s: got %s %s, want %s %s", path, got.Type(), got.Inspect(), want.Type(), want.Inspect())
		return
	}
	switch w := want.(type) {
	case *object.Integer:
		if g := got.(*object.Integer); g.Value != w.Value {
			t.Errorf("%s: got INTEGER %d, want %d", path, g.Value, w.Value)
		}
	case *object.Float:
		if g := got.(*object.Float); math.Float64bits(g.Value) != math.Float64bits(w.Value) {
			t.Errorf("%s: got FLOAT %v, want %v", path, g.Value, w.Value)
		}
	case *object.String:
		if g := got.(*object.String); g.Value != w.Value {
			t.Errorf("%s: got STRING %q, want %q", path, g.Value, w.Value)
		}
	case *object.Boolean:
		if g := got.(*object.Boolean); g.Value != w.Value {
			t.Errorf("%s: got BOOLEAN %v, want %v", path, g.Value, w.Value)
		}
	case *object.Null:
	case *object.Bytes:
		if g := got.(*object.Bytes); !bytes.Equal(g.Value, w.Value) {
			t.Errorf("%s: got BYTES %x, want %x", path, g.Value, w.Value)
		}
	case *object.Array:
		g := got.(*object.Array)
		if len(g.Elements) != len(w.Elements) {
			t.Errorf("%s: got %d elements, want %d", path, len(g.Elements), len(w.Elements))
			return
		}
		for i := range w.Elements {
			cacheBlt010Same(t, path+"["+intObj(int64(i)).Inspect()+"]", w.Elements[i], g.Elements[i])
		}
	case *object.Hash:
		g := got.(*object.Hash)
		if len(g.Pairs) != len(w.Pairs) {
			t.Errorf("%s: got %d pairs, want %d", path, len(g.Pairs), len(w.Pairs))
			return
		}
		for hashKey, wantPair := range w.Pairs {
			gotPair, ok := g.Pairs[hashKey]
			if !ok {
				t.Errorf("%s: the key %s %s is gone", path, wantPair.Key.Type(), wantPair.Key.Inspect())
				continue
			}
			keyPath := path + "{" + wantPair.Key.Inspect() + "}"
			cacheBlt010Same(t, keyPath+" key", wantPair.Key, gotPair.Key)
			cacheBlt010Same(t, keyPath, wantPair.Value, gotPair.Value)
		}
	case *object.Struct:
		g := got.(*object.Struct)
		if g.TypeName != w.TypeName || !slices.Equal(g.FieldOrder, w.FieldOrder) || len(g.Fields) != len(w.Fields) {
			t.Errorf("%s: got struct %s %v with %d fields, want %s %v with %d", path,
				g.TypeName, g.FieldOrder, len(g.Fields), w.TypeName, w.FieldOrder, len(w.Fields))
			return
		}
		for name, field := range w.Fields {
			cacheBlt010Same(t, path+"."+name, field, g.Fields[name])
		}
	case *object.EnumValue:
		g := got.(*object.EnumValue)
		if g.TypeName != w.TypeName || g.Tag != w.Tag {
			t.Errorf("%s: got %s.%s, want %s.%s", path, g.TypeName, g.Tag, w.TypeName, w.Tag)
		}
		cacheBlt010Same(t, path+" payload", w.Value, g.Value)
	default:
		t.Fatalf("%s: cacheBlt010Same has no rule for %s", path, want.Type())
	}
}

func cacheBlt010Hash(pairs ...object.Object) *object.Hash {
	hash := &object.Hash{Pairs: map[object.HashKey]object.HashPair{}}
	for i := 0; i+1 < len(pairs); i += 2 {
		hash.Pairs[pairs[i].(object.Hashable).HashKey()] = object.HashPair{Key: pairs[i], Value: pairs[i+1]}
	}
	return hash
}

func TestCacheGetReturnsTheTypeAndValueCachePutWasGiven(t *testing.T) {
	const cacheName = "t_cache_blt010_round_trip"
	cacheBlt010Open(t, cacheName)

	deadBeef := func() *object.Bytes { return &object.Bytes{Value: []byte{0xde, 0xad}} }
	notUTF8 := string([]byte{'a', 0xff, 'b'})

	cases := []struct {
		name  string
		value object.Object
	}{
		{"small integer", intObj(42)},
		{"integer past 2^53", intObj(9007199254740993)},
		{"integer the CLI reported", intObj(133700000000000001)},
		{"largest integer", intObj(math.MaxInt64)},
		{"smallest integer", intObj(math.MinInt64)},
		{"float with an integral value", floatObj(5)},
		{"float", floatObj(1.5)},
		{"negative zero", floatObj(math.Copysign(0, -1))},
		{"infinity", floatObj(math.Inf(1))},
		{"NaN", floatObj(math.NaN())},
		{"bytes", deadBeef()},
		{"empty bytes", &object.Bytes{Value: []byte{}}},
		{"string that is not UTF-8", stringObj(notUTF8)},
		{"empty string", stringObj("")},
		{"true", boolObj(true)},
		{"false", boolObj(false)},
		{"null", &object.Null{}},
		{"array of mixed kinds", &object.Array{Elements: []object.Object{
			intObj(1), floatObj(2), deadBeef(), stringObj(notUTF8), &object.Array{Elements: []object.Object{intObj(9007199254740993)}},
		}}},
		{"hash with keys of every kind", cacheBlt010Hash(
			stringObj("a"), intObj(1),
			intObj(7), stringObj("seven"),
			boolObj(true), floatObj(2.5),
			floatObj(0.5), &object.Null{},
			deadBeef(), deadBeef(),
		)},
		{"struct", &object.Struct{
			TypeName:   "Sample",
			FieldOrder: []string{"size", "head", "ratio"},
			Fields:     map[string]object.Object{"size": intObj(9007199254740993), "head": deadBeef(), "ratio": floatObj(3)},
		}},
		{"enum value", &object.EnumValue{TypeName: "Colour", Tag: "Green", Value: intObj(1)}},
	}

	for i, c := range cases {
		if got, ok := cacheBlt010RoundTrip(t, cacheName, "k"+intObj(int64(i)).Inspect(), c.name, c.value); ok {
			cacheBlt010Same(t, c.name, c.value, got)
		}
	}
}

// The cache copies on the way in and on the way out. The evaluator assigns to
// an element of an array, a hash or a buffer in place, so a cache that kept
// the object it was handed, or handed out the one it kept, would change after
// the fact. JSON gave that for free; a fix that stored the object itself would
// lose it.
func TestCacheEntryDoesNotShareMemoryWithItsCallers(t *testing.T) {
	const cacheName = "t_cache_blt010_no_sharing"
	cacheBlt010Open(t, cacheName)

	want := func() object.Object {
		return &object.Array{Elements: []object.Object{
			&object.Bytes{Value: []byte{0xde, 0xad}},
			&object.Array{Elements: []object.Object{intObj(1)}},
			makeHashObject(map[string]object.Object{"n": intObj(1)}),
		}}
	}

	stored := want().(*object.Array)
	if _, errObj := unwrapPair(t, CachePut(stringObj(cacheName), stringObj("k"), stored)); errObj != nil {
		t.Fatalf("cache_put: %s", errObj.Inspect())
	}

	// The program that stored it goes on changing its own value.
	stored.Elements[0].(*object.Bytes).Zero()
	stored.Elements[1].(*object.Array).Elements[0] = intObj(2)
	delete(stored.Elements[2].(*object.Hash).Pairs, stringObj("n").HashKey())

	got := cacheBlt010Get(t, cacheName, "k")
	cacheBlt010Same(t, "after the storer changed its value", want(), got)

	// So does a program that read it.
	if read, ok := got.(*object.Array); ok && len(read.Elements) == 3 {
		if buffer, ok := read.Elements[0].(*object.Bytes); ok && len(buffer.Value) == 2 {
			buffer.Value[1] = 0
		}
		if inner, ok := read.Elements[1].(*object.Array); ok && len(inner.Elements) == 1 {
			inner.Elements[0] = intObj(3)
		}
	}
	cacheBlt010Same(t, "after a reader changed its copy", want(), cacheBlt010Get(t, cacheName, "k"))
}

// What the cache cannot copy is refused in cache_put's words, and nothing is
// stored. A value that holds itself -- the evaluator makes one of `a[0] = a`
// -- used to take the process down: the conversion to JSON recursed until
// the stack ran out, which no recover can catch. It runs under a small stack
// so that a regression costs megabytes, not the host.
func TestCachePutRefusesAValueItCannotCopy(t *testing.T) {
	smallStack(t)
	const cacheName = "t_cache_blt010_refusals"
	cacheBlt010Open(t, cacheName)

	cyclic := &object.Array{}
	cyclic.Elements = []object.Object{intObj(1), cyclic}

	cases := []struct {
		name  string
		value object.Object
		want  string
	}{
		{"a function", &object.Function{}, "argument 3 to `cache_put` is a FUNCTION, and a cache entry holds only"},
		{"a closure inside a hash", makeHashObject(map[string]object.Object{"f": &object.Closure{}}),
			"argument 3 to `cache_put` holds a CLOSURE, and a cache entry holds only"},
		{"an array that holds itself", cyclic, "argument 3 to `cache_put` is nested more than 64 levels deep, or holds itself"},
	}
	for _, c := range cases {
		key := "k " + c.name
		_, errObj := unwrapPair(t, CachePut(stringObj(cacheName), stringObj(key), c.value))
		if errObj == nil {
			t.Errorf("%s: cache_put accepted it", c.name)
			continue
		}
		if !strings.Contains(errObj.Message, c.want) {
			t.Errorf("%s: got %q, want it to contain %q", c.name, errObj.Message, c.want)
		}
		payload, getErr := unwrapPair(t, CacheGet(stringObj(cacheName), stringObj(key)))
		if getErr != nil {
			t.Fatalf("%s: cache_get: %s", c.name, getErr.Inspect())
		}
		if found := cacheTestMustHashValue(t, payload.(*object.Hash), "found"); found.Inspect() != "false" {
			t.Errorf("%s: a refused put left an entry behind (found=%s)", c.name, found.Inspect())
		}
	}
}
