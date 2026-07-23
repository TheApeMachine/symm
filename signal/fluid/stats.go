package fluid

import (
	"sort"

	"gonum.org/v1/gonum/stat"
)

func sampleQuantile(percentile float64, values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	sorted := append([]float64(nil), values...)

	sort.Float64s(sorted)

	return stat.Quantile(percentile, stat.LinInterp, sorted, nil)
}
