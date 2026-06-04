package exhaust

import "github.com/theapemachine/symm/market/perspectives"

/*
exhaustMeasurement maps rolling exit features onto the exhaustion perspective.
Confidence accumulates when the category shifts.
*/
func exhaustMeasurement(
	history symbolHistory,
	tracked *perspectives.Category,
) (perspectives.Measurement, float64, error) {
	urgency, category, evidence := exitScoreLong(history)

	if urgency <= 0 {
		urgency, category, evidence = exitScoreShort(history)
	}

	if urgency <= 0 || category == perspectives.CategoryTypeNone {
		return perspectives.Measurement{}, 0, nil
	}

	// evidence is how decisively the dominant exit mode beat its runner-up (the
	// category-selection certainty); standout is the aggregate exhaustion intensity
	// itself — urgency — which SNR scores against this symbol's own history. A
	// clear-cut but mild exhaustion has high evidence and low standout.
	standout := urgency
	confidence, err := tracked.Observe(category, evidence, standout)

	if err != nil {
		return perspectives.Measurement{}, 0, err
	}

	return perspectives.Measurement{
		Source:     perspectives.SourceExhaustion,
		Category:   category,
		Strength:   urgency / (1 - urgency),
		Confidence: confidence,
	}, standout, nil
}
