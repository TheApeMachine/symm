package types

// Scalar is the unboxed 8-byte numeric carrier flowing through nomagique.
// Because its underlying type is float64, standard Go arithmetic works natively.
type Scalar float64

// Number is an alias for Scalar ensuring full compatibility.
type Number = Scalar

// Through steps this carrier value through any Node.
func (s Scalar) Through(node Node) Scalar {
	return node.Step(s)
}
