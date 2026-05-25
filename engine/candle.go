package engine

import "time"

/*
OHLCCandle is one completed or in-progress Kraken OHLC interval.
*/
type OHLCCandle struct {
	Time                   time.Time
	Open, High, Low, Close float64
	Volume                 float64
}

/*
CompletedCandles drops Kraken's trailing in-progress candle.
*/
func CompletedCandles(candles []OHLCCandle) []OHLCCandle {
	if len(candles) <= 1 {
		return nil
	}

	return candles[:len(candles)-1]
}

/*
OHLCWarmer seeds signal track stores from historical candles.
*/
type OHLCWarmer interface {
	WarmFromOHLC(candles map[string][]OHLCCandle)
}

/*
MinCompletedLength returns the shortest completed-candle count across symbols.
*/
func MinCompletedLength(candles map[string][]OHLCCandle) int {
	length := 0

	for _, bars := range candles {
		size := len(CompletedCandles(bars))

		if size == 0 {
			continue
		}

		if length == 0 || size < length {
			length = size
		}
	}

	return length
}
