package broker

import (
	"time"
)

/*
Ascent tracks smoothed PnL velocity (acceleration/deceleration) so exhaustion
is relative to this position's own peak climb rate, immune to discrete tick jitter.
*/
type Ascent struct {
	PeakVelocity   float64   `json:"PeakVelocity"`
	SmoothVelocity float64   `json:"SmoothVelocity"`
	LastPnLPct     float64   `json:"LastPnLPct"`
	LastTime       time.Time `json:"LastTime"`
	Samples        int       `json:"Samples"`
}

/*
Note advances the velocity tracker with the latest PnL and event timestamp.
Velocity is measured as % PnL gain per second (or unit time), smoothed with an EMA.
*/
func (ascent *Ascent) Note(pnlPct float64, at time.Time) {
	if at.IsZero() {
		at = time.Now()
	}

	if ascent.LastTime.IsZero() || at.Before(ascent.LastTime) {
		ascent.LastPnLPct = pnlPct
		ascent.LastTime = at
		ascent.Samples = 1
		return
	}

	// Unchanged PnL (L3 noise on lower levels) must not invent climb decay.
	if pnlPct == ascent.LastPnLPct {
		return
	}

	dt := at.Sub(ascent.LastTime).Seconds()

	if dt < 0.05 { // Clamp micro-burst jitter (<50ms)
		dt = 0.05
	}

	deltaPnL := pnlPct - ascent.LastPnLPct
	ascent.LastPnLPct = pnlPct
	ascent.LastTime = at
	ascent.Samples++

	// Instantaneous rate of climb (% per sec)
	instantVelocity := deltaPnL / dt

	// Adaptive smoothing: alpha responds faster during the early climb
	alpha := 0.25
	
	if ascent.Samples < 5 {
		alpha = 0.5
	}

	// Smooth velocity using EMA
	ascent.SmoothVelocity = (1-alpha)*ascent.SmoothVelocity + alpha*instantVelocity

	// Peak velocity only tracks positive acceleration phases
	if ascent.SmoothVelocity > ascent.PeakVelocity {
		ascent.PeakVelocity = ascent.SmoothVelocity
	}
}

/*
Exhaust returns 0.0 (full climb) to 1.0 (stalled/exhausted).
If the current smoothed climb rate has collapsed to a fraction of peak climb rate,
exhaustion approaches 1.0.
*/
func (ascent *Ascent) Exhaust() float64 {
	// Tiny peak velocity used to mean "not exhausted yet" forever — slow
	// grind arms never tripped. After enough samples, a flat/negative tape
	// on a weak climb is exhaustion.
	if ascent.PeakVelocity <= 0.001 {
		if ascent.Samples >= 8 && ascent.SmoothVelocity <= 0.001 {
			return 1.0
		}

		return 0
	}

	if ascent.SmoothVelocity >= ascent.PeakVelocity {
		return 0
	}

	if ascent.SmoothVelocity <= 0 {
		return 1.0
	}

	return 1.0 - (ascent.SmoothVelocity / ascent.PeakVelocity)
}

func (ascent *Ascent) Reset() {
	*ascent = Ascent{}
}
