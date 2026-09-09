package adaptive

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/transport"
	"math"
)

/*
Baseline owns causal moments and the configured observation-driven window.
Grid cells retain these fixed fields rather than copies of a record-processing
interpreter. Primitive delivery and typed observations use the same recurrence.
*/
type Baseline struct {
	*transport.Map
	window  *Window
	moments equation.Moments
	Reading BaselineReading
}

/* BaselineReading fixes causal scores and the post-observation moments. */
type BaselineReading struct {
	equation.MomentReading
	HasPrior                                                                bool
	Baseline, PriorVariance, ScoreScale, Residual, ZScore, Maturity, Retain float64
}

/* NewBaseline configures one independent causal baseline and window. */
func NewBaseline(window *Window) *Baseline {
	baseline := &Baseline{window: window}
	baseline.Map = transport.NewMap(&baselineStep{
		baseline: baseline, seed: transport.NewIO(core.From[any](nil)),
	})
	return baseline
}

/* Observe scores against prior moments, then applies the window's support shedding. */
func (baseline *Baseline) Observe(value float64) BaselineReading {
	reading := BaselineReading{MomentReading: baseline.moments.Update(value)}
	window := baseline.window.Observe(value)
	reading.Retain = window.ShedRatio
	baseline.moments.Shed(reading.Retain)
	reading.Summarize(baseline.moments)
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
	baseline.Reading = reading
	return reading
}

/* Fields materializes one immutable delivery at the Primitive boundary. */
func (reading BaselineReading) Fields() map[string]core.Primitive {
	fields := reading.MomentReading.Fields()
	fields["has_prior"] = core.From(reading.HasPrior)
	fields["baseline"] = core.From(reading.Baseline)
	fields["prior_variance"] = core.From(reading.PriorVariance)
	fields["score_scale"] = core.From(reading.ScoreScale)
	fields["residual"] = core.From(reading.Residual)
	fields["zscore"] = core.From(reading.ZScore)
	fields["maturity"] = core.From(reading.Maturity)
	fields["retain"] = core.From(reading.Retain)
	fields["noise_variance"] = core.From(reading.Variance)
	return fields
}

/* baselineStep owns Primitive delivery for a configured typed baseline. */
type baselineStep struct {
	core.PrimitiveError
	baseline *Baseline
	seed     *transport.IO
}

func (step *baselineStep) Next(input core.Primitive) core.Primitive {
	return core.Yield(step.seed, input, func(_ core.Primitive, value float64) core.Primitive {
		reading := step.baseline.Observe(value)
		return core.From(reading.Fields())
	}, step)
}
func (step *baselineStep) Read() any { return step.baseline.Reading.Fields() }
