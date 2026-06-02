package sentiment

import (
	"math"

	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/numeric/adaptive"
)

/*
sentimentReading classifies cross-section sentiment and returns shift evidence.
*/
func sentimentReading(
	breadth, change, surgeThreshold float64,
	leader bool,
) (perspectives.CategoryType, float64) {
	if breadth >= surgeThreshold {
		classifier := adaptive.NewClassifier(
			[]float64{surgeThreshold},
			[]float64{0, 1},
			[]string{"below", "surge"},
		)

		return perspectives.CategoryRiskOnSurge, classifier.Confidence(breadth)
	}

	if leader && change != 0 {
		margin := math.Abs(change)

		return perspectives.CategoryDivergentMove, margin / (margin + 1)
	}

	margin := surgeThreshold - breadth

	if leader || margin <= 0 {
		return perspectives.CategorySystemicSlump, 0
	}

	scale := math.Max(surgeThreshold, 1-surgeThreshold)

	if scale <= 0 {
		return perspectives.CategorySystemicSlump, 0
	}

	return perspectives.CategorySystemicSlump, margin / scale
}
