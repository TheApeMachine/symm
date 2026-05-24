package basis

import "github.com/theapemachine/symm/engine"

/*
WarmFromOHLC seeds relative-strength history from cross-section bar changes.
*/
func (trackStore *TrackStore) WarmFromOHLC(candles map[string][]engine.OHLCCandle) {
	length := minCompletedLength(candles)

	if length <= 0 {
		return
	}

	for index := 0; index < length; index++ {
		changes := barChangesAt(candles, index)
		median := crossSectionMedianChange(changes)

		for symbol, change := range changes {
			track := trackStore.ensure(symbol)
			track.recordRelativeStrength(change - median)
		}
	}
}

func minCompletedLength(candles map[string][]engine.OHLCCandle) int {
	length := 0

	for _, bars := range candles {
		completed := engine.CompletedCandles(bars)
		size := len(completed)

		if size == 0 {
			continue
		}

		if length == 0 || size < length {
			length = size
		}
	}

	return length
}

func barChangesAt(candles map[string][]engine.OHLCCandle, index int) map[string]float64 {
	changes := make(map[string]float64, len(candles))

	for symbol, bars := range candles {
		completed := engine.CompletedCandles(bars)

		if index >= len(completed) {
			continue
		}

		bar := completed[index]

		if bar.Open <= 0 {
			continue
		}

		changes[symbol] = (bar.Close - bar.Open) / bar.Open * 100
	}

	return changes
}
