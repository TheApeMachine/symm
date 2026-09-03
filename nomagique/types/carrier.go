package types

/*
Number is the unboxed 8-byte carrier for all streaming values in nomagique.
Its underlying type is float64, allowing it to participate natively in Go arithmetic
without unboxing, interface wrappers, or heap allocations.
*/
type Number float64

// Through steps this carrier value through any Node.
func (number Number) Through(node Node) Number {
	return node.Step(number)
}
