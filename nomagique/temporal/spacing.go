package temporal

import (
	"github.com/theapemachine/symm/nomagique/types"
)

var (
	SymbolSpacingNanos = types.MustIntern("spacing_nanos")
	SymbolSpacingReady = types.MustIntern("spacing_ready")
)

/*
Spacing emits the exact discrete median of the positive event-time gaps in one
series. It uses that series' retained timestamps only and imposes no external
time window.
*/
func (series Series) Spacing(frame types.Frame) types.Frame {
	gaps := [MaxPathSamples]int64{}
	gapCount := 0
	count := series.Count(frame)

	for index := 1; index < count; index++ {
		previous, _, hasPrevious := series.Sample(&frame, index-1)
		current, _, hasCurrent := series.Sample(&frame, index)

		if !hasPrevious || !hasCurrent || current <= previous {
			continue
		}

		gaps[gapCount] = current - previous
		gapCount++
	}

	if gapCount == 0 {
		frame.Put(SymbolSpacingReady, 0)

		return frame
	}

	for index := 1; index < gapCount; index++ {
		value := gaps[index]
		position := index

		for position > 0 && gaps[position-1] > value {
			gaps[position] = gaps[position-1]
			position--
		}

		gaps[position] = value
	}

	middle := gapCount / 2
	spacing := gaps[middle]

	if gapCount%2 == 0 {
		spacing = (gaps[middle-1] + gaps[middle]) / 2
	}

	frame.Put(SymbolSpacingNanos, float64(spacing))
	frame.Put(SymbolSpacingReady, 1)

	return frame
}
