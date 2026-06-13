package signal

import (
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/nomagique/learning"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
)

/*
AdaptiveSurpriseThreshold returns the live surprise bar for a source.
It replaces static signals.*.surprise_threshold YAML values.
*/
func AdaptiveSurpriseThreshold(source logic.SourceType) float64 {
	return market.GlobalSurpriseRegistry().Threshold(source)
}

/*
BoundedAdaptiveSurpriseThreshold clamps the adaptive surprise bar into the
classifier operating range.
*/
func BoundedAdaptiveSurpriseThreshold(source logic.SourceType) float64 {
	threshold := AdaptiveSurpriseThreshold(source)

	if threshold < 1 {
		return 1
	}

	if threshold > 5 {
		return 5
	}

	return threshold
}

/*
RefreshClassifierWeights updates classifier surprise sensitivity from the live gate.
*/
func RefreshClassifierWeights(
	source logic.SourceType,
	weights *learning.ClassifierWeights,
) {
	if weights == nil {
		return
	}

	weights.Threshold = BoundedAdaptiveSurpriseThreshold(source)
}

func ResolveComputeBatchInterval() time.Duration {
	interval := viper.GetDuration("trading.entry.batch_window")

	if interval <= 0 {
		interval = viper.GetDuration("telemetry.gauge.publish_interval")
	}

	if interval <= 0 {
		return 50 * time.Millisecond
	}

	return interval
}
