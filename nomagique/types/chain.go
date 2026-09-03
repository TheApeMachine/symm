package types

// Chain evaluates nodes sequentially in series: x -> A -> B -> C -> D -> y.
// Empty slots degenerate to the functional identity I(x) = x.
type Chain struct {
	A, B, C, D Node
}

func (c *Chain) Step(x Scalar) Scalar {
	if c.A != nil {
		x = c.A.Step(x)
	}

	if c.B != nil {
		x = c.B.Step(x)
	}

	if c.C != nil {
		x = c.C.Step(x)
	}

	if c.D != nil {
		x = c.D.Step(x)
	}

	return x
}
