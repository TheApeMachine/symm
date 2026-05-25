package sentiment

import "github.com/theapemachine/symm/engine"

/*
WarmFromOHLC seeds sentiment feature history from volume and bar-change z-scores.
*/
func (trackStore *TrackStore) WarmFromOHLC(candles map[string][]engine.OHLCCandle) {
	length := engine.MinCompletedLength(candles)

	if length <= 0 {
		return
	}

	for index := 0; index < length; index++ {
		pressures, changes := sectionFeaturesAt(candles, index)

		for symbol := range candles {
			pressure, hasPressure := pressures[symbol]
			change, hasChange := changes[symbol]

			if !hasPressure && !hasChange {
				continue
			}

			raw := sentimentRaw(
				crossSectionZScore(pressure, pressuresSlice(pressures)),
				crossSectionZScore(change, changesSlice(changes)),
			)

			if raw <= 0 {
				continue
			}

			track := trackStore.ensure(symbol)
			track.recordSentiment(raw)
		}
	}
}

func sectionFeaturesAt(
	candles map[string][]engine.OHLCCandle,
	index int,
) (map[string]float64, map[string]float64) {
	pressures := make(map[string]float64, len(candles))
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

		change := (bar.Close - bar.Open) / bar.Open * 100
		pressure := bar.Volume

		if bar.Close < bar.Open {
			pressure = -pressure
		}

		pressures[symbol] = pressure
		changes[symbol] = change
	}

	return pressures, changes
}

func pressuresSlice(values map[string]float64) []float64 {
	slice := make([]float64, 0, len(values))

	for _, value := range values {
		slice = append(slice, value)
	}

	return slice
}

func changesSlice(values map[string]float64) []float64 {
	slice := make([]float64, 0, len(values))

	for _, value := range values {
		slice = append(slice, value)
	}

	return slice
}
