package types

import "github.com/theapemachine/nomagique/learning"

/*
ResonanceReturnForecast is a direction call over the largest contiguous
horizon whose issued calls beat a coin flip at the regulated confidence.
Call is -1, 0, or 1. Zero means no call. Distribution remains the head's
signed lean; size is not the published claim.
*/
type ResonanceReturnForecast struct {
	Distribution learning.RLSOutput
	Horizon      int
	ForwardCurve []float64
	Call         float64
}

/*
ResonanceReturnForecastKey identifies the calibrated forecast alongside the
symbol's live resonance manifold.
*/
const ResonanceReturnForecastKey = "return_forecast"
