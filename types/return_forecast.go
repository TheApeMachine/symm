package types

import "github.com/theapemachine/nomagique/learning"

/*
ResonanceReturnForecast is a direction call over the largest contiguous
horizon whose issued calls beat a coin flip at the regulated confidence.
Call is the currently actionable -1, 0, or 1 direction. StableCall retains
the accepted side across a weak reversal, but a held state always publishes
Call zero so continuity can never impersonate fresh trading conviction. The
distribution is the direction head's signed lean and uncertainty; it is not a
priced return.
*/
type ResonanceReturnForecast struct {
	Distribution     learning.RLSOutput
	Horizon          int
	CandidateCall    float64
	Call             float64
	StableCall       float64
	Held             bool
	SwitchConfidence float64
	SwitchThreshold  float64
}

/*
ResonanceReturnForecastKey identifies the calibrated forecast alongside the
symbol's live resonance manifold.
*/
const ResonanceReturnForecastKey = "return_forecast"
