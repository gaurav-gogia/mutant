package object

import "fmt"

type Encrypted struct {
	Value   []byte
	EncType ObjectType
	Seed    int64
}

func (e *Encrypted) Type() ObjectType { return ENCRYPTED_OBJ }
func (e *Encrypted) Inspect() string  { return fmt.Sprintf("%v", e.Value) }
