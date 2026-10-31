package kraken

import (
	"fmt"
	"strconv"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
)

/*
TickerRow is one pair from GET /0/public/Ticker. Arrays are [today, last 24h]
except ask/bid/last (touch) and o (today's open).
*/
type TickerRow struct {
	Ask    []string `json:"a"`
	Bid    []string `json:"b"`
	Last   []string `json:"c"`
	Volume []string `json:"v"`
	VWAP   []string `json:"p"`
	Trades []int    `json:"t"`
	Low    []string `json:"l"`
	High   []string `json:"h"`
	Open   string   `json:"o"`
}

type Ticker struct {
	Error  []any                `json:"error"`
	Result map[string]TickerRow `json:"result"`
}

func ParseTicker(buf []byte) (map[string]TickerRow, error) {
	var ticker Ticker

	if err := sonic.Unmarshal(buf, &ticker); err != nil {
		return nil, errnie.Err(
			errnie.UnprocessableContent,
			"[kraken.ticker] invalid ticker frame",
			err,
		)
	}

	if len(ticker.Error) > 0 {
		return nil, errnie.Err(
			errnie.UnprocessableContent,
			"[kraken.ticker] rejected",
			fmt.Errorf("%v", ticker.Error),
		)
	}

	if len(ticker.Result) == 0 {
		return nil, errnie.Err(
			errnie.UnprocessableContent,
			"[kraken.ticker] empty result",
			nil,
		)
	}

	return ticker.Result, nil
}

/*
QuoteVolume24h is last-24h base volume times last-24h VWAP (quote notional).
*/
func (row TickerRow) QuoteVolume24h() float64 {
	base := atIndex(row.Volume, 1)
	vwap := atIndex(row.VWAP, 1)

	if base <= 0 || vwap <= 0 {
		return 0
	}

	return base * vwap
}

/*
ChangePct is percent move from today's open to last trade.
*/
func (row TickerRow) ChangePct() float64 {
	open := parseFloat(row.Open)
	last := atIndex(row.Last, 0)

	if open <= 0 || last <= 0 {
		return 0
	}

	return (last - open) / open * 100
}

func atIndex(vals []string, i int) float64 {
	if i < 0 || i >= len(vals) {
		return 0
	}

	return parseFloat(vals[i])
}

func parseFloat(raw string) float64 {
	if raw == "" {
		return 0
	}

	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0
	}

	return v
}
