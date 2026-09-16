package temporal

import (
	"iter"
	"math"
	"time"
	"unsafe"

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
RenewalReading is the rate after one observation.
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
RenewalRate accumulates quantity until a configured target is reached.
*/
type RenewalRate struct {
	*core.PrimitiveError

	target      float64
	origin      int64
	hasOrigin   bool
	accumulated float64
	spans       float64
	rate        float64
	lastSample  float64
	hasSample   bool
	out         RenewalReading
}

func NewRenewalRate(target float64) *RenewalRate {
	return &RenewalRate{PrimitiveError: core.NewPrimitiveError(), target: target}
}

func (renewalRate *RenewalRate) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			input := *(*RenewalInput)(arriving)

			if input.Increment < 0 || input.Sample <= 0 || renewalRate.target <= 0 {
				renewalRate.Error(core.ErrDomain)
				return
			}

			if !renewalRate.hasOrigin {
				renewalRate.origin = input.At
				renewalRate.hasOrigin = true
			}

			renewalRate.accumulated += input.Increment
			elapsed := float64(input.At-renewalRate.origin) / float64(time.Second)
			reading := RenewalReading{
				Rate:     renewalRate.rate,
				Target:   renewalRate.target,
				Elapsed:  elapsed,
				Spans:    renewalRate.spans,
				Maturity: renewalRate.spans / (renewalRate.spans + 1),
			}

			if elapsed < 0 {
				renewalRate.Error(core.ErrDomain)
				return
			}

			if renewalRate.accumulated >= renewalRate.target && elapsed > 0 {
				reading.Rate = renewalRate.accumulated / elapsed
				reading.Closed = true
				reading.Spans = renewalRate.spans + 1
				reading.Maturity = reading.Spans / (reading.Spans + 1)

				if renewalRate.hasSample {
					reading.Change = math.Log(input.Sample / renewalRate.lastSample)
				}

				renewalRate.rate = reading.Rate
				renewalRate.spans = reading.Spans
				renewalRate.lastSample = input.Sample
				renewalRate.hasSample = true
				renewalRate.accumulated = 0
				renewalRate.origin = input.At
			}

			renewalRate.out = reading

			if !yield(unsafe.Pointer(&renewalRate.out)) {
				return
			}
		}
	}
}
