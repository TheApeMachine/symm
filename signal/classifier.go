package signal

import (
	"github.com/theapemachine/nomagique/probability"
)

/*
ShareConfidence returns conservative category evidence share using 1-based indexing.
*/
func ShareConfidence(scores []float64, categoryIndex int) (float64, error) {
	return probability.CategoryShareConfidence(scores, categoryIndex)
}

/*
WinsorizeScore caps a score at an adaptive rolling percentile when provided.
*/
func WinsorizeScore(score float64, percentileCap float64) float64 {
	if percentileCap <= 0 || score <= percentileCap {
		return score
	}

	return percentileCap
}

/*
TransitionNovelty computes KL transition surprise from normalized probabilities.
*/
func TransitionNovelty(
	transition *probability.TransitionMatrix,
	probabilities []float64,
	leadingMass float64,
) (float64, error) {
	surpriseVector := transition.PadObserved(probabilities, leadingMass)

	return transition.Surprise(surpriseVector)
}
