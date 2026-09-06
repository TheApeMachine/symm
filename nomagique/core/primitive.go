package core

// Primitive is the complete computational contract. Next ends a delivery run
// with nil; a subsequent run remains the callee's concern.
type Primitive interface {
	Next(Primitive) Primitive
	Read() any
	Error(...error) error
}

// Numeric is a constraint, not a second runtime value representation.
type Numeric interface {
	~float64 | ~float32 | ~int | ~int64 | ~uint64
}

// Floating is the representation constraint for real-valued division. Integer
// division is a different operation and is not silently used as a field inverse.
type Floating interface{ ~float32 | ~float64 }
