package engine

/*
LiveReading is the strongest symbol-level score currently held in a signal track.
*/
type LiveReading struct {
	Symbol string
	Score  float64
}

/*
LiveScoreReader exposes live dashboard scores from signal track stores.
Scores update every scan, not only when a measurement is enqueued for the trader.
*/
type LiveScoreReader interface {
	LiveScore() float64
	PeakReading() LiveReading
}

/*
MeanConfidenceReader exposes the peak normalized confidence across the latest scan set.
*/
type MeanConfidenceReader interface {
	MeanConfidence() float64
}

/*
PeakLiveReading returns the strongest symbol-level live score from a score map.
*/
func PeakLiveReading(liveScores map[string]float64) LiveReading {
	return PeakLiveFromMap(liveScores, func(score float64) float64 {
		return score
	})
}

/*
PeakLiveFromMap scans keyed items and returns the symbol with the highest score.
*/
func PeakLiveFromMap[T any](items map[string]T, score func(T) float64) LiveReading {
	bestSymbol := ""
	bestScore := 0.0

	for symbol, item := range items {
		value := score(item)

		if value <= bestScore {
			continue
		}

		bestScore = value
		bestSymbol = symbol
	}

	return LiveReading{
		Symbol: bestSymbol,
		Score:  bestScore,
	}
}
