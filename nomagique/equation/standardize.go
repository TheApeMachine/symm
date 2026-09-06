package equation

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/store"
)

// NewStandardize composes (value - center) / scale. State and the choice of
// causal center/scale are supplied in the incoming record, not owned here.
func NewStandardize() core.Primitive {
	return NewRatio[float64](NewDifference[float64](store.NewGet("value"), store.NewGet("center")), store.NewGet("scale"))
}
