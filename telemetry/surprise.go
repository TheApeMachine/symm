package telemetry

import (
	"sync/atomic"

	"github.com/theapemachine/symm/logic"
)

type thresholdFnType func(source logic.SourceType) float64

var surpriseThresholdFn atomic.Pointer[thresholdFnType]

/*
SetSurpriseThresholdFn wires adaptive surprise bars from the runtime registry.
*/
func SetSurpriseThresholdFn(
	thresholdFn func(source logic.SourceType) float64,
) {
	if thresholdFn == nil {
		surpriseThresholdFn.Store(nil)
		return
	}

	fn := thresholdFnType(thresholdFn)
	surpriseThresholdFn.Store(&fn)
}

func gaugeSurpriseThreshold(source logic.SourceType) float64 {
	fnPointer := surpriseThresholdFn.Load()

	if fnPointer == nil {
		return 1.0
	}

	return (*fnPointer)(source)
}
