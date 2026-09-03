package types

/*
Product evaluates all non-nil branches and computes their multiplication.
Degenerate zero-value behavior: omitted slots contribute 1 (multiplicative identity).
*/
type Product struct {
	A Node
	B Node
	C Node
	D Node
}

func (productNode *Product) Step(number Number) Number {
	product := Number(1)
	hasAny := false

	if productNode.A != nil {
		product *= productNode.A.Step(number)
		hasAny = true
	}

	if productNode.B != nil {
		product *= productNode.B.Step(number)
		hasAny = true
	}

	if productNode.C != nil {
		product *= productNode.C.Step(number)
		hasAny = true
	}

	if productNode.D != nil {
		product *= productNode.D.Step(number)
		hasAny = true
	}

	if !hasAny {
		return 1
	}

	return product
}
