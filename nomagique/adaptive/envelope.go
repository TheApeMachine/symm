package adaptive

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
Envelope replaces a value with the inclusive moment interval when the
estimator has dispersion. Choice of coefficient is topology, not a type switch.
*/
type Envelope struct {
	core.Base[float64, float64]
	moments     core.Primitive[float64, equation.MomentReading]
	coefficient core.Primitive[float64, float64]
	bound       *equation.Bound[float64]
}

func NewEnvelope(
	moments core.Primitive[float64, equation.MomentReading],
	coefficient core.Primitive[float64, float64],
) *Envelope {
	return &Envelope{
		moments:     moments,
		coefficient: coefficient,
		bound:       equation.NewBound[float64](),
	}
}

func (op *Envelope) Next(
	in iter.Seq[core.Primitive[float64, float64]],
) iter.Seq[core.Primitive[float64, float64]] {
	return func(yield func(core.Primitive[float64, float64]) bool) {
		for reading := range op.moments.Next(in) {
			current := reading.Read()
			value := current.Value

			if current.Count > 1 && current.Dispersion > 0 {
				coefficient, err := transport.Evaluate(op.coefficient, transport.Values(current.Count))

				if err != nil {
					op.Error(err)
					return
				}

				margin := current.Dispersion * coefficient
				bounded, err := transport.Evaluate(op.bound, transport.Values(equation.BoundRecord[float64]{
					Value: value,
					Lower: current.Mean - margin,
					Upper: current.Mean + margin,
				}))

				if err != nil {
					op.Error(err)
					return
				}

				value = bounded
			}

			if !yield(op.Carrier(value)) {
				return
			}
		}

		op.Error(op.moments.Error())
	}
}
