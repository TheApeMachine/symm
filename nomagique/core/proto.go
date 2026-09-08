package core

/*
Proto carries a boundary value. It does not own delivery.

Proto is deliberately not pooled. A carrier that is handed out and read an
arbitrary number of times has no point at which ownership demonstrably ends,
so recycling it in Read returns an object its caller still holds: the next
NewProto hands the same pointer to someone else and overwrites state, and the
first reader observes a value of the wrong type. Pooling here needs an explicit
release at a real ownership boundary, not a release on every read.
*/
type Proto struct {
	PrimitiveError
	state any
}

func NewProto(state any) *Proto               { return &Proto{state: state} }
func (value *Proto) Next(Primitive) Primitive { return nil }
func (value *Proto) Read() any                { return value.state }
