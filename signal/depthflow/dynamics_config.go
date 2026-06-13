package depthflow

import (
	"math"
	"sync/atomic"

	"github.com/spf13/viper"
)

var spoofWeightedThreshold atomic.Uint64

func loadSpoofWeightedThreshold() float64 {
	if bits := spoofWeightedThreshold.Load(); bits != 0 {
		return math.Float64frombits(bits)
	}

	seed := viper.GetFloat64("signals.spoof_weighted_threshold")

	if seed > 0 && seed < 1 {
		if spoofWeightedThreshold.CompareAndSwap(0, math.Float64bits(seed)) {
			return seed
		}

		return math.Float64frombits(spoofWeightedThreshold.Load())
	}

	return 0
}

func seedSpoofWeightedThreshold(ratio float64) {
	if ratio <= 0 || ratio >= 1 {
		return
	}

	spoofWeightedThreshold.CompareAndSwap(0, math.Float64bits(ratio))
}
