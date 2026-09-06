package linear

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/store"
)

// NewDeterminant2 composes ad-bc for the four entries of a 2-by-2 matrix.
func NewDeterminant2() core.Primitive {
	return equation.NewDifference[float64](
		equation.NewProduct[float64](store.NewGet("a"), store.NewGet("d")),
		equation.NewProduct[float64](store.NewGet("b"), store.NewGet("c")),
	)
}
