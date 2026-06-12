package depthflow

import (
	"fmt"
	"sync"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
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

		errnie.Info(fmt.Sprintf(
			"depthflow: invalid signals.spoof_weighted_threshold %v, using default %v",
			seed,
			0.5,
		))

		spoofWeightedThreshold = 0.5
	})

	return spoofWeightedThreshold
}
