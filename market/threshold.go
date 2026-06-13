package market

import "github.com/theapemachine/symm/logic"

func (story *Story) thresholdContext() logic.ThresholdContext {
	return logic.NewThresholdContext(story.tree.ThresholdConfig(), story.regimeVolatility())
}

func (story *Story) regimeVolatility() float64 {
	if story == nil || story.regime == nil {
		return 0
	}

	mean, ready := story.regime.MarketMean()

	if !ready {
		return 0
	}

	return mean.Volatility
}
