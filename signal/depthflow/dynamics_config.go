package depthflow

import (
	"math"
	"sync/atomic"
)

var spoofWeightedThreshold atomic.Uint64

func loadSpoofWeightedThreshold() float64 {
	if bits := spoofWeightedThreshold.Load(); bits != 0 {
		return math.Float64frombits(bits)
	}

	return 0
}

func seedSpoofWeightedThreshold(ratio float64) {
	if ratio <= 0 || ratio >= 1 {
		return
	}

	spoofWeightedThreshold.CompareAndSwap(0, math.Float64bits(ratio))
}
