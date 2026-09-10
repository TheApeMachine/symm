package vector

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
Finite reports whether every scalar in the vector is finite.
*/
type Finite struct {
	core.Base[[]float64, bool]
	finite *logic.Finite[float64]
}

func NewFinite() *Finite {
	return &Finite{finite: logic.NewFinite[float64]()}
}

func (op *Finite) Next(
	in iter.Seq[core.Primitive[[]float64, []float64]],
) iter.Seq[core.Primitive[bool, bool]] {
	return func(yield func(core.Primitive[bool, bool]) bool) {
		for arriving := range in {
			valid := true

			for _, value := range arriving.Read() {
				ok := true

				for decision := range op.finite.Next(transport.Values(value)) {
					ok = decision.Read()
				}

				valid = valid && ok
			}

			if !yield(op.Carrier(valid)) {
				return
			}
		}
	}
}
