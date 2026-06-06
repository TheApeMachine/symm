package exhaust

import (
	"github.com/theapemachine/symm/market/perspectives/types"
)

/*
exhaustMeasurement maps rolling exit features onto the exhaustion perspective.
Confidence is how decisively the dominant exit mode beat its runner-up; standout
(urgency) is the exhaustion intensity SNR scores against the symbol's history.
*/
func exhaustMeasurement(
	history symbolHistory,
	tracked *types.Category,
) (types.Measurement, float64, error) {
	urgency, category, evidence := exitScoreLong(history)

	if urgency <= 0 {
		urgency, category, evidence = exitScoreShort(history)
	}

	if urgency <= 0 || category == types.CategoryTypeNone {
		return types.Measurement{}, 0, nil
	}

	// evidence is how decisively the dominant exit mode beat its runner-up (the
	// category-selection certainty); standout is the aggregate exhaustion intensity
	// itself — urgency — which SNR scores against this symbol's own history. A
	// clear-cut but mild exhaustion has high evidence and low standout.
	standout := urgency

	if err := tracked.Observe(category, evidence); err != nil {
		return types.Measurement{}, 0, err
	}

	return types.Measurement{
		Source:     types.SourceExhaustion,
		Category:   category,
		Strength:   urgency,
		Confidence: evidence,
	}, standout, nil
}
