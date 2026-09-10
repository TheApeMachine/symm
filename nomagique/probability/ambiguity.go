package probability

import (
	"iter"
	"math"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
Ambiguity divides entropy by the entropy of an equal-mass distribution.
A one-member distribution has zero ambiguity by definition.
*/
type Ambiguity struct {
	core.Base[float64, float64]
}

func NewAmbiguity() *Ambiguity {
	return &Ambiguity{}
}

func (op *Ambiguity) Next(
	in iter.Seq[core.Primitive[float64, float64]],
) iter.Seq[core.Primitive[float64, float64]] {
	return func(yield func(core.Primitive[float64, float64]) bool) {
		var values []float64

		for arriving := range in {
			values = append(values, arriving.Read())
		}

		if len(values) <= 1 {
			if !yield(op.Carrier(0)) {
				return
			}

			return
		}

		entropy := equation.NewEntropy[float64]()
		value := 0.0

		for out := range entropy.Next(equation.NewNormalize[float64]().Next(transport.Values(values...))) {
			value = out.Read()
		}

		if !yield(op.Carrier(value / math.Log(float64(len(values))))) {
			return
		}
	}
}
