package types

// Sum evaluates all non-nil branches and computes their unweighted summation.
// Degenerate zero-value behavior: omitted slots contribute 0.
type Sum struct {
	A Node
	B Node
	C Node
	D Node
}

func (sumNode *Sum) Step(x Scalar) Scalar {
	var sum Scalar

	if sumNode.A != nil {
		sum += sumNode.A.Step(x)
	}

	if sumNode.B != nil {
		sum += sumNode.B.Step(x)
	}

	if sumNode.C != nil {
		sum += sumNode.C.Step(x)
	}

	if sumNode.D != nil {
		sum += sumNode.D.Step(x)
	}

	return sum
}

// Slots exposes the nodes this sum is composed of.
func (sumNode *Sum) Slots() []Node {
	return []Node{sumNode.A, sumNode.B, sumNode.C, sumNode.D}
}
