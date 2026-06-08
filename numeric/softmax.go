package numeric

import (
	"math"
)

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
