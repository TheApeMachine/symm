package correlation

import (
	"iter"
	"time"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	sequence "github.com/theapemachine/symm/nomagique/data/sequence"
	"github.com/theapemachine/symm/nomagique/temporal"
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
	Defined     bool
}

/*
EstimateInput is one pair of decoded return paths, their energies, and the
timestamp offset at which the estimator is evaluated.
*/
type EstimateInput struct {
	Left        []temporal.LogReturn
	Right       []temporal.LogReturn
	LeftEnergy  float64
	RightEnergy float64
	Lag         int64
}

/*
LagProfileInput is two price paths searched at every lag.
*/
type LagProfileInput struct {
	Left  []temporal.Price
	Right []temporal.Price
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
*/
type LagProfile struct {
	*core.PrimitiveError

	estimator   core.Primitive
	pathReturns core.Primitive
	spacing     int64
	span        float64
}

/*
NewLagProfile creates a new LagProfile primitive over the supplied estimator.
*/
func NewLagProfile(estimator core.Primitive, spacing int64, span float64) *LagProfile {
	return &LagProfile{
		PrimitiveError: core.NewPrimitiveError(),
		estimator:      estimator,
		pathReturns:    temporal.NewPathReturns(),
		spacing:         spacing,
		span:            span,
	}
}

func (lagProfile *LagProfile) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		defer func() {
			if lagProfile.pathReturns != nil {
				if err := lagProfile.pathReturns.Error(); err != nil {
					lagProfile.Error(err)
				}
			}
		}()

		for arriving := range in {
			input := (*LagProfileInput)(arriving)
			var left temporal.ReturnPath

			for out := range lagProfile.pathReturns.Next(sequence.NewValues(temporal.PricePath{Prices: input.Left}).Next(nil)) {
				left = *(*temporal.ReturnPath)(out)
			}

			if err := lagProfile.pathReturns.Error(); err != nil {
				lagProfile.Error(err)
				return
			}

			var right temporal.ReturnPath

			for out := range lagProfile.pathReturns.Next(sequence.NewValues(temporal.PricePath{Prices: input.Right}).Next(nil)) {
				right = *(*temporal.ReturnPath)(out)
			}

			if err := lagProfile.pathReturns.Error(); err != nil {
				lagProfile.Error(err)
				return
			}

			limit := int(lagProfile.span*2 + 1)

			for index := 0; index < limit; index++ {
				lagIndex := float64(index) - lagProfile.span
				lag := int64(lagIndex * float64(lagProfile.spacing))
				var reading LagEstimate

				for out := range lagProfile.estimator.Next(sequence.NewValues(EstimateInput{
					Left:        left.Returns,
					Right:       right.Returns,
					LeftEnergy:  left.Energy,
					RightEnergy: right.Energy,
					Lag:         lag,
				}).Next(nil)) {
					reading = *(*LagEstimate)(out)
				}

				if err := lagProfile.estimator.Error(); err != nil {
					lagProfile.Error(err)
					return
				}

				candidate := LagCandidate{
					LagEstimate: reading,
					Index:       float64(index),
					LagIndex:    lagIndex,
					X:           float64(lag) / float64(time.Second),
					Y:           reading.Correlation,
				}

				if !yield(unsafe.Pointer(&candidate)) {
					return
				}
			}
		}
	}
}
