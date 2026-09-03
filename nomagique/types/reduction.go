package types

// Reduction is a pure fold over a contiguous slice of Scalar values.
type Reduction func([]Scalar) Scalar
