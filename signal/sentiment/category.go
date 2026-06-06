package sentiment

import (
	"math"

	"github.com/theapemachine/symm/market/perspectives/types"
	"github.com/theapemachine/symm/numeric/adaptive"
)

/*
sentimentReading classifies cross-section sentiment and returns the category plus
its clarity — how decisively the breadth lands in the selected band. The phenomenon
strength (the standout fed to SNR) is computed separately by the caller from breadth.
*/
func sentimentReading(
	breadth, change, surgeThreshold float64,
	leader bool,
) (types.CategoryType, float64) {
	if breadth >= surgeThreshold {
		classifier := adaptive.NewClassifier(
			[]float64{surgeThreshold},
			[]float64{0, 1},
			[]string{"below", "surge"},
		)

		return types.CategoryRiskOnSurge, classifier.Confidence(breadth)
	}

	if leader && change != 0 {
		margin := math.Abs(change)

		return types.CategoryDivergentMove, margin / (margin + 1)
	}

	margin := surgeThreshold - breadth

	if leader || margin <= 0 {
		return types.CategorySystemicSlump, 0
	}

	scale := math.Max(surgeThreshold, 1-surgeThreshold)

	if scale <= 0 {
		return types.CategorySystemicSlump, 0
	}

	return types.CategorySystemicSlump, margin / scale
}
