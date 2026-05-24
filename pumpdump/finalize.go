package pumpdump

import (
	"github.com/theapemachine/symm/engine"
)

/*
GaugeConfidence normalizes one raw score for dashboard gauges without measurement gates.
*/
func GaugeConfidence(
	trackStore *TrackStore,
	symbol string,
	rawConfidence float64,
) float64 {
	if rawConfidence <= 0 {
		return 0
	}

	track := trackStore.ensure(symbol)
	normalized := track.calibrator.NormalizeConfidence(rawConfidence, track.confidenceHistory)
	track.liveScore = normalized

	return normalized
}

/*
FinalizeReading normalizes raw confidence for emission and gauge history.
*/
func FinalizeReading(
	trackStore *TrackStore,
	symbol string,
	rawConfidence float64,
	reason string,
) (float64, string) {
	if rawConfidence <= 0 {
		return 0, ""
	}

	normalized := GaugeConfidence(trackStore, symbol, rawConfidence)

	if normalized <= 0 {
		return 0, ""
	}

	trackStore.RecordConfidence(symbol, rawConfidence)

	return normalized, reason
}
