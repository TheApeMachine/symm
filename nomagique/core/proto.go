package core

// Proto carries a boundary value. It does not own delivery.
type Proto struct {
	PrimitiveError
	state any
}

func NewProto(state any) *Proto               { return &Proto{state: state} }
func (value *Proto) Next(Primitive) Primitive { return nil }
func (value *Proto) Read() any                { return value.state }
