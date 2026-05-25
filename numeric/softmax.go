package numeric

import "math"

func logEvidenceValue(logEvidence map[string]float64, label string) float64 {
	val, ok := logEvidence[label]

	if !ok {
		return math.Inf(-1)
	}

	return val
}

/*
SoftmaxPercentages maps log-domain scores to percentages summing to 100 over labels
(max-subtraction for numerical stability).
*/
func SoftmaxPercentages(logEvidence map[string]float64, labels []string) map[string]float64 {
	expScores := make(map[string]float64, len(labels))
	maxLog := math.Inf(-1)

	for _, label := range labels {
		val := logEvidenceValue(logEvidence, label)

		if val > maxLog {
			maxLog = val
		}
	}

	out := make(map[string]float64, len(labels))

	if math.IsInf(maxLog, -1) {
		for _, label := range labels {
			out[label] = 0
		}

		return out
	}

	sumExp := 0.0

	for _, label := range labels {
		val := logEvidenceValue(logEvidence, label)
		expProbability := math.Exp(val - maxLog)
		expScores[label] = expProbability
		sumExp += expProbability
	}

	if math.IsNaN(sumExp) || sumExp == 0 {
		for _, label := range labels {
			out[label] = 0
		}

		return out
	}

	for _, label := range labels {
		out[label] = expScores[label] / sumExp * 100
	}

	return out
}
