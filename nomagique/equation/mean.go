package equation

import (
	"github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
)

func NewMean() core.Primitive {
	return NewRatio[float64](arithmetic.NewAdd[float64](transport.NewIO(core.From(0.0))), NewCount())
}
