package adaptive

import (
	"errors"
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
BaselineReading fixes causal scores and the post-observation moments.
*/
type BaselineReading struct {
	statistic.MomentReading
	HasPrior                                                                      bool
	Baseline, PriorVariance, ScoreScale, Residual, ZScore, Maturity, Retain, Span float64
}

/*
Baseline owns causal moments and the configured observation-driven window.
*/
type Baseline struct {
	err     error
	window  core.Primitive
	moments statistic.Moments
	out     BaselineReading
}

func NewBaseline(window core.Primitive) core.Primitive {
	return &Baseline{window: window}
}

func (op *Baseline) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			val := *(*float64)(arriving)
			reading := BaselineReading{MomentReading: op.moments.Update(val)}

			for wPtr := range op.window.Next(transport.NewOne(arriving).Next(nil)) {
				w := *(*WindowReading)(wPtr)
				reading.Retain = w.ShedRatio
				reading.Span = w.Capacity
			}

			op.moments.Shed(reading.Retain)
			reading.Summarize(op.moments)
			reading.HasPrior = reading.Prior.Count > 0
			reading.Baseline = val

			if reading.HasPrior {
				reading.Baseline = reading.Prior.Mean
			}

			if reading.Prior.Count > 1 {
				reading.PriorVariance = reading.Prior.M2 / (reading.Prior.Count - 1)
			}

			reading.ScoreScale = math.Sqrt(reading.PriorVariance)
			reading.Residual = val - reading.Baseline

			if reading.ScoreScale > 0 {
				reading.ZScore = reading.Residual / reading.ScoreScale
			}

			reading.Maturity = 1 - 1/(reading.Prior.Count+1)
			op.out = reading

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *Baseline) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	if op.window != nil {
		if err := op.window.Error(); err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
