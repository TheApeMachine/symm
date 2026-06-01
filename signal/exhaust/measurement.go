package exhaust

import "github.com/theapemachine/symm/market/perspectives"

const (
	reasonBookThinning  = "book_thinning"
	reasonSpreadWiden   = "spread_widen"
	reasonPressureFade  = "pressure_fade"
	reasonImbalanceFlip = "imbalance_flip"
)

/*
exhaustMeasurement maps rolling exit features onto the exhaustion perspective.
Confidence is category clarity — how decisively one exit mode beat the runner-up;
SNR is how surprising that clarity is versus the symbol's own recent baseline.
*/
func exhaustMeasurement(history symbolHistory) (perspectives.Measurement, bool) {
	urgency, reason, confidence := exitScoreLong(history)

	if urgency <= 0 {
		urgency, reason, confidence = exitScoreShort(history)
	}

	if urgency <= 0 || reason == "" {
		return perspectives.Measurement{}, false
	}

	if urgency > 0.999 {
		urgency = 0.999
	}

	return perspectives.Measurement{
		Source:   perspectives.SourceExhaustion,
		Category: exhaustCategory(reason),
		Strength:   urgency / (1 - urgency),
		Confidence: confidence,
	}, true
}
