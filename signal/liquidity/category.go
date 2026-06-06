package liquidity

import (
	"github.com/theapemachine/symm/market/perspectives/types"
	"github.com/theapemachine/symm/numeric"
)

/*
liquidityReading ranks quote volume against peer quartiles. clarity is band
margin; standout is returned separately by the caller from peer deviation.
*/
func liquidityReading(
	quoteVol float64,
	peers []float64,
) (types.CategoryType, float64, float64, error) {
	sorted := numeric.CopySorted(peers)
	q1 := numeric.PercentileSorted(sorted, 0.25)
	q3 := numeric.PercentileSorted(sorted, 0.75)

	categories, err := types.NewCategories(
		[]float64{q1, q3},
		[]types.CategoryType{
			types.CategoryExtremeScarcity,
			types.CategoryMedianDepth,
			types.CategoryRobustLiquidity,
		},
	)

	if err != nil {
		return types.CategoryTypeNone, 0, 0, err
	}

	category, err := categories.Classify(quoteVol)

	if err != nil {
		return types.CategoryTypeNone, 0, 0, err
	}

	return category, categories.Clarity(quoteVol), categories.Standout(quoteVol), nil
}
