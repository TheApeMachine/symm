package exhaust

import (
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
)

const exhaustSource = "exhaust"

/*
exhaustMeasurement maps rolling exit features onto the exit-thesis perspective.
*/
func exhaustMeasurement(symbol string, history symbolHistory) (engine.Measurement, bool) {
	urgency, reason := exitScoreLong(history)

	if urgency <= 0 {
		urgency, reason = exitScoreShort(history)
	}

	if urgency <= 0 || reason == "" {
		return engine.Measurement{}, false
	}

	confidence := engine.ConfidenceFromScore(urgency)

	if confidence <= 0 {
		return engine.Measurement{}, false
	}

	return engine.Measurement{
		Type:       engine.Momentum,
		Source:     exhaustSource,
		Regime:     "exhaust",
		Reason:     reason,
		Category:   exhaustCategory(reason),
		Pairs:      []asset.Pair{{Wsname: symbol}},
		Confidence: confidence,
		Last:       history.lastPrice,
	}, true
}
