package hawkes

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
IntensityInput is one side's pre-event intensity at a horizon.
*/
type IntensityInput struct {
	Parameters
	Side    float64
	Horizon float64
	Events  []Event
}

/*
IntensityResult is intensity and its beta derivative.
*/
type IntensityResult struct {
	Intensity     float64
	IntensityBeta float64
}

/*
Intensity evaluates one side's pre-event intensity and beta derivative.
*/
type Intensity struct {
	core.Base[IntensityInput, IntensityResult]
}

func NewIntensity() *Intensity {
	return &Intensity{}
}

func (op *Intensity) Next(
	in iter.Seq[core.Primitive[IntensityInput, IntensityInput]],
) iter.Seq[core.Primitive[IntensityResult, IntensityResult]] {
	return func(yield func(core.Primitive[IntensityResult, IntensityResult]) bool) {
		for arriving := range in {
			input := arriving.Read()
			mu, alphaX, alphaY := input.MuY, input.AlphaYX, input.AlphaYY

			if input.Side == 0 {
				mu, alphaX, alphaY = input.MuX, input.AlphaXX, input.AlphaXY
			}

			supportX, supportY := 0.0, 0.0
			supportXBeta, supportYBeta := 0.0, 0.0

			for _, event := range input.Events {
				if !(event.At < input.Horizon) {
					continue
				}

				age := input.Horizon - event.At
				k := kernel(input.Beta, age)
				kb := -age * k

				if event.Side == 0 {
					supportX += k
					supportXBeta += kb
					continue
				}

				supportY += k
				supportYBeta += kb
			}

			if !yield(op.Carrier(IntensityResult{
				Intensity:     mu + alphaX*supportX + alphaY*supportY,
				IntensityBeta: alphaX*supportXBeta + alphaY*supportYBeta,
			})) {
				return
			}
		}
	}
}
