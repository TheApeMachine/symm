package types

// Product evaluates all non-nil branches and computes their multiplicative product.
// Degenerate zero-value behavior: omitted slots contribute 1.
type Product struct {
	A Node
	B Node
	C Node
	D Node
}

func (productNode *Product) Step(x Scalar) Scalar {
	product := Scalar(1)

	if productNode.A != nil {
		product *= productNode.A.Step(x)
	}

	if productNode.B != nil {
		product *= productNode.B.Step(x)
	}

	if productNode.C != nil {
		product *= productNode.C.Step(x)
	}

	if productNode.D != nil {
		product *= productNode.D.Step(x)
	}

	return product
}

// Slots exposes the nodes this product is composed of.
func (productNode *Product) Slots() []Node {
	return []Node{productNode.A, productNode.B, productNode.C, productNode.D}
}
