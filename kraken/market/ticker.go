package market

import (
	"encoding/json"
	"time"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken/types"
)

const (
	TickerTriggerBBO    = "bbo"
	TickerTriggerTrades = "trades"
)

/*
TickerParams is the Kraken WebSocket v2 subscribe payload for the ticker channel.
*/
type TickerParams struct {
	Channel      string   `json:"channel"`
	Symbol       []string `json:"symbol"`
	Snapshot     bool     `json:"snapshot"`
	EventTrigger string   `json:"event_trigger,omitempty"`
}

/*
TickerTriggers returns both Kraken ticker event triggers.

Kraken accepts one trigger per subscription; subscribing to both keeps touch
quotes fresh on illiquid symbols (bbo) while preserving trade-driven 24h stats.
*/
func TickerTriggers() []string {
	return []string{TickerTriggerBBO, TickerTriggerTrades}
}

func NewTickerParams(symbols []string, eventTrigger string) json.RawMessage {
	params := &TickerParams{
		Channel:      "ticker",
		Symbol:       symbols,
		Snapshot:     true,
		EventTrigger: eventTrigger,
	}

	raw, err := sonic.Marshal(params)

	if errnie.Error(err) != nil {
		return nil
	}

	return json.RawMessage(raw)
}

/*
TickerUpdate is one top-of-book and 24h summary row from the public ticker feed.

A live rolling 24-hour summary per symbol, pushed on change: best bid and ask with
their sizes, last price, session high and low, volume, VWAP, and the absolute and
percent change. It is the lowest-latency at-a-glance state of a market, already
reduced to the day's move and activity without computing it from the tape.
*/
type TickerUpdate struct {
	Symbol    string    `json:"symbol"`
	Ask       float64   `json:"ask"`
	AskQty    float64   `json:"ask_qty"`
	Bid       float64   `json:"bid"`
	BidQty    float64   `json:"bid_qty"`
	Change    float64   `json:"change"`
	ChangePct float64   `json:"change_pct"`
	High      float64   `json:"high"`
	Last      float64   `json:"last"`
	Low       float64   `json:"low"`
	Volume    float64   `json:"volume"`
	VWAP      float64   `json:"vwap"`
	Timestamp time.Time `json:"timestamp"`
	Type      string    `json:"-"`
}

func (ticker *TickerUpdate) Marshal() []byte {
	raw, err := sonic.Marshal(ticker)

	if errnie.Error(err) != nil {
		return nil
	}

	return raw
}

func (ticker *TickerUpdate) Unmarshal(message *types.SocketMessage) error {
	if err := sonic.Unmarshal(message.Data, ticker); err != nil {
		return err
	}

	ticker.Type = message.Type

	return nil
}

type TickerUpdates []*TickerUpdate

func (updates *TickerUpdates) Unmarshal(message *types.SocketMessage) error {
	return sonic.Unmarshal(message.Data, updates)
}
