package adaptive

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
Threshold composes a moment estimator with a dispersion coefficient. The
coefficient is applied to the estimator's count; a source with no dispersion
has threshold one.
*/
type Threshold struct {
	core.Base[float64, float64]
	moments     core.Primitive[float64, equation.MomentReading]
	coefficient core.Primitive[float64, float64]
	threshold   *equation.Threshold[float64]
}

func NewThreshold(
	moments core.Primitive[float64, equation.MomentReading],
	coefficient core.Primitive[float64, float64],
) *Threshold {
	return &Threshold{
		moments:     moments,
		coefficient: coefficient,
		threshold:   equation.NewThreshold[float64](),
	}
}

func (op *Threshold) Next(
	in iter.Seq[core.Primitive[float64, float64]],
) iter.Seq[core.Primitive[float64, float64]] {
	return func(yield func(core.Primitive[float64, float64]) bool) {
		for reading := range op.moments.Next(in) {
			current := reading.Read()
			coefficient, err := transport.Evaluate(op.coefficient, transport.Values(current.Count))

			if err != nil {
				op.Error(err)
				return
			}

			value, err := transport.Evaluate(op.threshold, transport.Values(equation.ThresholdInput[float64]{
				Dispersion:  current.Dispersion,
				Coefficient: coefficient,
			}))

			if err != nil {
				op.Error(err)
				return
			}

			if !yield(op.Carrier(value)) {
				return
			}
		}

		op.Error(op.moments.Error())
	}
}
