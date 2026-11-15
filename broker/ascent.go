package broker

import (
	"math"
	"time"
)

/*
Ascent tracks smoothed PnL velocity so exhaustion is relative to this
position's own peak climb rate. SmoothDelta/MeanDt are EMA'd separately so a
1ms print cannot spike PeakVelocity by 1000×.
*/
type Ascent struct {
	PeakVelocity   float64   `json:"PeakVelocity"`
	SmoothVelocity float64   `json:"SmoothVelocity"`
	SmoothDelta    float64   `json:"SmoothDelta"`
	MeanUp         float64   `json:"MeanUp"`
	MeanDt         float64   `json:"MeanDt"`
	LastPnLPct     float64   `json:"LastPnLPct"`
	LastTime       time.Time `json:"LastTime"`
	Samples        int       `json:"Samples"`
}

/*
Seed stores the latest pre-arm PnL so a gap through the arm floor has a real
initial upward delta instead of starting blind with PeakVelocity == 0.
*/
func (ascent *Ascent) Seed(pnlPct float64, at time.Time) {
	if at.IsZero() {
		at = time.Now()
	}

	*ascent = Ascent{
		LastPnLPct: pnlPct,
		LastTime:   at,
	}
}

/*
Note advances velocity with the latest PnL. Unchanged PnL is ignored so L3
noise does not invent climb decay.
*/
func (ascent *Ascent) Note(pnlPct float64, at time.Time) {
	if at.IsZero() {
		at = time.Now()
	}

	if ascent.LastTime.IsZero() || at.Before(ascent.LastTime) {
		ascent.LastPnLPct = pnlPct
		ascent.LastTime = at
		ascent.Samples = 1
		ascent.SmoothDelta = 0
		ascent.MeanDt = 0
		ascent.MeanUp = 0
		ascent.SmoothVelocity = 0
		ascent.PeakVelocity = 0
		return
	}

	if pnlPct == ascent.LastPnLPct {
		return
	}

	dt := at.Sub(ascent.LastTime).Seconds()

	if dt <= 0 {
		return
	}

	deltaPnL := pnlPct - ascent.LastPnLPct
	ascent.LastPnLPct = pnlPct
	ascent.LastTime = at

	// Seeded pre-arm (Samples == 0 with LastTime set): first post-arm delta.
	if ascent.Samples == 0 {
		ascent.Samples = 2
		ascent.SmoothDelta = deltaPnL
		ascent.MeanDt = dt

		if deltaPnL > 0 {
			ascent.MeanUp = deltaPnL
		}

		ascent.SmoothVelocity = ascent.SmoothDelta / ascent.MeanDt
		return
	}

	ascent.Samples++
	alpha := 2.0 / (math.Sqrt(float64(ascent.Samples)) + 1.0)

	if ascent.Samples == 2 {
		ascent.SmoothDelta = deltaPnL
		ascent.MeanDt = dt

		if deltaPnL > 0 {
			ascent.MeanUp = deltaPnL
		}

		ascent.SmoothVelocity = ascent.SmoothDelta / ascent.MeanDt
		return
	}

	ascent.SmoothDelta = (1-alpha)*ascent.SmoothDelta + alpha*deltaPnL
	ascent.MeanDt = (1-alpha)*ascent.MeanDt + alpha*dt

	if deltaPnL > 0 {
		ascent.MeanUp = (1-alpha)*ascent.MeanUp + alpha*deltaPnL
	}

	if ascent.MeanDt > 0 {
		ascent.SmoothVelocity = ascent.SmoothDelta / ascent.MeanDt
	}

	if ascent.SmoothVelocity > ascent.PeakVelocity {
		ascent.PeakVelocity = ascent.SmoothVelocity
	}
}

/*
Exhaust is 0 (full climb / no peak yet) to 1 (stalled) relative to peak velocity.
Grind arms with PeakVelocity <= 0 stay at 0 — they never invent full exhaustion.
*/
func (ascent *Ascent) Exhaust() float64 {
	if ascent.PeakVelocity <= 0 {
		return 0
	}

	if ascent.SmoothVelocity >= ascent.PeakVelocity {
		return 0
	}

	if ascent.SmoothVelocity <= 0 {
		return 1
	}

	return 1 - ascent.SmoothVelocity/ascent.PeakVelocity
}

func (ascent *Ascent) Reset() {
	*ascent = Ascent{}
}
