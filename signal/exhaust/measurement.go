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

	standout := evidence
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
