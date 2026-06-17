package toxicity

import (
	"github.com/theapemachine/nomagique/statistic"
)

func medianLevelQty(levels map[l2Key]*l2Level) float64 {
	if len(levels) == 0 {
		return 0
	}

	quantities := make([]float64, 0, len(levels))

	for _, level := range levels {
		if level == nil || level.qty <= 0 {
			continue
		}

		quantities = append(quantities, level.qty)
	}

	if len(quantities) == 0 {
		return 0
	}

	return statistic.MedianOf(quantities)
}
