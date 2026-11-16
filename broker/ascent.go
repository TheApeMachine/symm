package broker

import "math"

/*
Ascent is the wire snapshot of path memory for the hub/UI. Live learning lives
on path (nomagique CUSUM / MoveBaseline / Momentum / TimeElastic / Hysteresis);
Stoploss copies readings here each mark.
*/
type Ascent struct {
	PeakVelocity      float64 `json:"PeakVelocity"`
	SmoothVelocity    float64 `json:"SmoothVelocity"`
	MeanUp            float64 `json:"MeanUp"`
	MeanDown          float64 `json:"MeanDown"`
	PeakPnLPct        float64 `json:"PeakPnLPct"`
	OpenPullback      float64 `json:"OpenPullback"`
	RecoveredPullback float64 `json:"RecoveredPullback"`
	LastPnLPct        float64 `json:"LastPnLPct"`
	Samples           int     `json:"Samples"`
	CusumPos          float64 `json:"CusumPos,omitempty"`
	CusumNeg          float64 `json:"CusumNeg,omitempty"`
}

func (ascent *Ascent) absorb(reading pathReading) {
	ascent.MeanUp = reading.meanUp
	ascent.MeanDown = reading.meanDown
	ascent.PeakPnLPct = reading.peak
	ascent.OpenPullback = reading.openPull
	ascent.RecoveredPullback = reading.recovered
	ascent.LastPnLPct = reading.last
	ascent.Samples = reading.samples
	ascent.CusumPos = reading.cusumPos
	ascent.CusumNeg = reading.cusumNeg
	ascent.SmoothVelocity = reading.momentum
	if reading.momentum > ascent.PeakVelocity {
		ascent.PeakVelocity = reading.momentum
	}
}

/*
NormalDown is the learned down envelope for chart giveback bands.
*/
func (ascent *Ascent) NormalDown() float64 {
	return math.Max(ascent.RecoveredPullback, ascent.MeanDown)
}

/*
Exhaust is down-drift share of CUSUM evidence (0..1) — derived, not a cutoff.
*/
func (ascent *Ascent) Exhaust() float64 {
	total := ascent.CusumPos + ascent.CusumNeg
	if total <= 0 {
		return 0
	}
	return ascent.CusumNeg / total
}

func (ascent *Ascent) Reset() {
	*ascent = Ascent{}
}
