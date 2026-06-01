package perspectives

/*
Perspective is one trade thesis encoded as entry and exit decision trees.
*/
type Perspective interface {
	Walk(measurements []Measurement) Perspective
}
