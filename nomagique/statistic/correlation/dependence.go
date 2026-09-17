package correlation

import (
	"iter"
	"math"
	"time"
	"unsafe"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/core"
	sequence "github.com/theapemachine/symm/nomagique/data/sequence"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/temporal"
)

/*
DependenceReading preserves estimator fields and the path diagnostics around one
pair.
*/
type DependenceReading struct {
	LagEstimate
	LeftReturns     float64
	RightReturns    float64
	LeftEnergyRate  float64
	RightEnergyRate float64
	Defined         bool
	SharedTime      float64
	OverlapDensity  float64
}

/*
Dependence owns the typed path diagnostics surrounding an opaque estimator.
*/
type Dependence struct {
	*core.PrimitiveError

	estimator   core.Primitive
	pathReturns core.Primitive
	rate        core.Primitive
}

/*
NewDependence creates a new Dependence primitive over the supplied estimator.
*/
func NewDependence(estimator core.Primitive) *Dependence {
	return &Dependence{
		PrimitiveError: core.NewPrimitiveError(),
		estimator:      estimator,
		pathReturns:    temporal.NewPathReturns(),
		rate:           nomagique.NewNumber(temporal.NewEnergyRates(), statistic.NewMedian()),
	}
}

func (dependence *Dependence) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		defer func() {
			if dependence.pathReturns != nil {
				if err := dependence.pathReturns.Error(); err != nil {
					dependence.Error(err)
				}
			}
		}()

		for arriving := range in {
			input := (*LagProfileInput)(arriving)
			var left temporal.ReturnPath

			for out := range dependence.pathReturns.Next(sequence.NewValues(temporal.PricePath{Prices: input.Left}).Next(nil)) {
				left = *(*temporal.ReturnPath)(out)
			}

			if err := dependence.pathReturns.Error(); err != nil {
				dependence.Error(err)
				return
			}

			var right temporal.ReturnPath

			for out := range dependence.pathReturns.Next(sequence.NewValues(temporal.PricePath{Prices: input.Right}).Next(nil)) {
				right = *(*temporal.ReturnPath)(out)
			}

			if err := dependence.pathReturns.Error(); err != nil {
				dependence.Error(err)
				return
			}

			var estimate LagEstimate

			for out := range dependence.estimator.Next(sequence.NewValues(EstimateInput{
				Left:        left.Returns,
				Right:       right.Returns,
				LeftEnergy:  left.Energy,
				RightEnergy: right.Energy,
				Lag:         0,
			}).Next(nil)) {
				estimate = *(*LagEstimate)(out)
			}

			if err := dependence.estimator.Error(); err != nil {
				dependence.Error(err)
				return
			}

			shared, density := 0.0, 0.0

			if len(left.Returns) > 0 && len(right.Returns) > 0 {
				leftFrom := left.Returns[0].From
				leftThrough := left.Returns[len(left.Returns)-1].To
				rightFrom := right.Returns[0].From
				rightThrough := right.Returns[len(right.Returns)-1].To

				start := max(leftFrom, rightFrom)
				end := min(leftThrough, rightThrough)

				if end > start {
					shared = float64(end-start) / float64(time.Second)
				}
			}

			if shared > 0 {
				density = estimate.Support / shared
			}

			leftRate := math.NaN()

			if len(left.Returns) > 0 {
				inputs := make([]temporal.EnergyRateInput, len(left.Returns))

				for index, r := range left.Returns {
					inputs[index] = temporal.EnergyRateInput{
						Value: r.Value,
						From:  r.From,
						To:    r.To,
					}
				}

				for out := range dependence.rate.Next(sequence.NewValues(inputs...).Next(nil)) {
					leftRate = *(*float64)(out)
				}
			}

			rightRate := math.NaN()

			if len(right.Returns) > 0 {
				inputs := make([]temporal.EnergyRateInput, len(right.Returns))

				for index, r := range right.Returns {
					inputs[index] = temporal.EnergyRateInput{
						Value: r.Value,
						From:  r.From,
						To:    r.To,
					}
				}

				for out := range dependence.rate.Next(sequence.NewValues(inputs...).Next(nil)) {
					rightRate = *(*float64)(out)
				}
			}

			out := DependenceReading{
				LagEstimate:     estimate,
				LeftReturns:     float64(len(left.Returns)),
				RightReturns:    float64(len(right.Returns)),
				LeftEnergyRate:  leftRate,
				RightEnergyRate: rightRate,
				Defined:         estimate.Defined,
				SharedTime:      shared,
				OverlapDensity:  density,
			}

			if !yield(unsafe.Pointer(&out)) {
				return
			}
		}
	}
}
