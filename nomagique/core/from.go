package core

// From lifts a boundary value. A Primitive is already inside the algebra.
func From[T any](value T) Primitive {
	// Inspect the type without taking the payload's address and forcing it to escape.
	if _, primitiveType := any((*T)(nil)).(*Primitive); primitiveType && any(value) == nil {
		return nil
	}

	if primitive, ok := any(value).(Primitive); ok {
		return primitive
	}

	return NewProto(value)
}
