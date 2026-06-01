package exhaust

import "github.com/theapemachine/symm/market/perspectives"

/*
exhaustMeasurement maps rolling exit features onto the exhaustion perspective.
Confidence accumulates when the category shifts.
*/
func exhaustMeasurement(
	history symbolHistory,
	tracked *perspectives.Category,
) (perspectives.Measurement, bool) {
	urgency, category, evidence := exitScoreLong(history)

	if urgency <= 0 {
		urgency, category, evidence = exitScoreShort(history)
	}

	if urgency <= 0 || category == perspectives.CategoryTypeNone {
		return perspectives.Measurement{}, false
	}

	if urgency > 0.999 {
		urgency = 0.999
	}

	confidence, err := tracked.Observe(category, evidence)

	if err != nil {
		return perspectives.Measurement{}, false
	}

	return perspectives.Measurement{
		Source:     perspectives.SourceExhaustion,
		Category:   category,
		Strength:   urgency / (1 - urgency),
		Confidence: confidence,
	}, true
}
