package tests

import (
	"encoding/json"
	"time"

	tes "github.com/theapemachine/symm/tests/types"
)

/*
candleState aggregates the same executed samples published on the trade and
ticker channels into one live OHLC interval.
*/
type candleState struct {
	begin       time.Time
	end         time.Time
	open        float64
	high        float64
	low         float64
	close       float64
	trades      int64
	volume      float64
	tradedValue float64
}

/*
renderCandle updates and renders one coherent Kraken OHLC observation.
*/
func (market *Market) renderCandle(sample tes.Sample) []byte {
	candle := market.candles[sample.Symbol]
	interval := market.Config.CandleInterval
	begin := sample.Timestamp.Truncate(interval)

	if candle == nil || !sample.Timestamp.Before(candle.end) {
		candle = &candleState{
			begin: begin,
			end:   begin.Add(interval),
			open:  sample.Last,
			high:  sample.Last,
			low:   sample.Last,
		}
		market.candles[sample.Symbol] = candle
	}

	candle.high = max(candle.high, sample.Last)
	candle.low = min(candle.low, sample.Last)
	candle.close = sample.Last
	candle.trades++
	candle.volume += sample.StepVolume
	candle.tradedValue += sample.Last * sample.StepVolume
	vwap := candle.tradedValue / candle.volume

	payload, err := json.Marshal(map[string]any{
		"channel":   "ohlc",
		"type":      "update",
		"timestamp": sample.Timestamp.Format(time.RFC3339Nano),
		"data": []map[string]any{{
			"symbol":         sample.Symbol,
			"open":           candle.open,
			"high":           candle.high,
			"low":            candle.low,
			"close":          candle.close,
			"trades":         candle.trades,
			"volume":         candle.volume,
			"vwap":           vwap,
			"interval_begin": candle.begin.Format(time.RFC3339Nano),
			"interval":       int64(interval / time.Minute),
			"timestamp":      candle.end.Format(time.RFC3339Nano),
		}},
	})

	if err != nil {
		panic(err)
	}

	return payload
}
