package equation

import (
	"iter"
	"math"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
AdaptiveZScore uses log-space moments. Non-positive logarithms keep the
underlying real-domain result; they are not replaced by log(1).
*/
type AdaptiveZScore struct {
	core.Base[float64, CausalResidualResult]
	moments  core.Primitive[float64, MomentReading]
	residual *CausalResidual
}

func NewAdaptiveZScore(moments core.Primitive[float64, MomentReading]) *AdaptiveZScore {
	return &AdaptiveZScore{moments: moments, residual: NewCausalResidual()}
}

func (op *AdaptiveZScore) Next(
	in iter.Seq[core.Primitive[float64, float64]],
) iter.Seq[core.Primitive[CausalResidualResult, CausalResidualResult]] {
	return func(yield func(core.Primitive[CausalResidualResult, CausalResidualResult]) bool) {
		logged := func(yield func(core.Primitive[float64, float64]) bool) {
			carrier := &core.Carrier[float64]{}

			for arriving := range in {
				if !yield(carrier.Carrier(math.Log(arriving.Read()))) {
					return
				}
			}
		}

		for reading := range op.moments.Next(logged) {
			for result := range op.residual.Next(transport.One(reading)) {
				reading := result.Read()
				reading.Baseline = math.Exp(reading.Baseline)

				if !yield(op.Carrier(reading)) {
					return
				}
			}
		}

		op.Error(op.moments.Error(), op.residual.Error())
	}
}
