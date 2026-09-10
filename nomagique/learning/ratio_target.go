package learning

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
RatioTarget is the relative change, with an explicit nonzero past domain.
*/
type RatioTarget struct {
	core.Base[Observation, float64]
	finite *logic.Finite[float64]
}

func NewRatioTarget() *RatioTarget {
	return &RatioTarget{finite: logic.NewFinite[float64]()}
}

func (op *RatioTarget) Next(
	in iter.Seq[core.Primitive[Observation, Observation]],
) iter.Seq[core.Primitive[float64, float64]] {
	return func(yield func(core.Primitive[float64, float64]) bool) {
		for arriving := range in {
			sample := arriving.Read()
			current, err := transport.Evaluate(op.finite, transport.Values(sample.Current))

			if err != nil {
				op.Error(err)
				return
			}

			past, err := transport.Evaluate(op.finite, transport.Values(sample.Past))

			if err != nil {
				op.Error(err)
				return
			}

			if !current || !past || sample.Past == 0 {
				op.Error(core.ErrDomain)
				return
			}

			if !yield(op.Carrier(sample.Current/sample.Past - 1)) {
				return
			}
		}
	}
}
