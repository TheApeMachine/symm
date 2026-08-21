package temporal

import "github.com/theapemachine/symm/nomagique"

var (
	SymbolSpacingNanos = nomagique.MustIntern("spacing_nanos")
	SymbolSpacingReady = nomagique.MustIntern("spacing_ready")
)

/*
Spacing emits the exact discrete median of the positive event-time gaps in a
Path. It uses retained timestamps only and imposes no external time window.
*/
func Spacing(
	state nomagique.Frame,
	input nomagique.Frame,
) (nomagique.Frame, nomagique.Frame, error) {
	gaps := [MaxPathSamples]int64{}
	gapCount := 0
	count, _ := input.Get(nomagique.SampleCount)

	for index := 1; index < int(count); index++ {
		previous, _, hasPrevious := PathSample(&input, index-1)
		current, _, hasCurrent := PathSample(&input, index)

		if !hasPrevious || !hasCurrent || current <= previous {
			continue
		}

		gaps[gapCount] = current - previous
		gapCount++
	}

	output := input

	if gapCount == 0 {
		output.Put(SymbolSpacingReady, 0)

		return state, output, nil
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

	output.Put(SymbolSpacingNanos, float64(spacing))
	output.Put(SymbolSpacingReady, 1)

	return state, output, nil
}
