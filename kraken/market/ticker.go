package market

import (
	"encoding/json"
	"errors"
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

/*
ResolvePrice picks the best available price from a ticker row.
*/
func (ticker *TickerUpdate) ResolvePrice() (float64, error) {
	if ticker.Last > 0 {
		return ticker.Last, nil
	}

	if ticker.Bid > 0 && ticker.Ask > ticker.Bid {
		return (ticker.Ask + ticker.Bid) / 2, nil
	}

	return 0, errnie.Error(errors.New("kraken: ticker price is required"))
}

/*
ResolveValue derives a dimensionless fractional move from ticker fields.

When ChangePct is present it is returned directly. Otherwise the method
normalizes absolute change, session range, or touch spread by price so
callers receive a unitless ratio rather than a quote-currency amount.
*/
func (ticker *TickerUpdate) ResolveValue() (float64, error) {
	if ticker.ChangePct != 0 {
		return ticker.ChangePct, nil
	}

	price, err := ticker.ResolvePrice()

	if err != nil {
		return 0, err
	}

	if price <= 0 {
		return 0, errnie.Error(errors.New("kraken: ticker price is required"))
	}

	if ticker.Change != 0 {
		return ticker.Change / price, nil
	}

	if ticker.High > 0 && ticker.Low > 0 && ticker.High > ticker.Low {
		return (ticker.High - ticker.Low) / price, nil
	}

	if ticker.Bid > 0 && ticker.Ask > ticker.Bid {
		return (ticker.Ask - ticker.Bid) / price, nil
	}

	return 0, errnie.Error(errors.New("kraken: ticker value is required"))
}

/*
ResolveVolume picks the best available volume from a ticker row.

24h volume and touch quantities may be zero on book-triggered updates before
session prints; in that case volume is derived from the touch spread relative
to price, matching ResolveValue microstructure fallback.
*/
func (ticker *TickerUpdate) ResolveVolume(price float64) (float64, error) {
	if ticker.Volume > 0 {
		return ticker.Volume, nil
	}

	touchQty := ticker.AskQty + ticker.BidQty

	if touchQty > 0 {
		return touchQty, nil
	}

	if price <= 0 {
		return 0, errnie.Error(errors.New("kraken: ticker price is required"))
	}

	if ticker.Bid > 0 && ticker.Ask > ticker.Bid {
		return (ticker.Ask - ticker.Bid) / price, nil
	}

	// No quantity-based volume is available on partial ticker rows.
	return 0, errnie.Error(errors.New("kraken: volume is required"))
}

/*
CompleteSymbol builds a full cross-section row from one ticker update.
*/
func (ticker *TickerUpdate) CompleteSymbol(pressure float64, at time.Time) (*Symbol, error) {
	price, err := ticker.ResolvePrice()

	if err != nil {
		return nil, err
	}

	value, err := ticker.ResolveValue()

	if err != nil {
		return nil, err
	}

	volume, err := ticker.ResolveVolume(price)

	if err != nil {
		return nil, err
	}

	return NewSymbolRow(ticker.Symbol, price, value, volume, pressure, at)
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
