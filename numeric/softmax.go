package numeric

import (
	"math"
)

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

/*
SoftmaxScores maps raw scores to a normalized probability vector.
*/
func SoftmaxScores(scores []float64) []float64 {
	if len(scores) == 0 {
		return nil
	}

	probabilities := make([]float64, len(scores))
	maxScore := scores[0]

	for _, score := range scores[1:] {
		if score > maxScore {
			maxScore = score
		}
	}

	expSum := 0.0

	for index, score := range scores {
		weighted := math.Exp(score - maxScore)
		probabilities[index] = weighted
		expSum += weighted
	}

	if expSum <= 0 {
		return probabilities
	}

	for index := range probabilities {
		probabilities[index] /= expSum
	}

	return probabilities
}

/*
ArgmaxIndex returns the index of the largest value.
*/
func ArgmaxIndex(values []float64) int {
	if len(values) == 0 {
		return 0
	}

	bestIndex := 0
	bestValue := values[0]

	for index, value := range values[1:] {
		if value > bestValue {
			bestValue = value
			bestIndex = index + 1
		}
	}

	return bestIndex
}
