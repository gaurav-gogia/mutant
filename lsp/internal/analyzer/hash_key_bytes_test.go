package analyzer

import "testing"

// A buffer is a hash key the way a string is, in a keyspace of its own, and
// has_key, get, set and delete all take one. The argument-type rule flagged
// each of them and named bytes_to_string as the repair -- a repair that moves
// the key into the string keyspace, so has_key on the converted value answered
// false for the entry set() had just made.
func TestBytesHashKeysAreNotFlagged(t *testing.T) {
	sources := []string{
		"let b, err = hex_decode_bytes(\"4d5a\");\nlet h = set({}, b, 1);\n",
		"let b, err = hex_decode_bytes(\"4d5a\");\nhas_key({\"a\": 1}, b);\n",
		"let b, err = hex_decode_bytes(\"4d5a\");\nget({\"a\": 1}, b, 0);\n",
		"let b, err = hex_decode_bytes(\"4d5a\");\ndelete({\"a\": 1}, b);\n",
	}
	for _, src := range sources {
		if msgs := argTypeMessages(t, src); len(msgs) != 0 {
			t.Errorf("%q produced %+v, want none", src, msgs)
		}
	}
}

// The rule still holds a key no hash can take, and names the kinds that can,
// BYTES among them.
func TestUnhashableKeyNamesBytesAmongTheKeyKinds(t *testing.T) {
	want := "argument 2 to `has_key` must be STRING, BYTES, INTEGER, FLOAT, or BOOLEAN, got ARRAY"
	msgs := argTypeMessages(t, "has_key({\"a\": 1}, [1]);\n")
	for _, m := range msgs {
		if m == want {
			return
		}
	}
	t.Fatalf("expected %q, got %+v", want, msgs)
}
