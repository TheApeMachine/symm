package signal

import (
	"fmt"
	"math"

	"github.com/theapemachine/nomagique/probability"
)

/*
ClassifierProbabilities maps heterogeneous raw scores to a scale-invariant distribution.
*/
func ClassifierProbabilities(scores []float64) ([]float64, error) {
	return probability.SoftmaxScoresNormalized(scores)
}

/*
CategoryConfidence returns the stronger of pseudocount share and linear share for the
selected 1-based category index. Index 0 is invalid for real categories.
*/
func CategoryConfidence(scores []float64, categoryIndex int) (float64, error) {
	if len(scores) == 0 {
		return 0, fmt.Errorf("signal: scores required")
	}

	if categoryIndex < 1 || categoryIndex > len(scores) {
		return 0, fmt.Errorf(
			"signal: real category index %d out of range [1,%d]",
			categoryIndex,
			len(scores),
		)
	}

	selected := scores[categoryIndex-1]

	share, err := probability.CategoryShareConfidence(scores, categoryIndex)

	if err != nil {
		return 0, err
	}

	positiveSum := 0.0

	for _, score := range scores {
		if score > 0 {
			positiveSum += score
		}
	}

	if selected <= 0 || positiveSum <= 0 {
		return 0, nil
	}

	linearShare := selected / positiveSum

	return math.Max(share, linearShare), nil
}

/*
BoundedFeatureScore applies a log1p transform for unbounded physical features.
*/
func BoundedFeatureScore(raw float64, baseline float64) float64 {
	if raw <= 0 {
		return 0
	}

	excess := raw

	if baseline > 0 {
		excess = raw - baseline
	}

	if excess <= 0 {
		return 0
	}

	return math.Log1p(excess)
}
