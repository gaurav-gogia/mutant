package compiler

import (
	"errors"
	"fmt"
)

// LegacyRegistry says which release's builtin registry the OpGetBuiltin
// operands of a versionless program index.
//
// Bytecode compiled before v2.5.0 carries an ordinal into the registry of the
// release that compiled it, and the registries of v2.3.x and v2.4.x disagree at
// one ordinal. help was appended to the registry in init(), after every literal
// entry, so it sat at whatever the end was: ordinal 398 in v2.3.x, and 408 in
// v2.4.x, whose ten new builtins put pmap at 398. One operand therefore means
// help in one release and pmap in the next, and neither release wrote
// ByteCode.Version, so the version cannot say which was meant.
//
// The container can. v2.4.0 added ByteCode.OpcodeMap, and gob writes a struct's
// type ahead of the first value of it, naming every exported field whether or
// not the value holds anything there. So a versionless stream whose ByteCode
// type declares OpcodeMap was written by v2.4.x, and one whose type does not
// was written by v2.3.x or earlier. LegacyRegistryOf reads it.
//
// The answer is exact for every release from v2.3.0 to v2.4.x. v2.2.x wrote the
// same container as v2.3.x, and its registry is the first 211 entries of
// v2.3.x's, but a program of its that calls a builtin does not run correctly on
// this runtime: it was compiled for a one-byte OpGetBuiltin operand, and
// bytecode_version.go says what that does. The registries of releases before
// v2.2.0 ordered their entries differently, and neither table describes them.
//
// It is not exact for the two development commits between the v2.3.0 and
// v2.4.0 tags that registered pmap before OpcodeMap existed, 821bece and
// 31182f3. No release was made from either. An artifact one of them built reads
// as v2.3.x, so its pmap call reaches help; its operands above 398 still reach
// the builtins v2.4.0 added, because v2.3.x's table keeps them.
type LegacyRegistry int

const (
	// LegacyRegistryV24 is v2.4.x's registry, the snapshot frozen in
	// builtin/legacy_ordinals.go. It is the zero value, so versionless bytecode
	// that never came through a stream -- built in memory by a test -- reads as
	// it did before the registries were told apart.
	LegacyRegistryV24 LegacyRegistry = iota

	// LegacyRegistryV23 is the registry v2.3.x compiled against, of which
	// v2.2.x's is the first 211 entries. Its table is v2.4.x's snapshot with
	// help, not pmap, at 398: v2.3.x's 399 entries, then v2.4.x's after them.
	// Every versionless stream without an OpcodeMap reads as this.
	LegacyRegistryV23
)

// LegacyRegistry reports which builtin registry this program's ordinals index.
// It means nothing for a program that carries BuiltinNames, whose operands
// index that table instead.
func (bc *ByteCode) LegacyRegistry() LegacyRegistry { return bc.legacyRegistry }

// SetLegacyRegistry records which builtin registry this program's ordinals
// index, as LegacyRegistryOf read it from the stream the program was decoded
// from.
func (bc *ByteCode) SetLegacyRegistry(registry LegacyRegistry) {
	bc.legacyRegistry = registry
}

// LegacyRegistryOf reads, from the gob stream a versionless ByteCode was
// decoded from, which builtin registry that program's ordinals index.
//
// Only the stream's first message is read. gob defines a value's own type
// before the types of its fields, so an artifact's first message is ByteCode
// as the release that wrote it declared ByteCode. A stream that does not begin
// that way is refused rather than guessed at: guessing is how one operand came
// to mean two builtins.
func LegacyRegistryOf(stream []byte) (LegacyRegistry, error) {
	name, fields, err := gobLeadingStruct(stream)
	if err != nil {
		return 0, fmt.Errorf("the type this program's container was written with cannot be read, "+
			"so which release's builtins its calls name cannot be told: %w", err)
	}
	if name != byteCodeTypeName {
		return 0, fmt.Errorf("this program's container begins with the type %q rather than %s, "+
			"so which release's builtins its calls name cannot be told", name, byteCodeTypeName)
	}
	for _, field := range fields {
		if field == opcodeMapFieldName {
			return LegacyRegistryV24, nil
		}
	}
	return LegacyRegistryV23, nil
}

const (
	// byteCodeTypeName and opcodeMapFieldName are what gob calls ByteCode and
	// its OpcodeMap field in a type definition: the Go names.
	byteCodeTypeName   = "ByteCode"
	opcodeMapFieldName = "OpcodeMap"

	// gobWireStructField is the field number of StructT in encoding/gob's
	// wireType, whose fields are ArrayT, SliceT, StructT, MapT, GobEncoderT,
	// BinaryMarshalerT and TextMarshalerT, in that order.
	//
	//mutant:format encoding/gob wire format: wireType
	gobWireStructField = 2

	// gobWireFields is how many fields wireType has, which is the most of any
	// struct read here, so no field delta in a well-formed message exceeds it.
	//
	//mutant:format encoding/gob wire format: wireType
	gobWireFields = 7
)

var errGobTruncated = errors.New("the stream ends inside its first message")

// gobLeadingStruct decodes the first message of a gob stream as the definition
// of a struct type and returns the struct's name and its fields' names, in
// field order.
//
// The layout is encoding/gob's wire format as its package documentation sets
// it out. A message is a byte count, a type id -- negative for a type
// definition -- and a wireType value. A struct value is (field delta, value)
// pairs closed by a zero delta, with zero-valued fields left out, so each field
// is matched by its number rather than assumed present. wireType holds a
// structType for a struct; a structType is a CommonType (Name, Id) and a list
// of fieldType (Name, Id).
func gobLeadingStruct(stream []byte) (string, []string, error) {
	outer := gobReader{buf: stream}
	size := outer.readUint()
	if outer.err == nil && size > uint64(len(outer.buf)-outer.off) {
		outer.err = errGobTruncated
	}
	if outer.err != nil {
		return "", nil, outer.err
	}
	msg := &gobReader{buf: outer.buf[outer.off : outer.off+int(size)]}

	if id := msg.readInt(); msg.err == nil && id >= 0 {
		return "", nil, errors.New("the stream does not begin with a type definition")
	}

	var (
		name   string
		fields []string
		found  bool
	)
	for field := msg.field(-1); field >= 0; field = msg.field(field) {
		if field != gobWireStructField {
			msg.fail("the stream's first type is not a struct")
			break
		}
		found = true
		name, fields = msg.structType()
	}
	if msg.err != nil {
		return "", nil, msg.err
	}
	if !found {
		return "", nil, errors.New("the stream's first type is not a struct")
	}
	return name, fields, nil
}

// gobReader reads the primitive encodings of encoding/gob from a byte slice.
// The first failure sticks: every later read returns a zero value, so a caller
// checks err once, after the reads it cares about.
type gobReader struct {
	buf []byte
	off int
	err error
}

func (r *gobReader) fail(message string) {
	if r.err == nil {
		r.err = errors.New(message)
	}
}

// readUint reads an unsigned integer: one byte below 0x80, otherwise a byte
// holding the negated count of the big-endian bytes that follow.
func (r *gobReader) readUint() uint64 {
	if r.err != nil {
		return 0
	}
	if r.off >= len(r.buf) {
		r.err = errGobTruncated
		return 0
	}
	b := r.buf[r.off]
	r.off++
	if b < 0x80 {
		return uint64(b)
	}
	n := -int(int8(b))
	if n > 8 {
		r.fail("an unsigned integer in the stream is longer than eight bytes")
		return 0
	}
	if n > len(r.buf)-r.off {
		r.err = errGobTruncated
		return 0
	}
	var value uint64
	for _, c := range r.buf[r.off : r.off+n] {
		value = value<<8 | uint64(c)
	}
	r.off += n
	return value
}

// readInt reads a signed integer, which gob carries in an unsigned one: the
// value shifted up by one bit, with bit 0 saying it was complemented.
func (r *gobReader) readInt() int64 {
	u := r.readUint()
	if u&1 != 0 {
		return ^int64(u >> 1)
	}
	return int64(u >> 1)
}

// readString reads a byte count and that many bytes.
func (r *gobReader) readString() string {
	n := r.readUint()
	if r.err == nil && n > uint64(len(r.buf)-r.off) {
		r.err = errGobTruncated
	}
	if r.err != nil {
		return ""
	}
	s := string(r.buf[r.off : r.off+int(n)])
	r.off += int(n)
	return s
}

// field reads the delta that introduces a struct's next field and returns that
// field's number, given the number of the field before it (-1 at the start).
// It returns -1 at the zero delta that closes the struct, and on a failure.
func (r *gobReader) field(previous int) int {
	delta := r.readUint()
	if r.err != nil || delta == 0 {
		return -1
	}
	if delta > gobWireFields {
		r.fail("a field number in the stream is out of range")
		return -1
	}
	return previous + int(delta)
}

// structType reads a structType: its CommonType's name, and its fields' names.
func (r *gobReader) structType() (string, []string) {
	var (
		name   string
		fields []string
	)
	for field := r.field(-1); field >= 0; field = r.field(field) {
		switch field {
		case 0: // CommonType
			name = r.nameAndID()
		case 1: // Field: a count, then that many fieldType values
			count := r.readUint()
			// Each element takes at least the byte that closes it, which bounds
			// the count by what is left of the message.
			if r.err == nil && count > uint64(len(r.buf)-r.off) {
				r.err = errGobTruncated
			}
			for i := uint64(0); i < count && r.err == nil; i++ {
				fields = append(fields, r.nameAndID())
			}
		default:
			r.fail("a struct type in the stream has a field encoding/gob does not define")
		}
	}
	return name, fields
}

// nameAndID reads a CommonType or a fieldType, which share one layout: Name,
// then Id. It returns the name.
func (r *gobReader) nameAndID() string {
	var name string
	for field := r.field(-1); field >= 0; field = r.field(field) {
		switch field {
		case 0:
			name = r.readString()
		case 1:
			r.readInt()
		default:
			r.fail("a type name in the stream has a field encoding/gob does not define")
		}
	}
	return name
}
