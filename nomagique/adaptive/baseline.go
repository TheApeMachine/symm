package adaptive

import (
	"iter"
	"math"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
)

/*
Baseline owns causal moments and the configured observation-driven window.
*/
type Baseline struct {
	core.Base[float64, BaselineReading]
	window  *Window
	moments equation.Moments
	Reading BaselineReading
}

/*
BaselineReading fixes causal scores and the post-observation moments.
*/
type BaselineReading struct {
	equation.MomentReading
	HasPrior                                                                      bool
	Baseline, PriorVariance, ScoreScale, Residual, ZScore, Maturity, Retain, Span float64
}

func NewBaseline(window *Window) *Baseline {
	return &Baseline{window: window}
}

func (op *Baseline) Next(
	in iter.Seq[core.Primitive[float64, float64]],
) iter.Seq[core.Primitive[BaselineReading, BaselineReading]] {
	return func(yield func(core.Primitive[BaselineReading, BaselineReading]) bool) {
		for arriving := range in {
			if !yield(op.Carrier(op.Observe(arriving.Read()))) {
				return
			}
		}
	}
}

/*
Observe scores against prior moments, then applies the window's support shedding.
*/
func (op *Baseline) Observe(value float64) BaselineReading {
	reading := BaselineReading{MomentReading: op.moments.Update(value)}
	window := op.window.Observe(value)
	reading.Retain = window.ShedRatio
	reading.Span = window.Capacity
	op.moments.Shed(reading.Retain)
	reading.Summarize(op.moments)
	reading.HasPrior = reading.Prior.Count > 0
	reading.Baseline = value

	if reading.HasPrior {
		reading.Baseline = reading.Prior.Mean
	}

	reading.Residual = value - reading.Baseline
	reading.ScoreScale = math.Abs(reading.Residual)

	if reading.Prior.Count > 1 {
		reading.PriorVariance = reading.Prior.M2 / (reading.Prior.Count - 1)
	}

	if reading.PriorVariance > 0 {
		reading.ScoreScale = math.Sqrt(reading.PriorVariance)
	}

	if reading.ScoreScale > 0 {
		reading.ZScore = reading.Residual / reading.ScoreScale
	}

	reading.Maturity = 1 - 1/(reading.Prior.Count+1)
	op.Reading = reading
	return reading
}
