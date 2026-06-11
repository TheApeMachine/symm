package market

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken/types"
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

func NewTickerParams(symbols []string) json.RawMessage {
	params := &TickerParams{
		Channel:  "ticker",
		Symbol:   symbols,
		Snapshot: true,
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

func (ticker *TickerUpdate) Unmarshal(message *types.SocketMessage) error {
	if err := sonic.Unmarshal(message.Data, ticker); err != nil {
		return err
	}

	ticker.Type = message.Type

	return nil
}

type TickerUpdates []*TickerUpdate

func (updates *TickerUpdates) Unmarshal(message *types.SocketMessage) error {
	if err := sonic.Unmarshal(message.Data, updates); err != nil {
		return err
	}

	for _, update := range *updates {
		if update == nil {
			continue
		}

		update.Type = message.Type
	}

	return nil
}

/*
ResolvePrice returns the best available tradeable price from a ticker row.
*/
func (ticker *TickerUpdate) ResolvePrice() (float64, error) {
	if ticker == nil {
		return 0, fmt.Errorf("kraken: ticker update is nil")
	}

	price := ticker.Last

	if price <= 0 {
		price = (ticker.Ask + ticker.Bid) / 2
	}

	if price <= 0 {
		return 0, fmt.Errorf("kraken: ticker %q: price is zero or negative", ticker.Symbol)
	}

	return price, nil
}

/*
CompleteSymbol builds a full cross-section row from one ticker update.
*/
func (ticker *TickerUpdate) CompleteSymbol(at time.Time, pressure float64) (*Symbol, error) {
	price, err := ticker.ResolvePrice()

	if err != nil {
		return nil, err
	}

	quoteVolume := ticker.Volume * price

	if quoteVolume <= 0 {
		return nil, fmt.Errorf("kraken: ticker %q: volume is zero or negative", ticker.Symbol)
	}

	row := &Symbol{
		Name:     ticker.Symbol,
		Price:    price,
		Value:    ticker.ChangePct,
		Volume:   quoteVolume,
		Pressure: pressure,
		Updated:  at,
	}

	if err := row.Validate(); err != nil {
		return nil, err
	}

	return row, nil
}
