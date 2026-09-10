package equation

import (
	"iter"
	"math"
	"time"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
RenewalInput is one increment toward a quantity target.
*/
type RenewalInput struct {
	Increment float64
	Sample    float64
	At        int64
}

/*
RenewalReading is the rate after one observation. Closed distinguishes a
completed span from a retained prior rate.
*/
type RenewalReading struct {
	Rate     float64
	Change   float64
	Maturity float64
	Closed   bool
	Spans    float64
	Elapsed  float64
	Target   float64
}

/*
RenewalRate accumulates quantity until a configured target is reached and
positive time has elapsed. The target is a quantity, not a window.
*/
type RenewalRate struct {
	core.Base[RenewalInput, RenewalReading]
	target      float64
	origin      int64
	hasOrigin   bool
	accumulated float64
	spans       float64
	rate        float64
	lastSample  float64
	hasSample   bool
}

func NewRenewalRate(target float64) *RenewalRate {
	return &RenewalRate{target: target}
}

func (op *RenewalRate) Next(
	in iter.Seq[core.Primitive[RenewalInput, RenewalInput]],
) iter.Seq[core.Primitive[RenewalReading, RenewalReading]] {
	return func(yield func(core.Primitive[RenewalReading, RenewalReading]) bool) {
		for arriving := range in {
			input := arriving.Read()

			if input.Increment < 0 || input.Sample <= 0 || op.target <= 0 {
				op.Error(core.ErrDomain)
				return
			}

			if !op.hasOrigin {
				op.origin = input.At
				op.hasOrigin = true
			}

			op.accumulated += input.Increment
			elapsed := float64(input.At-op.origin) / float64(time.Second)
			reading := RenewalReading{
				Rate:     op.rate,
				Target:   op.target,
				Elapsed:  elapsed,
				Spans:    op.spans,
				Maturity: op.spans / (op.spans + 1),
			}

			if elapsed < 0 {
				op.Error(core.ErrDomain)
				return
			}

			if op.accumulated >= op.target && elapsed > 0 {
				reading.Rate = op.accumulated / elapsed
				reading.Closed = true
				reading.Spans = op.spans + 1
				reading.Maturity = reading.Spans / (reading.Spans + 1)

				if op.hasSample {
					reading.Change = math.Log(input.Sample / op.lastSample)
				}

				op.rate = reading.Rate
				op.spans = reading.Spans
				op.lastSample = input.Sample
				op.hasSample = true
				op.accumulated = 0
				op.origin = input.At
			}

			if !yield(op.Carrier(reading)) {
				return
			}
		}
	}
}
