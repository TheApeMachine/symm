package temporal

import (
	"errors"
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
	err         error
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

func NewRenewalRate(target float64) core.Primitive {
	return &RenewalRate{target: target}
}

func (op *RenewalRate) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			input := *(*RenewalInput)(arriving)

			if input.Increment < 0 || input.Sample <= 0 || op.target <= 0 {
				op.err = errors.Join(op.err, core.ErrDomain)
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
				op.err = errors.Join(op.err, core.ErrDomain)
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

			op.out = reading

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *RenewalRate) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
