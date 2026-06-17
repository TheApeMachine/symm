package logic

import "math"

/*
MarketRegimeFrame aggregates live measurements into cross-section regime axes.
*/
func MarketRegimeFrame(measurements []Measurement) map[string]any {
	frame := map[string]any{
		"type":   "regime",
		"symbol": "market",
	}

	if len(measurements) == 0 {
		frame["volatility"] = 0.0
		frame["trend"] = 0.0
		frame["bullish"] = 0.0
		frame["bearish"] = 0.0
		frame["choppiness"] = 0.0

		return frame
	}

	var (
		volatilitySum float64
		trendSum      float64
		bullishSum    float64
		bearishSum    float64
		choppinessSum float64
		volatilityN   float64
		trendN        float64
		bullishN      float64
		bearishN      float64
		choppinessN   float64
	)

	for _, measurement := range measurements {
		if !measurement.Publishable() {
			continue
		}

		if measurement.Price > 0 && ScalarFinite(measurement.Spread) {
			spreadRatio := measurement.Spread / measurement.Price

			if spreadRatio > 0 {
				volatilitySum += clampUnit(spreadRatio * 100.0)
				volatilityN++
			}
		}

		if ScalarFinite(measurement.Strength) && measurement.Strength > 0 {
			trendSum += clampUnit(measurement.Strength * measurement.Confidence)
			trendN++
		}

		switch measurement.Category {
		case CategoryVerticalIgnition, CategoryRiskOnSurge, CategoryInertial, CategoryAggressiveDrive:
			bullishSum += clampUnit(measurement.Confidence)
			bullishN++
		case CategoryActiveReversal, CategorySystemicSlump, CategoryLiquidityVacuum:
			bearishSum += clampUnit(measurement.Confidence)
			bearishN++
		}

		if ScalarFinite(measurement.Surprise) && measurement.Surprise > 0 {
			choppinessSum += clampUnit(measurement.Surprise)
			choppinessN++
		}
	}

	frame["volatility"] = meanAxis(volatilitySum, volatilityN)
	frame["trend"] = meanAxis(trendSum, trendN)
	frame["bullish"] = meanAxis(bullishSum, bullishN)
	frame["bearish"] = meanAxis(bearishSum, bearishN)
	frame["choppiness"] = meanAxis(choppinessSum, choppinessN)

	return frame
}

func meanAxis(sum float64, count float64) float64 {
	if count <= 0 {
		return 0
	}

	return clampUnit(sum / count)
}

func clampUnit(value float64) float64 {
	if !ScalarFinite(value) {
		return 0
	}

	return math.Min(1, math.Max(0, value))
}
