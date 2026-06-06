package sentiment

import (
	"math"

	"github.com/theapemachine/symm/market/perspectives/types"
	"github.com/theapemachine/symm/numeric/adaptive"
)

// uniformSentimentConfidence is the 1/N floor across the three sentiment categories
// (risk-on surge, divergent move, systemic slump): a selection with no measurable
// margin is no better than a uniform guess, and is never zero confidence.
const uniformSentimentConfidence = 1.0 / 3.0

/*
sentimentReading classifies cross-section sentiment and returns the category plus
its confidence — how decisively the breadth lands in the selected band. The
phenomenon strength (the standout fed to SNR) is computed separately by the caller
from breadth. A selection with no measurable margin floors at 1/N, never 0.
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

		return types.CategoryDivergentMove, types.UnitMagnitudeMargin(margin)
	}

	margin := surgeThreshold - breadth

	if leader || margin <= 0 {
		return types.CategorySystemicSlump, uniformSentimentConfidence
	}

	scale := math.Max(surgeThreshold, 1-surgeThreshold)

	if scale <= 0 {
		return types.CategorySystemicSlump, uniformSentimentConfidence
	}

	return types.CategorySystemicSlump, types.UnitCompetitionMargin(margin, scale)
}
