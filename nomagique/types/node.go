package types

// Node is the closed transformation contract across all primitives and compositions.
type Node interface {
	Step(Scalar) Scalar
}