package public

import (
	"fmt"
	"time"

	"github.com/bytedance/sonic"
)

const ChartOhlcIntervalMinutes = 1

/*
OhlcRow is one forming or closed bar from the Kraken v2 ohlc feed.
See https://docs.kraken.com/api/docs/websocket-v2/ohlc
*/
type OhlcRow struct {
	Symbol        string  `json:"symbol"`
	Open          float64 `json:"open"`
	High          float64 `json:"high"`
	Low           float64 `json:"low"`
	Close         float64 `json:"close"`
	VWAP          float64 `json:"vwap"`
	Trades        float64 `json:"trades"`
	Volume        float64 `json:"volume"`
	IntervalBegin string  `json:"interval_begin"`
	Interval      int     `json:"interval"`
	Type          string  `json:"-"`
}

/*
OhlcSubscribeFrame builds a Kraken v2 ohlc subscribe request for one chart symbol.
*/
func OhlcSubscribeFrame(symbol string) map[string]any {
	return map[string]any{
		"method": "subscribe",
		"params": map[string]any{
			"channel":  CandlesChannel,
			"symbol":   []string{symbol},
			"interval": ChartOhlcIntervalMinutes,
			"snapshot": true,
		},
	}
}

/*
DecodeOhlc decodes snapshot and update rows from an ohlc channel envelope.
*/
func DecodeOhlc(message *SocketMessage) ([]OhlcRow, error) {
	var candles []OhlcRow

	if err := sonic.Unmarshal(message.Data, &candles); err != nil {
		return nil, err
	}

	for index := range candles {
		candles[index].Type = message.Type
	}

	return candles, nil
}

/*
CandleIntervalSec maps interval_begin to unix seconds for chart bar identity.
*/
func CandleIntervalSec(candle OhlcRow) (int64, error) {
	if candle.IntervalBegin == "" {
		return 0, fmt.Errorf("kraken ohlc: missing interval_begin for %s", candle.Symbol)
	}

	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		at, err := time.Parse(layout, candle.IntervalBegin)

		if err != nil {
			continue
		}

		return at.UTC().Unix(), nil
	}

	return 0, fmt.Errorf(
		"kraken ohlc: parse interval_begin %q for %s",
		candle.IntervalBegin,
		candle.Symbol,
	)
}
