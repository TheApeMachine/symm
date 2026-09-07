package matrix

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
	"math"
)

/* Finite evaluates the existing matrix validity predicate without scalar delivery graphs. */
type Finite struct {
	core.PrimitiveError
	seed    *transport.IO
	current core.Primitive
}

func NewFinite() *Finite {
	return &Finite{seed: transport.NewIO(core.From(true))}
}

func (finite *Finite) Next(input core.Primitive) core.Primitive {
	result := core.Yield(finite.seed, input, func(valid bool, rows [][]float64) bool {
		for _, row := range rows {
			for _, value := range row {
				valid = valid && !math.IsNaN(value) && !math.IsInf(value, 0)
			}
		}
		return valid
	}, finite)

	if result != nil {
		finite.current = result
	}
	return result
}

func (finite *Finite) Read() any { return core.To[any](finite.current) }
