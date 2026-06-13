package depthflow

import (
	"fmt"
	"math"
	"sync/atomic"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
)

var spoofWeightedThreshold atomic.Uint64

func loadSpoofWeightedThreshold() float64 {
	if bits := spoofWeightedThreshold.Load(); bits != 0 {
		return math.Float64frombits(bits)
	}

	seed := viper.GetFloat64("signals.spoof_weighted_threshold")
	threshold := 0.5

	if seed > 0 && seed < 1 {
		threshold = seed
	} else {
		errnie.Info(fmt.Sprintf(
			"depthflow: invalid signals.spoof_weighted_threshold %v, using default %v",
			seed,
			threshold,
		))
	}

	if spoofWeightedThreshold.CompareAndSwap(0, math.Float64bits(threshold)) {
		return threshold
	}

	return math.Float64frombits(spoofWeightedThreshold.Load())
}
