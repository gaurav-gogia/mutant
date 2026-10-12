package builtin

import (
	"bytes"
	"fmt"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"mutant/object"
)

type cacheEntry struct {
	expiresAt time.Time
}

type cacheStore struct {
	entries map[string]cacheEntry
	values  map[string]object.Object
	stats   cacheStoreStats
}

type cacheStoreStats struct {
	hits    int64
	misses  int64
	puts    int64
	deletes int64
	expires int64
	clears  int64
}

var runtimeCacheStores = struct {
	sync.RWMutex
	stores map[string]*cacheStore
}{
	stores: map[string]*cacheStore{},
}

// cacheValues serialises reads and writes of every store's values map.
//
// One process-wide lock rather than one per store, deliberately: the values
// used to be held by calling a gRPC server method in-process, which required
// swapping a package-level global in the backend for the duration of the call,
// so a single process-wide lock is the serialisation this code has always had.
// Keeping it means the change cannot introduce a concurrency difference. It is
// held only across a single map read or write.
var cacheValues sync.Mutex

func CacheOpen(args ...object.Object) object.Object {
	if len(args) != 1 {
		return resultAndError(nil, newError("wrong number of arguments. got=%d, want=1", len(args)))
	}

	nameObj, ok := args[0].(*object.String)
	if !ok {
		return resultAndError(nil, newError("argument 1 to `cache_open` must be STRING, got %s", args[0].Type()))
	}
	if nameObj.Value == "" {
		return resultAndError(nil, newError("argument 1 to `cache_open` must not be empty"))
	}

	runtimeCacheStores.Lock()
	_, exists := runtimeCacheStores.stores[nameObj.Value]
	if !exists {
		runtimeCacheStores.stores[nameObj.Value] = &cacheStore{
			entries: map[string]cacheEntry{},
			values:  map[string]object.Object{},
		}
	}
	runtimeCacheStores.Unlock()

	return resultAndError(makeHashObject(map[string]object.Object{
		"name":    stringObj(nameObj.Value),
		"opened":  boolObj(true),
		"created": boolObj(!exists),
	}), nil)
}

func CachePut(args ...object.Object) object.Object {
	if len(args) != 3 && len(args) != 4 {
		return resultAndError(nil, newError("wrong number of arguments. got=%d, want=3 or 4", len(args)))
	}

	cacheName, key, errObj := cacheNameAndKey(BuiltinNameCachePut, args[0], args[1])
	if errObj != nil {
		return resultAndError(nil, errObj)
	}
	if errObj := refuseClassified(BuiltinNameCachePut, args...); errObj != nil {
		return resultAndError(nil, errObj)
	}

	store, errObj := cacheStoreByName(cacheName)
	if errObj != nil {
		return resultAndError(nil, errObj)
	}

	expiresAt := time.Time{}
	if len(args) == 4 {
		ttlObj, ok := args[3].(*object.Integer)
		if !ok {
			return resultAndError(nil, newError("argument 4 to `cache_put` must be INTEGER, got %s", args[3].Type()))
		}
		if ttlObj.Value < 0 {
			return resultAndError(nil, newError("argument 4 to `cache_put` must be >= 0"))
		}
		if ttlObj.Value > 0 {
			expiresAt = time.Now().UTC().Add(time.Duration(ttlObj.Value) * time.Second)
		}
	}

	if errObj := cacheSetValue(store, key, args[2]); errObj != nil {
		return resultAndError(nil, errObj)
	}

	runtimeCacheStores.Lock()
	store.entries[key] = cacheEntry{expiresAt: expiresAt}
	store.stats.puts++
	runtimeCacheStores.Unlock()

	return resultAndError(boolObj(true), nil)
}

func CacheGet(args ...object.Object) object.Object {
	if len(args) != 2 {
		return resultAndError(nil, newError("wrong number of arguments. got=%d, want=2", len(args)))
	}

	cacheName, key, errObj := cacheNameAndKey(BuiltinNameCacheGet, args[0], args[1])
	if errObj != nil {
		return resultAndError(nil, errObj)
	}

	store, errObj := cacheStoreByName(cacheName)
	if errObj != nil {
		return resultAndError(nil, errObj)
	}

	now := time.Now().UTC()
	runtimeCacheStores.Lock()
	entry, ok := store.entries[key]
	if ok && !entry.expiresAt.IsZero() && now.After(entry.expiresAt) {
		delete(store.entries, key)
		store.stats.expires++
		ok = false
	}
	if ok {
		store.stats.hits++
	} else {
		store.stats.misses++
	}
	runtimeCacheStores.Unlock()

	if !ok {
		return resultAndError(makeHashObject(map[string]object.Object{
			"found": boolObj(false),
			"value": &object.Null{},
		}), nil)
	}

	value, errObj := cacheGetValue(store, key)
	if errObj != nil {
		if strings.Contains(strings.ToLower(errObj.Message), "not found") {
			runtimeCacheStores.Lock()
			delete(store.entries, key)
			store.stats.hits-- // undo the optimistic hit counted above; this call is a miss
			store.stats.misses++
			runtimeCacheStores.Unlock()
			return resultAndError(makeHashObject(map[string]object.Object{
				"found": boolObj(false),
				"value": &object.Null{},
			}), nil)
		}
		return resultAndError(nil, errObj)
	}

	return resultAndError(makeHashObject(map[string]object.Object{
		"found": boolObj(true),
		"value": value,
	}), nil)
}

func CacheDelete(args ...object.Object) object.Object {
	if len(args) != 2 {
		return resultAndError(nil, newError("wrong number of arguments. got=%d, want=2", len(args)))
	}

	cacheName, key, errObj := cacheNameAndKey(BuiltinNameCacheDelete, args[0], args[1])
	if errObj != nil {
		return resultAndError(nil, errObj)
	}

	store, errObj := cacheStoreByName(cacheName)
	if errObj != nil {
		return resultAndError(nil, errObj)
	}

	runtimeCacheStores.Lock()
	_, existed := store.entries[key]
	if existed {
		delete(store.entries, key)
		store.stats.deletes++
	}
	runtimeCacheStores.Unlock()

	if existed {
		_ = cacheSetValue(store, key, &object.Null{})
	}

	return resultAndError(boolObj(existed), nil)
}

func CacheKeys(args ...object.Object) object.Object {
	if len(args) != 1 {
		return resultAndError(nil, newError("wrong number of arguments. got=%d, want=1", len(args)))
	}

	cacheNameObj, ok := args[0].(*object.String)
	if !ok {
		return resultAndError(nil, newError("argument 1 to `cache_keys` must be STRING, got %s", args[0].Type()))
	}

	store, errObj := cacheStoreByName(cacheNameObj.Value)
	if errObj != nil {
		return resultAndError(nil, errObj)
	}

	now := time.Now().UTC()
	runtimeCacheStores.Lock()
	cacheCleanupExpiredLocked(store, now)
	keys := make([]string, 0, len(store.entries))
	for key := range store.entries {
		keys = append(keys, key)
	}
	runtimeCacheStores.Unlock()

	sort.Strings(keys)
	elements := make([]object.Object, len(keys))
	for i, key := range keys {
		elements[i] = stringObj(key)
	}
	return resultAndError(&object.Array{Elements: elements}, nil)
}

func CacheStats(args ...object.Object) object.Object {
	if len(args) != 1 {
		return resultAndError(nil, newError("wrong number of arguments. got=%d, want=1", len(args)))
	}

	cacheNameObj, ok := args[0].(*object.String)
	if !ok {
		return resultAndError(nil, newError("argument 1 to `cache_stats` must be STRING, got %s", args[0].Type()))
	}

	store, errObj := cacheStoreByName(cacheNameObj.Value)
	if errObj != nil {
		return resultAndError(nil, errObj)
	}

	now := time.Now().UTC()
	runtimeCacheStores.Lock()
	cacheCleanupExpiredLocked(store, now)
	items := int64(len(store.entries))
	stats := store.stats
	runtimeCacheStores.Unlock()

	return resultAndError(makeHashObject(map[string]object.Object{
		"name":    stringObj(cacheNameObj.Value),
		"items":   intObj(items),
		"hits":    intObj(stats.hits),
		"misses":  intObj(stats.misses),
		"puts":    intObj(stats.puts),
		"deletes": intObj(stats.deletes),
		"expires": intObj(stats.expires),
		"clears":  intObj(stats.clears),
	}), nil)
}

func CacheClear(args ...object.Object) object.Object {
	if len(args) != 1 {
		return resultAndError(nil, newError("wrong number of arguments. got=%d, want=1", len(args)))
	}

	cacheNameObj, ok := args[0].(*object.String)
	if !ok {
		return resultAndError(nil, newError("argument 1 to `cache_clear` must be STRING, got %s", args[0].Type()))
	}

	store, errObj := cacheStoreByName(cacheNameObj.Value)
	if errObj != nil {
		return resultAndError(nil, errObj)
	}

	runtimeCacheStores.Lock()
	removed := int64(len(store.entries))
	store.entries = map[string]cacheEntry{}
	store.values = map[string]object.Object{}
	store.stats.clears++
	runtimeCacheStores.Unlock()

	return resultAndError(intObj(removed), nil)
}

func CacheClose(args ...object.Object) object.Object {
	if len(args) != 1 {
		return resultAndError(nil, newError("wrong number of arguments. got=%d, want=1", len(args)))
	}

	cacheNameObj, ok := args[0].(*object.String)
	if !ok {
		return resultAndError(nil, newError("argument 1 to `cache_close` must be STRING, got %s", args[0].Type()))
	}

	runtimeCacheStores.Lock()
	store, ok := runtimeCacheStores.stores[cacheNameObj.Value]
	if ok {
		delete(runtimeCacheStores.stores, cacheNameObj.Value)
	}
	runtimeCacheStores.Unlock()

	if !ok {
		return resultAndError(nil, newError("cache `%s` not found; call `cache_open` first", cacheNameObj.Value))
	}

	runtimeCacheStores.Lock()
	store.entries = map[string]cacheEntry{}
	store.values = nil
	runtimeCacheStores.Unlock()

	return resultAndError(makeHashObject(map[string]object.Object{
		"name":   stringObj(cacheNameObj.Value),
		"closed": boolObj(true),
	}), nil)
}

func cacheNameAndKey(opName string, cacheNameObj object.Object, keyObj object.Object) (string, string, *object.Error) {
	cacheName, ok := cacheNameObj.(*object.String)
	if !ok {
		return "", "", newError("argument 1 to `%s` must be STRING, got %s", opName, cacheNameObj.Type())
	}
	key, ok := keyObj.(*object.String)
	if !ok {
		return "", "", newError("argument 2 to `%s` must be STRING, got %s", opName, keyObj.Type())
	}
	return cacheName.Value, key.Value, nil
}

func cacheStoreByName(name string) (*cacheStore, *object.Error) {
	runtimeCacheStores.RLock()
	store, ok := runtimeCacheStores.stores[name]
	runtimeCacheStores.RUnlock()
	if !ok {
		return nil, newError("cache `%s` not found; call `cache_open` first", name)
	}
	return store, nil
}

func cacheCleanupExpiredLocked(store *cacheStore, now time.Time) {
	for key, entry := range store.entries {
		if !entry.expiresAt.IsZero() && now.After(entry.expiresAt) {
			delete(store.entries, key)
			store.stats.expires++
		}
	}
}

// cacheSetValue stores a copy of value under key. Only cache_put hands it a
// value a program chose, so a value it cannot copy is refused in cache_put's
// words; cache_delete's NULL always copies.
func cacheSetValue(store *cacheStore, key string, value object.Object) *object.Error {
	stored, err := cacheCopy(value, 0)
	if err != nil {
		return newError("argument 3 to `%s` %s", BuiltinNameCachePut, err.Error())
	}

	values := cacheStoreValuesSnapshot(store)
	if values == nil {
		return newError("cache backend set: cache is closed")
	}

	cacheValues.Lock()
	values[key] = stored
	cacheValues.Unlock()

	return nil
}

// cacheStoreValuesSnapshot reads store.values under the same lock that
// CacheClear/CacheClose use to write it, avoiding a data race and a nil-deref
// after close. The returned map stays usable for the caller even if the store
// is closed concurrently: CacheClose replaces the field, it does not clear the
// map, so an in-flight operation completes against the pre-close values.
func cacheStoreValuesSnapshot(store *cacheStore) map[string]object.Object {
	runtimeCacheStores.RLock()
	values := store.values
	runtimeCacheStores.RUnlock()
	return values
}

func cacheGetValue(store *cacheStore, key string) (object.Object, *object.Error) {
	values := cacheStoreValuesSnapshot(store)
	if values == nil {
		return nil, newError("cache backend get: cache is closed")
	}

	cacheValues.Lock()
	stored, found := values[key]
	cacheValues.Unlock()

	// "not found" is load-bearing, not decoration: CacheGet matches this
	// message to recognise an entry whose value has gone and correct its own
	// bookkeeping from a hit to a miss. The old backend returned a gRPC
	// NotFound status whose text happened to contain the same two words.
	if !found {
		return nil, newError("cache backend get: key not found")
	}

	// Copied outside the lock, which is safe because nothing changes a stored
	// value: a put replaces it and every get hands out a copy of its own. The
	// copy cannot fail either, since what is stored is what cacheCopy made.
	value, err := cacheCopy(stored, 0)
	if err != nil {
		return nil, newError("cache backend get: the stored value %s", err.Error())
	}
	return value, nil
}

// cacheCopy returns a value of value's type, equal to it, that shares nothing
// a program can change with it. cache_put keeps one and cache_get hands out
// another, so a value comes back as it went in.
//
// It replaced a round trip through JSON, which has one kind of number and no
// binary type: an INTEGER came back as a FLOAT, rounded once past 2^53, a
// BYTES came back as its hex in a STRING, a STRUCT as a HASH, and a STRING
// that was not UTF-8 had each stray byte replaced with U+FFFD (M26-BLT-010).
//
// It copies in both directions, and all the way down, because a value can be
// changed in place after cache_put has returned: the evaluator assigns to an
// element of an array, a hash or a buffer where it stands, so `b[0] = 0` on a
// buffer just stored would reach into an entry that shared its memory, and a
// program that read the entry could do the same to what it was handed.
//
// The depth bound is the one refuseClassified walks to, and cache_put has
// already refused a value nested deeper. What reaches it here holds itself --
// the evaluator makes one of `a[0] = a` -- or reaches one container by two
// paths, one of them longer than the bound.
func cacheCopy(value object.Object, depth int) (object.Object, error) {
	if depth > classifiedWalkDepth {
		return nil, fmt.Errorf("is nested more than %d levels deep, or holds itself; the cache keeps a copy "+
			"of what it is given, and a value that holds itself has no end to copy", classifiedWalkDepth)
	}
	switch v := value.(type) {
	case nil:
		return nil, nil
	case *object.String:
		return stringObj(v.Value), nil
	case *object.Integer:
		return intObj(v.Value), nil
	case *object.Float:
		return floatObj(v.Value), nil
	case *object.Boolean:
		return boolObj(v.Value), nil
	case *object.Null:
		return &object.Null{}, nil
	case *object.Bytes:
		// The mark goes with the bytes. cache_put refuses a marked buffer
		// before it gets here, and a copy that dropped the mark would launder
		// one the day that changed.
		return &object.Bytes{Value: bytes.Clone(v.Value), Classified: v.Classified}, nil
	case *object.Array:
		elements := make([]object.Object, len(v.Elements))
		for i, element := range v.Elements {
			copied, err := cacheCopy(element, depth+1)
			if err != nil {
				return nil, err
			}
			elements[i] = copied
		}
		return &object.Array{Elements: elements}, nil
	case *object.Hash:
		// Each pair stays filed under the HashKey it had: the copy of its key
		// holds the same value.
		pairs := make(map[object.HashKey]object.HashPair, len(v.Pairs))
		for hashKey, pair := range v.Pairs {
			key, err := cacheCopy(pair.Key, depth+1)
			if err != nil {
				return nil, err
			}
			item, err := cacheCopy(pair.Value, depth+1)
			if err != nil {
				return nil, err
			}
			pairs[hashKey] = object.HashPair{Key: key, Value: item}
		}
		return &object.Hash{Pairs: pairs}, nil
	case *object.Struct:
		fields := make(map[string]object.Object, len(v.Fields))
		for name, field := range v.Fields {
			copied, err := cacheCopy(field, depth+1)
			if err != nil {
				return nil, err
			}
			fields[name] = copied
		}
		return &object.Struct{TypeName: v.TypeName, FieldOrder: slices.Clone(v.FieldOrder), Fields: fields}, nil
	case *object.EnumValue:
		payload, err := cacheCopy(v.Value, depth+1)
		if err != nil {
			return nil, err
		}
		return &object.EnumValue{TypeName: v.TypeName, Tag: v.Tag, Value: payload}, nil
	}
	verb := "holds"
	if depth == 0 {
		verb = "is"
	}
	return nil, fmt.Errorf("%s a %s, and a cache entry holds only STRING, BYTES, INTEGER, FLOAT, BOOLEAN and NULL, "+
		"and ARRAY, HASH, STRUCT and ENUM_VALUE values made of them", verb, value.Type())
}
