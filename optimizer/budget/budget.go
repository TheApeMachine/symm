package budget

import (
	"math"
	"runtime"
)

/*
DeriveMeasurementSampleCap limits JSONL loading for large captures without a fixed
row ceiling. Sample size grows with sqrt(file rows) and workers.
*/
func DeriveMeasurementSampleCap(totalRows int, workers int) int {
	return deriveMeasurementSampleCap(totalRows, workers)
}

func deriveMeasurementSampleCap(totalRows int, workers int) int {
	if totalRows <= 0 {
		return 0
	}

	if workers <= 0 {
		workers = runtime.NumCPU()
	}

	if totalRows <= workers*workers {
		return totalRows
	}

	sample := int(math.Ceil(math.Sqrt(float64(totalRows)) * float64(workers)))

	if sample > totalRows {
		return totalRows
	}

	return sample
}
