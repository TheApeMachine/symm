package learning

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
BinaryTarget classifies an increase without inventing a new numeric rule.
*/
type BinaryTarget struct {
	core.Base[Observation, float64]
	finite *logic.Finite[float64]
}

func NewBinaryTarget() *BinaryTarget {
	return &BinaryTarget{finite: logic.NewFinite[float64]()}
}

func (op *BinaryTarget) Next(
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

			if !current || !past {
				op.Error(core.ErrDomain)
				return
			}

			value := 0.0

			if sample.Current > sample.Past {
				value = 1
			}

			if !yield(op.Carrier(value)) {
				return
			}
		}
	}
}
