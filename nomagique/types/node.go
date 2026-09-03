package types

/*
Node is the closed engine contract for all transformations, filters, equations,
and reductions in nomagique.
*/
type Node interface {
	Step(Number) Number
}
