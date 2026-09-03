package types

/*
Router is the universal condition key for parallel Split nodes.
It returns four branch weights (wA, wB, wC, wD) evaluated from the input sample.
*/
type Router interface {
	Route(Number) (Number, Number, Number, Number)
}
