package toxicity

import (
	"math"
	"sort"

	"github.com/theapemachine/nomagique/statistic"
	"gonum.org/v1/gonum/stat"
)

func sampleMedian(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	return statistic.MedianOf(values)
}

func sampleQuantile(percentile float64, values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)

	return stat.Quantile(percentile, stat.LinInterp, sorted, nil)
}

func sampleMedianAbsolute(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	absoluteValues := make([]float64, len(values))

	for index, value := range values {
		absoluteValues[index] = math.Abs(value)
	}

	return statistic.MedianOf(absoluteValues)
}
