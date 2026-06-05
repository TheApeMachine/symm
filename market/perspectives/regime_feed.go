package perspectives

import "sync/atomic"

var currentRegime atomic.Uint32

/*
PublishRegime stores the latest price-action regime for self-calibrating signals.
Story publishes on every measurement; calibrators read it when refitting band edges.
*/
func PublishRegime(regime Regime) {
	currentRegime.Store(uint32(regime))
}

/*
CurrentRegime returns the regime Story last published, or RegimeNone before the first tick.
*/
func CurrentRegime() Regime {
	return Regime(currentRegime.Load())
}

/*
ResetRegimeForTest clears the published regime between isolated tests.
*/
func ResetRegimeForTest() {
	currentRegime.Store(uint32(RegimeNone))
}
