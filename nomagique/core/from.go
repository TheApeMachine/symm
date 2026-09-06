package core

// From lifts a boundary value. A Primitive is already inside the algebra.
func From[T any](value T) Primitive {
	if slot, ok := any(&value).(*Primitive); ok {
		return *slot
	}
	if primitive, ok := any(value).(Primitive); ok {
		return primitive
	}
	return NewProto(value)
}
