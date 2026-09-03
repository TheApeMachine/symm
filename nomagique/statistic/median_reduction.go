package statistic

import (
	"math"

	"github.com/theapemachine/symm/nomagique/types"
)

// MedianReduction calculates the exact median of a slice of Numbers without mutating caller memory.
var MedianReduction types.Reduction = func(values []types.Number) types.Number {
	count := len(values)

	if count == 0 {
		return 0
	}

	if count == 1 {
		return values[0]
	}

	// Copy into a scratch slice to avoid mutating input slice order
	scratch := make([]types.Number, count)
	copy(scratch, values)

	middle := count / 2
	upper := quickSelect(scratch, middle)

	if count%2 != 0 {
		return upper
	}

	lower := quickSelect(scratch[:middle], middle-1)

	return lower + (upper-lower)/2
}

func quickSelect(values []types.Number, target int) types.Number {
	left := 0
	right := len(values) - 1

	for left < right {
		pivot := values[(left+right)/2]
		low := left
		high := right

		for low <= high {
			for low <= right && values[low] < pivot {
				low++
			}

			for high >= left && values[high] > pivot {
				high--
			}

			if low <= high {
				values[low], values[high] = values[high], values[low]
				low++
				high--
			}
		}

		if target <= high {
			right = high
			continue
		}

		if target >= low {
			left = low
			continue
		}

		return values[target]
	}

	return values[target]
}

/*
MedianAbsoluteOf computes the median of the absolute values of a slice.
*/
func MedianAbsoluteOf(values []float64) (float64, bool) {
	if len(values) == 0 {
		return 0, false
	}

	absValues := make([]types.Number, 0, len(values))

	for _, value := range values {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return 0, false
		}

		absValues = append(absValues, types.Number(math.Abs(value)))
	}

	count := len(absValues)
	middle := count / 2
	upper := quickSelect(absValues, middle)

	if count%2 != 0 {
		return float64(upper), true
	}

	lower := quickSelect(absValues[:middle], middle-1)

	return float64(lower + (upper-lower)/2), true
}
