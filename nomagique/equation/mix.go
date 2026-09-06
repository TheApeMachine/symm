package equation

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/store"
)

// NewMix is left + weight*(right-left). Weight is data supplied in the record,
// not a mode on arithmetic. Zero preserves left; one selects right.
func NewMix() core.Primitive {
	return NewSum[float64](
		store.NewGet("left"),
		NewProduct[float64](store.NewGet("weight"),
			NewDifference[float64](store.NewGet("right"), store.NewGet("left"))),
	)
}
