package types

import "github.com/theapemachine/nomagique/learning"

/*
ResonanceReturnForecast is the cumulative return distribution over the largest
contiguous horizon whose resolved predictions beat the zero-return baseline at
the regulated confidence.
*/
type ResonanceReturnForecast struct {
	Distribution learning.RLSOutput
	Horizon      int
}

/*
ResonanceReturnForecastKey identifies the calibrated forecast alongside the
symbol's live resonance manifold.
*/
const ResonanceReturnForecastKey = "return_forecast"
