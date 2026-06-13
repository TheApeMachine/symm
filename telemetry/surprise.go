package telemetry

import "github.com/theapemachine/symm/logic"

var surpriseThresholdFn func(source logic.SourceType) float64

/*
SetSurpriseThresholdFn wires adaptive surprise bars from the runtime registry.
*/
func SetSurpriseThresholdFn(
	thresholdFn func(source logic.SourceType) float64,
) {
	surpriseThresholdFn = thresholdFn
}

func gaugeSurpriseThreshold(source logic.SourceType) float64 {
	if surpriseThresholdFn != nil {
		return surpriseThresholdFn(source)
	}

	return 1.0
}
