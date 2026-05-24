package trader

import "github.com/theapemachine/symm/engine"

/*
WarmReturnModelFromOHLC seeds trader-owned forward-return buckets from OHLC warm
data so live measurements can become candidates before live prediction settlement.
*/
func (crypto *Crypto) WarmReturnModelFromOHLC(
	candles map[string][]engine.OHLCCandle,
) int {
	if crypto == nil || crypto.returnModel == nil || len(candles) == 0 {
		return 0
	}

	total := 0

	for _, signal := range crypto.signals {
		if signal == nil {
			continue
		}

		total += crypto.returnModel.WarmFromOHLC(
			signal.Source(),
			warmRegimes(signal.Source()),
			candles,
		)
	}

	return total
}

/*
WarmFromOHLC records dynamic realized forward returns for a source/regime pair.
*/
func (model *ReturnModel) WarmFromOHLC(
	source string,
	regimes []string,
	candles map[string][]engine.OHLCCandle,
) int {
	if model == nil || source == "" || len(regimes) == 0 || len(candles) == 0 {
		return 0
	}

	total := 0

	for symbol, bars := range candles {
		completed := engine.CompletedCandles(bars)

		for index := 0; index < len(completed)-1; index++ {
			bar := completed[index]
			next := completed[index+1]
			predictedReturn := warmPredictedReturn(bar)

			if predictedReturn <= 0 {
				continue
			}

			for _, regime := range regimes {
				actualReturn := warmActualReturn(regime, bar.Close, next.Close)

				if actualReturn <= 0 {
					continue
				}

				model.Apply(engine.PredictionFeedback{
					Source:          source,
					Regime:          regime,
					Symbol:          symbol,
					PredictedReturn: predictedReturn,
					ActualReturn:    actualReturn,
				})
				total++
			}
		}
	}

	return total
}

func warmRegimes(source string) []string {
	switch source {
	case "hawkes":
		return []string{"momentum", "dump"}
	case "fluid":
		return []string{"flow"}
	case "pumpdump":
		return []string{"pump"}
	case "causal":
		return []string{"causal"}
	case "depthflow":
		return []string{"depth"}
	case "leadlag":
		return []string{"cross"}
	case "basis":
		return []string{"basis"}
	case "sentiment":
		return []string{"sentiment"}
	default:
		return []string{source}
	}
}

func warmPredictedReturn(bar engine.OHLCCandle) float64 {
	if bar.Close <= 0 || bar.High <= bar.Low {
		return 0
	}

	return (bar.High - bar.Low) / bar.Close
}

func warmActualReturn(regime string, entry, exit float64) float64 {
	if entry <= 0 || exit <= 0 {
		return 0
	}

	actual := (exit - entry) / entry

	if regime == "dump" {
		return -actual
	}

	return actual
}
