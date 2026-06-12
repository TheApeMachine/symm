package depthflow

import (
	"math"

	"github.com/spf13/viper"
	"github.com/theapemachine/nomagique"
	"github.com/theapemachine/nomagique/statistic"
)

func spoofContrastRatio(weightedHistory, level1History []float64) float64 {
	if len(weightedHistory) >= 3 && len(level1History) >= 3 {
		weightedMedian := float64(statistic.NewMedianAbsolute(nil).Observe(nomagique.Numbers(weightedHistory...)...))
		level1Median := float64(statistic.NewMedianAbsolute(nil).Observe(nomagique.Numbers(level1History...)...))
		denominator := weightedMedian + level1Median

		if denominator > 0 {
			return weightedMedian / denominator
		}
	}

	seed := viper.GetFloat64("signals.spoof_weighted_threshold")

	if seed > 0 && seed < 1 {
		return seed
	}

	return 0.5
}

func thinningDepthGate(weightedHistory, flatHistory []float64) float64 {
	if len(weightedHistory) >= 3 && len(flatHistory) >= 3 {
		weightedMedian := float64(statistic.NewMedianAbsolute(nil).Observe(nomagique.Numbers(weightedHistory...)...))
		flatMedian := float64(statistic.NewMedianAbsolute(nil).Observe(nomagique.Numbers(flatHistory...)...))

		if weightedMedian > 0 {
			return flatMedian / weightedMedian
		}
	}

	return 0.5
}

func loadedPressureScale(tradePressure, weightedThreshold float64) float64 {
	if weightedThreshold <= 0 {
		return 1
	}

	confirmWeight := math.Abs(tradePressure) / (math.Abs(tradePressure) + weightedThreshold)

	return 1 + confirmWeight*tradePressure
}
