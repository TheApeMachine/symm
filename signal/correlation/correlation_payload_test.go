package correlation

import (
	"time"
)

func seedHerdScenario(signal *Signal, eventAt time.Time) {
	symbols := []string{"BTC/EUR", "ETH/EUR", "SOL/EUR", "ADA/EUR"}
	prices := map[string]float64{"BTC/EUR": 100, "ETH/EUR": 50, "SOL/EUR": 25, "ADA/EUR": 10}

	for index, shock := range []float64{1.05, 1.06, 1.05, 1.07, 1.06, 1.05, 1.06, 1.07} {
		for _, symbol := range symbols {
			prices[symbol] *= shock
			insertTradeRow(signal, symbol, prices[symbol], 1, eventAt.Add(time.Duration(index)*time.Second))
		}
	}
}

func seedAlphaScenario(signal *Signal, eventAt time.Time) {
	herdPrices := map[string]float64{"BTC/EUR": 100, "ETH/EUR": 50, "SOL/EUR": 25}
	shocks := []float64{1.04, 1.05, 1.04, 1.06, 1.05, 1.04, 1.05, 1.06}
	altPrices := []float64{12, 9, 14, 8, 16, 7, 18, 6}

	for index, shock := range shocks {
		herdPrices["BTC/EUR"] *= shock
		herdPrices["ETH/EUR"] *= shock
		herdPrices["SOL/EUR"] *= shock
		at := eventAt.Add(time.Duration(index) * time.Second)
		insertTradeRow(signal, "BTC/EUR", herdPrices["BTC/EUR"], 1, at)
		insertTradeRow(signal, "ETH/EUR", herdPrices["ETH/EUR"], 1, at)
		insertTradeRow(signal, "SOL/EUR", herdPrices["SOL/EUR"], 1, at)
		insertTradeRow(signal, "ALT/EUR", altPrices[index], 1, at)
	}
}

func seedNoiseScenario(signal *Signal, eventAt time.Time) {
	for index := range 8 {
		at := eventAt.Add(time.Duration(index) * time.Second)
		insertTradeRow(signal, "FLAT/EUR", 100+float64(index)*0.00001, 1, at)
		insertTradeRow(signal, "ETH/EUR", 50+float64(index)*0.05, 1, at)
		insertTradeRow(signal, "SOL/EUR", 25+float64(index)*0.05, 1, at)
		insertTradeRow(signal, "ADA/EUR", 10+float64(index)*0.05, 1, at)
	}
}

func seedStressScenario(signal *Signal, eventAt time.Time) {
	riserShocks := []float64{1.01, 1.02, 1.03, 1.04, 1.05, 1.06, 1.07, 1.08}
	stressShocks := []float64{0.99, 0.98, 0.97, 0.96, 0.95, 0.94, 0.93, 0.92}
	prices := map[string]float64{"BTC/EUR": 100, "ETH/EUR": 50, "SOL/EUR": 25, "ADA/EUR": 10}
	stress := 100.0

	for index := range 8 {
		for symbol, price := range prices {
			price *= riserShocks[index]
			prices[symbol] = price
			insertTradeRow(signal, symbol, price, 1, eventAt.Add(time.Duration(index)*time.Second))
		}

		stress *= stressShocks[index]
		insertTradeRow(signal, "STRESS/EUR", stress, 1, eventAt.Add(time.Duration(index)*time.Second))
	}
}
