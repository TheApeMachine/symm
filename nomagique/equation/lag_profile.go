package equation

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
LagEstimate is what an estimator publishes for one timestamp offset.
*/
type LagEstimate struct {
	Correlation float64
	Covariance  float64
	Support     float64
	LeftEnergy  float64
	RightEnergy float64
}

/*
Defined is overlap with two positive energies. Zero-energy normalization stays
undefined.
*/
func (estimate LagEstimate) Defined() bool {
	return estimate.Support > 0 && estimate.LeftEnergy > 0 && estimate.RightEnergy > 0
}

/*
LagEstimator consumes borrowed, already decoded return paths for one timestamp
offset. It must finish reading them before returning.
*/
type LagEstimator interface {
	Estimate(left, right *LogReturns, lag int64) (LagEstimate, error)
}

/*
LagProfileInput is two price paths searched at every lag.
*/
type LagProfileInput struct {
	Left  []Price
	Right []Price
}

/*
LagCandidate retains the complete estimator record and its own support.
*/
type LagCandidate struct {
	LagEstimate
	Index    float64
	LagIndex float64
	X        float64
	Y        float64
}

/*
LagProfile owns the configured estimator and exact discrete search coordinates.
Spacing is nanoseconds; span counts steps on either side.
*/
type LagProfile struct {
	core.Base[LagProfileInput, LagCandidate]
	estimator LagEstimator
	spacing   int64
	span      float64
	paths     [2]LogReturns
}

func NewLagProfile(estimator LagEstimator, spacing int64, span float64) *LagProfile {
	return &LagProfile{estimator: estimator, spacing: spacing, span: span}
}

func (op *LagProfile) Next(
	in iter.Seq[core.Primitive[LagProfileInput, LagProfileInput]],
) iter.Seq[core.Primitive[LagCandidate, LagCandidate]] {
	return func(yield func(core.Primitive[LagCandidate, LagCandidate]) bool) {
		for arriving := range in {
			input := arriving.Read()

			if err := op.paths[0].Load(input.Left); err != nil {
				op.Error(err)
				return
			}

			if err := op.paths[1].Load(input.Right); err != nil {
				op.Error(err)
				return
			}

			limit := int(op.span*2 + 1)

			for index := 0; index < limit; index++ {
				candidate, err := op.candidate(float64(index))

				if err != nil {
					op.Error(err)
					return
				}

				if !yield(op.Carrier(candidate)) {
					return
				}
			}
		}
	}
}

func (op *LagProfile) candidate(index float64) (LagCandidate, error) {
	lagIndex := index - op.span
	lag := int64(lagIndex * float64(op.spacing))
	estimate, err := op.estimator.Estimate(&op.paths[0], &op.paths[1], lag)

	if err != nil {
		return LagCandidate{}, err
	}

	return LagCandidate{
		LagEstimate: estimate,
		Index:       index,
		LagIndex:    lagIndex,
		X:           float64(lag) * 1e-9,
		Y:           estimate.Correlation,
	}, nil
}
