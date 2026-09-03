package types

/*
Split distributes an incoming signal across parallel branches and computes
their weighted summation: Output = sum(w_i * Branch_i(x)).
Degenerate zero-value behavior: empty slots contribute 0 (additive identity).
*/
type Split struct {
	Route Router
	A     Node
	B     Node
	C     Node
	D     Node
}

func (split *Split) Step(number Number) Number {
	weightA := Number(1)
	weightB := Number(1)
	weightC := Number(1)
	weightD := Number(1)

	if split.Route != nil {
		weightA, weightB, weightC, weightD = split.Route.Route(number)
	}

	var sum Number

	if split.A != nil && weightA > 0 {
		sum += weightA * split.A.Step(number)
	}

	if split.B != nil && weightB > 0 {
		sum += weightB * split.B.Step(number)
	}

	if split.C != nil && weightC > 0 {
		sum += weightC * split.C.Step(number)
	}

	if split.D != nil && weightD > 0 {
		sum += weightD * split.D.Step(number)
	}

	return sum
}
