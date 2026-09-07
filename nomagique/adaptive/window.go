package adaptive

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/transport"
	"math"
)

/*
Window owns the all/recent moment approximation of the existing mean-shift
policy. It does not claim to implement the full ADWIN algorithm. Its state is
fixed numeric storage; no record maps or computation graphs grow per observation.
*/
type Window struct {
	*transport.Map
	all, recent            equation.Moments
	observations, capacity float64
	Reading                WindowReading
}

/* WindowReading is the immutable numeric result of one policy transition. */
type WindowReading struct {
	core.PrimitiveError
	All, Recent                              equation.MomentReading
	Value, Capacity, Observations, ShedRatio float64
	Variance, RecentCount, PriorCount        float64
}

/* NewWindow creates an independent policy with the two-sample variance minimum. */
func NewWindow() *Window {
	window := &Window{capacity: 2}
	window.Map = transport.NewMap(&windowStep{
		window: window, seed: transport.NewIO(&WindowReading{}),
	})
	return window
}

/* Observe advances the policy directly over unboxed numeric state. */
func (window *Window) Observe(value float64) WindowReading {
	window.observations++
	window.capacity++
	reading := WindowReading{
		All: window.all.Update(value), Recent: window.recent.Update(value),
		Value: value, Observations: window.observations, ShedRatio: 1,
	}
	// The comparison uses two subwindows; keep recent support within its half.
	if window.observations > 3 && window.recent.Count > window.capacity*0.5 {
		window.recent.Shed(0.5)
		reading.Recent.Summarize(window.recent)
	}
	reading.Variance = reading.All.Variance
	reading.RecentCount = window.recent.Count
	reading.PriorCount = window.capacity - reading.RecentCount

	if window.observations > 3 && reading.RecentCount > 1 && reading.PriorCount > 1 && reading.Variance > 0 {
		shift := equation.MeanShift{
			Variance: reading.Variance, Observations: window.observations,
			RecentCount: reading.RecentCount, PriorCount: reading.PriorCount,
		}
		bound := shift.Bound()
		if math.Abs(window.recent.Mean-window.all.Mean) > bound {
			capacity := math.Max(2, math.Floor(window.capacity*0.5))
			reading.ShedRatio = capacity / window.capacity
			window.capacity = capacity
			window.all.Shed(reading.ShedRatio)
			reading.All.Summarize(window.all)
			window.recent = equation.Moments{}
			reading.Recent = equation.MomentReading{}
		}
	}
	reading.Capacity = window.capacity
	window.Reading = reading
	return reading
}

/* Read materializes the named record only for a generic Primitive consumer. */
func (reading *WindowReading) Read() any {
	return core.To[map[string]core.Primitive](core.Record(map[string]any{
		"all": reading.All.Fields(), "recent": reading.Recent.Fields(),
		"value": reading.Value, "capacity": reading.Capacity, "observations": reading.Observations,
		"shed_ratio": reading.ShedRatio, "variance": reading.Variance,
		"recent_count": reading.RecentCount, "prior_count": reading.PriorCount,
	}))
}
func (reading *WindowReading) Next(core.Primitive) core.Primitive { return nil }

/* windowStep binds numeric observations into Primitive delivery without changing the recurrence. */
type windowStep struct {
	core.PrimitiveError
	window *Window
	seed   *transport.IO
}

func (step *windowStep) Next(input core.Primitive) core.Primitive {
	return core.Yield(step.seed, input, func(_ core.Primitive, value float64) core.Primitive {
		reading := step.window.Observe(value)
		return &reading
	}, step)
}
func (step *windowStep) Read() any { return step.window.Reading.Read() }
