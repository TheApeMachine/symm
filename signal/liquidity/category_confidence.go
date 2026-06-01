package liquidity

import (
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/numeric"
	"github.com/theapemachine/symm/numeric/adaptive"
)

var liquidityBandCodes = []float64{0, 1, 2}

var liquidityBandLabels = []string{
	"extreme_scarcity",
	"median_depth",
	"robust_liquidity",
}

/*
classifyLiquidity maps quote volume against peer quartiles and returns the
scarcity category plus clarity — margin to the nearest quartile boundary.
*/
func classifyLiquidity(
	quoteVol float64,
	peers []float64,
) (perspectives.CategoryType, float64) {
	sorted := numeric.CopySorted(peers)
	q1 := numeric.PercentileSorted(sorted, 0.25)
	q3 := numeric.PercentileSorted(sorted, 0.75)
	classifier := adaptive.NewClassifier(
		[]float64{q1, q3},
		liquidityBandCodes,
		liquidityBandLabels,
	)
	confidence := classifier.Confidence(quoteVol)

	switch {
	case quoteVol >= q3:
		return perspectives.CategoryRobustLiquidity, confidence
	case quoteVol >= q1:
		return perspectives.CategoryMedianDepth, confidence
	default:
		return perspectives.CategoryExtremeScarcity, confidence
	}
}
