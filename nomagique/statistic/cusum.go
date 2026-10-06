package statistic

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
CUSUM signal values published in slot [0] of a CUSUM reading.
*/
const (
	CUSUMNone  = 0.0
	CUSUMUpper = 1.0
	CUSUMLower = -1.0
)

/*
CUSUM implements Page's two-sided cumulative sum control filter (1954).
It accumulates directional innovations exceeding a friction/noise hurdle,
tracking structural departures without arbitrary polling horizons or single-tick hair-triggers.

Each arrival is one sequentially stamped observation and its operating
hurdles as *[4]float64 {sequence, value, hurdle, threshold}. It yields the
control state as *[5]float64
{signal, upper sum, lower sum, upper start, lower start}, where signal is
CUSUMNone, CUSUMUpper or CUSUMLower.
*/
type CUSUM struct {
	*core.PrimitiveError
	observed   bool
	previous   float64
	upper      float64
	lower      float64
	upperStart float64
	lowerStart float64
	out        [5]float64
}

func NewCUSUM() *CUSUM {
	return &CUSUM{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

func (op *CUSUM) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			obs := (*[4]float64)(arriving)
			sequence, value, hurdle, threshold := obs[0], obs[1], obs[2], obs[3]

			if !op.observed {
				op.previous = value
				op.observed = true
				op.upperStart = sequence
				op.lowerStart = sequence
				op.out = [5]float64{CUSUMNone, 0, 0, sequence, sequence}

				if !yield(unsafe.Pointer(&op.out)) {
					return
				}

				continue
			}

			delta := value - op.previous
			op.previous = value

			if op.upper == 0 && delta > hurdle {
				op.upperStart = sequence - 1
			}

			op.upper += delta - hurdle

			if op.upper < 0 {
				op.upper = 0
				op.upperStart = sequence
			}

			if op.lower == 0 && delta < -hurdle {
				op.lowerStart = sequence - 1
			}

			op.lower += delta + hurdle

			if op.lower > 0 {
				op.lower = 0
				op.lowerStart = sequence
			}

			op.out = [5]float64{CUSUMNone, op.upper, op.lower, op.upperStart, op.lowerStart}

			if threshold > 0 {
				if op.upper >= threshold {
					op.out[0] = CUSUMUpper
					op.upper = 0
					op.upperStart = sequence
				}

				if op.lower <= -threshold {
					op.out[0] = CUSUMLower
					op.lower = 0
					op.lowerStart = sequence
				}
			}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}
