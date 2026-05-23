package pumpdump

import (
	"github.com/theapemachine/symm/stats"
)

func quartiles(values []float64) (lower, upper float64) {
	return stats.Quartiles(values)
}

func percentileSorted(sorted []float64, quantile float64) float64 {
	return stats.PercentileSorted(sorted, quantile)
}

func volumeRatioFence(ratios []float64) float64 {
	if len(ratios) == 0 {
		return 0
	}

	lower, upper := quartiles(ratios)
	spread := upper - lower

	if spread > 0 {
		return upper + spread + spread/2
	}

	return maxFloat(ratios)
}

func volumeRatios(volumes []float64) []float64 {
	baseline := mean(volumes)

	if baseline <= 0 {
		return nil
	}

	ratios := make([]float64, len(volumes))

	for index, volume := range volumes {
		ratios[index] = volume / baseline
	}

	return ratios
}

func maxFloat(values []float64) float64 {
	return stats.Max(values)
}

func crossSectionMedian(values []float64) float64 {
	return stats.CrossSectionMedian(values)
}

func median(values []float64) float64 {
	return stats.Median(values)
}

func sortFloats(values []float64) {
	stats.SortFloats(values)
}
