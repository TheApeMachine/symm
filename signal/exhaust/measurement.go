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
SNR is the urgency expressed as odds — a decisively exhausting book clears the
noise floor, a marginal one does not.
*/
func exhaustMeasurement(history symbolHistory) (perspectives.Measurement, bool) {
	urgency, reason := exitScoreLong(history)

	if urgency <= 0 {
		urgency, reason = exitScoreShort(history)
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
		SNR:      urgency / (1 - urgency),
	}, true
}
