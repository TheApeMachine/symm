package statistic

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

type CUSUMSignal int

const (
	CUSUMNone CUSUMSignal = iota
	CUSUMUpper
	CUSUMLower
)

/*
CUSUMObservation carries one sequentially stamped observation and its operating hurdles.
*/
type CUSUMObservation struct {
	Sequence  int64
	Value     float64
	Hurdle    float64
	Threshold float64
}

/*
CUSUMReading reports Page's two-sided cumulative sum control state.
*/
type CUSUMReading struct {
	Signal     CUSUMSignal
	UpperSum   float64
	LowerSum   float64
	UpperStart int64
	LowerStart int64
}

/*
CUSUM implements Page's two-sided cumulative sum control filter (1954).
It accumulates directional innovations exceeding a friction/noise hurdle,
tracking structural departures without arbitrary polling horizons or single-tick hair-triggers.
*/
type CUSUM struct {
	*core.PrimitiveError
	observed   bool
	previous   float64
	upper      float64
	lower      float64
	upperStart int64
	lowerStart int64
	out        CUSUMReading
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

			obs := *(*CUSUMObservation)(arriving)

			if !op.observed {
				op.previous = obs.Value
				op.observed = true
				op.upperStart = obs.Sequence
				op.lowerStart = obs.Sequence

				op.out = CUSUMReading{
					UpperStart: obs.Sequence,
					LowerStart: obs.Sequence,
				}

				if !yield(unsafe.Pointer(&op.out)) {
					return
				}

				continue
			}

			delta := obs.Value - op.previous
			op.previous = obs.Value

			if op.upper == 0 && delta > obs.Hurdle {
				op.upperStart = obs.Sequence - 1
			}

			op.upper += delta - obs.Hurdle

			if op.upper < 0 {
				op.upper = 0
				op.upperStart = obs.Sequence
			}

			if op.lower == 0 && delta < -obs.Hurdle {
				op.lowerStart = obs.Sequence - 1
			}

			op.lower += delta + obs.Hurdle

			if op.lower > 0 {
				op.lower = 0
				op.lowerStart = obs.Sequence
			}

			op.out = CUSUMReading{
				Signal:     CUSUMNone,
				UpperSum:   op.upper,
				LowerSum:   op.lower,
				UpperStart: op.upperStart,
				LowerStart: op.lowerStart,
			}

			if obs.Threshold > 0 {
				if op.upper >= obs.Threshold {
					op.out.Signal = CUSUMUpper
					op.upper = 0
					op.upperStart = obs.Sequence
				}

				if op.lower <= -obs.Threshold {
					op.out.Signal = CUSUMLower
					op.lower = 0
					op.lowerStart = obs.Sequence
				}
			}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}
