package equation

import (
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
)

func NewRMS() core.Primitive {
	return transport.NewPipe(NewRatio[float64](NewEnergy(), NewCount()), calculus.NewSqrt(transport.NewIO(core.From(0.0))))
}
