package equation

import (
	"iter"
	"math"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
FisherInput is a correlation and the support that scales its Fisher z.
*/
type FisherInput struct {
	Correlation float64
	Support     float64
}

/*
Fisher owns the Fisher-z normal-tail formula. The n-3 degrees of freedom and
the √2 in the complementary error function are identities of that formula.
Invalid domains propagate NaN/Inf normally.
*/
type Fisher struct {
	core.Base[FisherInput, float64]
}

func NewFisher() *Fisher {
	return &Fisher{}
}

func (op *Fisher) Next(
	in iter.Seq[core.Primitive[FisherInput, FisherInput]],
) iter.Seq[core.Primitive[float64, float64]] {
	return func(yield func(core.Primitive[float64, float64]) bool) {
		for arriving := range in {
			input := arriving.Read()
			z := math.Atanh(input.Correlation) * math.Sqrt(input.Support-3)
			p := math.Erfc(math.Abs(z) / math.Sqrt2)

			if !yield(op.Carrier(p)) {
				return
			}
		}
	}
}
