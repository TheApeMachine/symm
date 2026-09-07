package equation

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
)

/* Spacings owns consecutive timestamp differences within one delivery run. */
type Spacings struct {
	core.PrimitiveError
	seed    *transport.IO
	current core.Primitive
}

/*
NewSpacings subtracts integer nanosecond timestamps before conversion to float.
An empty or one-observation run has no adjacent pair and emits no spacing.
*/
func NewSpacings() core.Primitive {
	return transport.NewPipe(
		&Spacings{seed: transport.NewIO(core.From([]float64{}))},
		transport.NewSpread[float64](),
	)
}

func (spacings *Spacings) Next(input core.Primitive) core.Primitive {
	var previous int64
	observed := false
	result := core.Yield(spacings.seed, input,
		func(held []float64, fields map[string]core.Primitive) []float64 {
			at, err := core.Field[int64](fields, "at")
			spacings.Error(err)

			if observed {
				held = append(held, float64(at-previous))
			}
			previous, observed = at, true
			return held
		}, spacings)

	if result != nil {
		spacings.current = result
	}
	return result
}

func (spacings *Spacings) Read() any { return core.To[any](spacings.current) }
