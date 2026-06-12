package depthflow

import (
	"sync"

	"github.com/spf13/viper"
)

var (
	spoofWeightedThresholdOnce sync.Once
	spoofWeightedThreshold     float64
)

func loadSpoofWeightedThreshold() float64 {
	spoofWeightedThresholdOnce.Do(func() {
		seed := viper.GetFloat64("signals.spoof_weighted_threshold")

		if seed > 0 && seed < 1 {
			spoofWeightedThreshold = seed
			return
		}

		spoofWeightedThreshold = 0.5
	})

	return spoofWeightedThreshold
}
