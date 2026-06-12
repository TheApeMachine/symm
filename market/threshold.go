package market

import "github.com/theapemachine/symm/logic"

func (story *Story) thresholdContext() logic.ThresholdContext {
	regimeVolatility := 0.0

	if story.regime != nil {
		mean, ready := story.regime.MarketMean()

		if ready {
			regimeVolatility = mean.Volatility
		}
	}

	return logic.NewThresholdContext(regimeVolatility)
}
