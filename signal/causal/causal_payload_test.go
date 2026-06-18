package causal

import "time"

func seedDefaultTrades(signal *Signal, symbol string, base time.Time) {
	for index := range 64 {
		wobble := float64((index*7)%13) * 0.5
		side := "buy"

		if index%3 == 0 {
			side = "sell"
		}

		feedTrade(
			signal,
			symbol,
			side,
			100+wobble,
			0.02+wobble*0.005,
			base.Add(time.Duration(index)*time.Second),
		)
	}
}

func seedRampTrades(signal *Signal, symbol string, base time.Time) {
	for index := range 64 {
		feedTrade(signal, symbol, "buy", 100+float64(index)*0.5, 0.2+float64(index)*0.01, base.Add(time.Duration(index)*time.Second))
	}
}

func seedLiquidityShockTrades(signal *Signal, symbol string, base time.Time) {
	for index := range 64 {
		qty := 0.05 + float64(index%5)*0.02
		feedTrade(signal, symbol, "sell", 100-float64(index)*0.2, qty, base.Add(time.Duration(index)*time.Second))
	}
}

func seedFlatTrades(signal *Signal, symbol string, base time.Time) {
	for index := range 64 {
		feedTrade(signal, symbol, "buy", 100+float64(index)*0.0001, 1, base.Add(time.Duration(index)*time.Second))
	}
}
