package numeric

import (
	"fmt"
	"math"
)

/*
SoftmaxScores maps raw scores to a normalized probability vector.
*/
func SoftmaxScores(scores []float64) ([]float64, error) {
	if len(scores) == 0 {
		return nil, fmt.Errorf("numeric: softmax requires at least one score")
	}

	for index, score := range scores {
		if math.IsNaN(score) || math.IsInf(score, 0) {
			return nil, fmt.Errorf("numeric: softmax score[%d] is non-finite", index)
		}
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
		return nil, fmt.Errorf("numeric: softmax normalization sum is non-positive")
	}

	for index := range probabilities {
		probabilities[index] /= expSum
	}

	return probabilities, nil
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

/*
CategoryConfidence returns the softmax probability for the selected category.
categoryIndex is 1-based; when zero, the winning category probability is used.
*/
func CategoryConfidence(probabilities []float64, categoryIndex int) (float64, error) {
	if len(probabilities) == 0 {
		return 0, fmt.Errorf("numeric: category confidence requires probabilities")
	}

	probabilityIndex := ArgmaxIndex(probabilities)

	if categoryIndex > 0 && categoryIndex-1 < len(probabilities) {
		probabilityIndex = categoryIndex - 1
	}

	return probabilities[probabilityIndex], nil
}
