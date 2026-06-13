package signal

import (
	"fmt"
	"math"

	"github.com/theapemachine/nomagique/probability"
)

/*
CategoryConfidence returns the stronger of softmax share and linear share so
decisive single-category evidence is not diluted by collinear competitors.
*/
func CategoryConfidence(scores []float64, categoryIndex int) (float64, error) {
	if categoryIndex < 0 || categoryIndex >= len(scores) {
		return 0, fmt.Errorf("signal: category index %d out of range", categoryIndex)
	}

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

	if positiveSum <= 0 {
		return 0, fmt.Errorf("signal: category scores must be positive")
	}

	winner := scores[categoryIndex]

	if winner <= 0 {
		return 0, fmt.Errorf("signal: winning category score must be positive")
	}

	linearShare := winner / positiveSum

	return math.Max(share, linearShare), nil
}
