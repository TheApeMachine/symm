package types

/*
IdentityNode is the functional identity wire I(x) = x for Node pipelines.
*/
type IdentityNode struct{}

func (identity IdentityNode) Step(number Number) Number {
	return number
}
