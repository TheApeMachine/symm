package liquidity

import (
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/numeric"
)

/*
liquidityReading ranks quote volume against peer quartiles and returns shift evidence.
*/
func liquidityReading(
	quoteVol float64,
	peers []float64,
) (perspectives.CategoryType, float64, error) {
	sorted := numeric.CopySorted(peers)
	q1 := numeric.PercentileSorted(sorted, 0.25)
	q3 := numeric.PercentileSorted(sorted, 0.75)

	categories, err := perspectives.NewCategories(
		[]float64{q1, q3},
		[]perspectives.CategoryType{
			perspectives.CategoryExtremeScarcity,
			perspectives.CategoryMedianDepth,
			perspectives.CategoryRobustLiquidity,
		},
	)

	if err != nil {
		return perspectives.CategoryTypeNone, 0, err
	}

	category, err := categories.Classify(quoteVol)

	if err != nil {
		return perspectives.CategoryTypeNone, 0, err
	}

	return category, categories.Clarity(quoteVol), nil
}
