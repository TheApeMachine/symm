package sentiment

import (
	"math"

	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/numeric/adaptive"
)

/*
categoryConfidence returns how decisively the assigned sentiment category wins
over its neighbors — not how extreme the breadth odds are.
*/
func categoryConfidence(
	category perspectives.CategoryType,
	breadth, change, surgeThreshold float64,
	leader bool,
) float64 {
	switch category {
	case perspectives.CategoryRiskOnSurge:
		return surgeConfidence(breadth, surgeThreshold)
	case perspectives.CategoryDivergentMove:
		return divergentConfidence(change, leader)
	case perspectives.CategorySystemicSlump:
		return slumpConfidence(breadth, surgeThreshold, leader)
	default:
		return 0
	}
}

func surgeConfidence(breadth, surgeThreshold float64) float64 {
	classifier := adaptive.NewClassifier(
		[]float64{surgeThreshold},
		[]float64{0, 1},
		[]string{"below", "surge"},
	)

	return classifier.Confidence(breadth)
}

func divergentConfidence(change float64, leader bool) float64 {
	if !leader || change == 0 {
		return 0
	}

	return math.Min(1, math.Abs(change))
}

func slumpConfidence(breadth, surgeThreshold float64, leader bool) float64 {
	if leader {
		return 0
	}

	margin := surgeThreshold - breadth

	if margin <= 0 {
		return 0
	}

	scale := math.Max(surgeThreshold, 1-surgeThreshold)

	if scale <= 0 {
		return 0
	}

	return margin / scale
}
