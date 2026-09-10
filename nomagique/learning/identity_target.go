package learning

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
Observation is the current reference and the past reference a target may use.
*/
type Observation struct {
	Current float64
	Past    float64
}

/*
IdentityTarget selects the finite current value.
*/
type IdentityTarget struct {
	core.Base[Observation, float64]
	finite *logic.Finite[float64]
}

func NewIdentityTarget() *IdentityTarget {
	return &IdentityTarget{finite: logic.NewFinite[float64]()}
}

func (op *IdentityTarget) Next(
	in iter.Seq[core.Primitive[Observation, Observation]],
) iter.Seq[core.Primitive[float64, float64]] {
	return func(yield func(core.Primitive[float64, float64]) bool) {
		for arriving := range in {
			value := arriving.Read().Current
			defined, err := transport.Evaluate(op.finite, transport.Values(value))

			if err != nil {
				op.Error(err)
				return
			}

			if !defined {
				op.Error(core.ErrDomain)
				return
			}

			if !yield(op.Carrier(value)) {
				return
			}
		}
	}
}
