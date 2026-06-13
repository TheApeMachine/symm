package calibration

import (
	"sync/atomic"

	"github.com/theapemachine/symm/logic"
)

var globalRegistry atomic.Pointer[Registry]

/*
Register installs the process-wide calibration registry.
*/
func Register(registry *Registry) {
	globalRegistry.Store(registry)
}

/*
Global returns the installed calibration registry.
*/
func Global() *Registry {
	return globalRegistry.Load()
}

/*
WireLogic connects calibration lookups into logic evidence resolution.
*/
func WireLogic() {
	registry := Global()

	if registry == nil {
		return
	}

	logic.CalibrationRegistryFn = func(
		source logic.SourceType,
		category logic.CategoryType,
		categoryConfidence float64,
	) (float64, float64, bool) {
		edgeConfidence := registry.EdgeConfidence(source, category, categoryConfidence)
		expectedMove, moveOK := registry.ExpectedMoveBps(source, category)

		if edgeConfidence <= 0 && !moveOK {
			return 0, 0, false
		}

		return edgeConfidence, expectedMove, edgeConfidence > 0 || moveOK
	}
}
