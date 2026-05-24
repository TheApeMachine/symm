package pumpdump

import (
	"time"

	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/stats"
)

/*
FinalizeMeasurement normalizes raw confidence and derives bucket runway.
*/
func FinalizeMeasurement(
	trackStore *TrackStore,
	symbol string,
	rawConfidence float64,
	now time.Time,
	reason string,
) (float64, float64, time.Duration, string) {
	if rawConfidence <= 0 {
		return 0, 0, 0, ""
	}

	track, ok := trackStore.bySymbol[symbol]

	if !ok {
		return 0, 0, 0, ""
	}

	confidenceHistory := append([]float64(nil), track.confidenceHistory...)
	priceMoves := append([]float64(nil), track.priceMoves...)
	bucketStart := track.bucketStart

	normalized := engine.NormalizeConfidence(rawConfidence, confidenceHistory)
	trackStore.SetLiveScore(symbol, normalized)

	runway := bucketRunway(bucketStart, now)

	if runway <= 0 {
		return 0, 0, 0, ""
	}

	trackStore.RecordConfidence(symbol, rawConfidence)
	expectedReturn := expectedReturnFromMoves(priceMoves, runway)

	return normalized, expectedReturn, runway, reason
}

func bucketRunway(bucketStart time.Time, now time.Time) time.Duration {
	if bucketStart.IsZero() {
		return 0
	}

	remaining := bucketWindow - now.Sub(bucketStart)

	if remaining <= 0 {
		return 0
	}

	return remaining
}

func expectedReturnFromMoves(priceMoves []float64, runway time.Duration) float64 {
	if len(priceMoves) < minPriceHistory || runway <= 0 {
		return 0
	}

	quietLine := stats.Median(priceMoves)

	return quietLine * (runway.Seconds() / bucketWindow.Seconds())
}
