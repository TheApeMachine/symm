package equation

import (
	"github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
)

func NewKish() core.Primitive {
	return NewRatio[float64](
		transport.NewPipe(
			arithmetic.NewAdd[float64](transport.NewIO(core.From(0.0))),
			calculus.NewSquare(transport.NewIO(core.From(0.0))),
		),
		NewEnergy(),
	)
}
