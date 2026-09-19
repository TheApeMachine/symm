package learning

import (
	"fmt"
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/data/sequence"
	"github.com/theapemachine/symm/nomagique/probability"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/types"
)

/*
Pace is the adaptive learning rate controller, built entirely via functional composition
of canonical mathematical and statistical atoms.

It takes an error magnitude and yields a bounded exponential moving average of an
adapted learning rate, scaled by the empirical rank of the incoming error.
*/
func Pace(rest, lower, upper, gain, band float64, window int) types.Value[float64, float64] {
	return types.Value[float64, float64](nomagique.NewNumber(
		// 1. Maintain history and map error magnitude to empirical rank
		types.Value[float64, float64](probability.NewCalibrator(
			types.Value[[]float64, []float64](sequence.NewTail[float64](window)),
		)),

		// 2. Map the rank to a target log-alpha based on the bands
		types.Value[float64, float64](statistic.NewThreshold(band, math.Log(rest), math.Log(lower), math.Log(upper))),

		// 3. Smooth the target log-alpha with an Exponential Moving Average
		types.Value[float64, float64](statistic.NewEMA(gain)),

		// 4. Clamp the internal log-alpha to bounds
		types.Value[float64, float64](arithmetic.NewClamp(math.Log(lower), math.Log(upper))),

		// 5. Convert back from log-space to time-space
		types.Value[float64, float64](arithmetic.NewExp()),

		// 6. Clamp the final alpha to hard bounds
		types.Value[float64, float64](arithmetic.NewClamp(lower, upper)),
	))
}

// PacePrimitive wraps a Pace closure in the legacy core.Primitive interface.
type PacePrimitive struct {
	pipeline types.Value[float64, float64]
	err      error
}

func NewPacePrimitive(rest, lower, upper, gain, band float64, window int) *PacePrimitive {
	return &PacePrimitive{
		pipeline: Pace(rest, lower, upper, gain, band, window),
	}
}

func (p *PacePrimitive) Next(seq iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for v := range seq {
			if v == nil {
				continue
			}

			valPtr := (*float64)(v)
			if valPtr == nil {
				p.err = fmt.Errorf("pace: received nil value")
				break
			}

			res := p.pipeline(*valPtr)
			if !yield(unsafe.Pointer(&res)) {
				return
			}
		}
	}
}

func (p *PacePrimitive) Error(errs ...error) error {
	if len(errs) > 0 {
		p.err = errs[0]
	}
	return p.err
}
