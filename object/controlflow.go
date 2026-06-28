package object

type Break struct{}

func (b *Break) Type() ObjectType {
	return BREAK_OBJ
}

func (b *Break) Inspect() string {
	return "break"
}

type Continue struct{}

func (c *Continue) Type() ObjectType {
	return CONTINUE_OBJ
}

func (c *Continue) Inspect() string {
	return "continue"
}
