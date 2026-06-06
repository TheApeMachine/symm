package perspectives

import "github.com/theapemachine/symm/market/perspectives/types"

/*
Perspective is one trade thesis encoded as entry and exit decision trees.
*/
type Perspective interface {
	Walk(measurements []types.Measurement) Perspective
}
