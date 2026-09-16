package adaptive

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	sequence "github.com/theapemachine/symm/nomagique/data/sequence"
	"github.com/theapemachine/symm/nomagique/statistic"
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
	*core.PrimitiveError

	window  core.Primitive
	moments statistic.Moments
	out     BaselineReading
}

func NewBaseline(window core.Primitive) *Baseline {
	return &Baseline{PrimitiveError: core.NewPrimitiveError(), window: window}
}

func (baseline *Baseline) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		defer func() {

			if baseline.window != nil {
				if err := baseline.window.Error(); err != nil {
					baseline.Error(err)
				}
			}
		}()
		for arriving := range in {
			val := *(*float64)(arriving)
			reading := BaselineReading{MomentReading: baseline.moments.Update(val)}

			for wPtr := range baseline.window.Next(sequence.NewOne(arriving).Next(nil)) {
				w := *(*WindowReading)(wPtr)
				reading.Retain = w.ShedRatio
				reading.Span = w.Capacity
			}

			baseline.moments.Shed(reading.Retain)
			reading.Summarize(baseline.moments)
			reading.HasPrior = reading.Prior.Count > 0
			reading.Baseline = val

			if reading.HasPrior {
				reading.Baseline = reading.Prior.Mean
			}

			reading.ScoreScale = reading.Dispersion
			reading.Residual = val - reading.Baseline

			if reading.ScoreScale > 0 {
				reading.ZScore = reading.Residual / reading.ScoreScale
			}

			reading.Maturity = 1 - 1/(reading.Prior.Count+1)
			baseline.out = reading

			if !yield(unsafe.Pointer(&baseline.out)) {
				return
			}
		}
	}
}
