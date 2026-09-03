package types

/*
Sum evaluates all non-nil branches and computes their unweighted summation.
Degenerate zero-value behavior: omitted slots contribute 0.
*/
type Sum struct {
	A Node
	B Node
	C Node
	D Node
}

func (sumNode *Sum) Step(number Number) Number {
	var sum Number

	if sumNode.A != nil {
		sum += sumNode.A.Step(number)
	}

	if sumNode.B != nil {
		sum += sumNode.B.Step(number)
	}

	if sumNode.C != nil {
		sum += sumNode.C.Step(number)
	}

	if sumNode.D != nil {
		sum += sumNode.D.Step(number)
	}

	return sum
}
