package market

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/symm/kraken/types"
)

/*
CandleParams is the Kraken WebSocket v2 subscribe payload for the ohlc channel.
*/
type CandleParams struct {
	Channel  string   `json:"channel"`
	Symbol   []string `json:"symbol"`
	Interval int      `json:"interval"`
	Snapshot bool     `json:"snapshot"`
}

/*
CandleUpdate is one forming or closed OHLC bar from the public ohlc feed.
*/
type CandleUpdate struct {
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
}

func NewCandleParams(symbols []string, intervalMinutes int) (json.RawMessage, error) {
	params := &CandleParams{
		Channel:  "ohlc",
		Symbol:   symbols,
		Interval: intervalMinutes,
		Snapshot: true,
	}

	raw, err := sonic.Marshal(params)

	if err != nil {
		return nil, fmt.Errorf("market: marshal ohlc params: %w", err)
	}

	return json.RawMessage(raw), nil
}

func (candle *CandleUpdate) Unmarshal(message *types.SocketMessage) error {
	return sonic.Unmarshal(message.Data, candle)
}

type CandleUpdates []*CandleUpdate

func (updates *CandleUpdates) Unmarshal(message *types.SocketMessage) error {
	return sonic.Unmarshal(message.Data, updates)
}

/*
IntervalSec parses interval_begin into unix seconds for the trade chart wire.
*/
func (candle *CandleUpdate) IntervalSec() (int64, error) {
	if candle.IntervalBegin == "" {
		return 0, fmt.Errorf("candle: empty interval_begin")
	}

	parsed, err := time.Parse(time.RFC3339Nano, candle.IntervalBegin)

	if err != nil {
		parsed, err = time.Parse(time.RFC3339, candle.IntervalBegin)
	}

	if err != nil {
		return 0, fmt.Errorf("candle: interval_begin: %w", err)
	}

	return parsed.Unix(), nil
}

/*
UIFrame is the websocket payload expected by the trade chart adapter.
*/
func (candle *CandleUpdate) UIFrame() (map[string]any, error) {
	sec, err := candle.IntervalSec()

	if err != nil {
		return nil, err
	}

	return map[string]any{
		"symbol": candle.Symbol,
		"sec":    sec,
		"open":   candle.Open,
		"high":   candle.High,
		"low":    candle.Low,
		"close":  candle.Close,
		"volume": candle.Volume,
	}, nil
}
