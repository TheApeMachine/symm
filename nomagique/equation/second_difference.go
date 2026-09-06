package equation

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/store"
)

// NewSecondDifference composes 2*center-left-right. Choice of magnitude versus
// signed value, and spacing normalization, are supplied by the surrounding graph.
func NewSecondDifference(left, center, right core.Primitive) core.Primitive {
	return NewDifference[float64](
		NewProduct[float64](store.NewConstant(core.From(2.0)), center), NewSum[float64](left, right))
}
