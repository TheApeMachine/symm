package types

/*
Chain executes an ordered sequence of transformations in series (x -> A -> B -> C -> D -> y).
The output of slot A feeds directly into slot B, etc.
Degenerate zero-value behavior: empty slots are transparent (I(x) = x).
*/
type Chain struct {
	A Node
	B Node
	C Node
	D Node
}

func (chain *Chain) Step(number Number) Number {
	if chain.A != nil {
		number = chain.A.Step(number)
	}

	if chain.B != nil {
		number = chain.B.Step(number)
	}

	if chain.C != nil {
		number = chain.C.Step(number)
	}

	if chain.D != nil {
		number = chain.D.Step(number)
	}

	return number
}
