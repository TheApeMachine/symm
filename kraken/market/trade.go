package market

import (
	"time"
)

/*
TradeParams is the Kraken WebSocket v2 subscribe payload for the trade channel.
*/
type TradeParams struct {
	Channel  string   `json:"channel"`
	Symbol   []string `json:"symbol"`
	Snapshot bool     `json:"snapshot"`
}

/*
TradeUpdate is one executed trade from the public trade WebSocket feed.

One executed trade from the public tape: price, size, aggressor side, order type,
ID, and time. It is the ground truth of what actually transacted rather than what
was merely quoted -- the aggressor side reveals which side lifted or hit, and the
stream is the finest-grained record of realized volume and price discovery as it
happens.
*/
type TradeUpdate struct {
	Symbol    string    `json:"symbol"`
	Side      string    `json:"side"`
	Price     float64   `json:"price"`
	Qty       float64   `json:"qty"`
	OrdType   string    `json:"ord_type"`
	TradeID   int64     `json:"trade_id"`
	Timestamp time.Time `json:"timestamp"`
	Type      string    `json:"-"`
}

/*
SetEnvelopeType records the channel envelope tag (snapshot or update).
*/
func (trade *TradeUpdate) SetEnvelopeType(kind string) {
	trade.Type = kind
}
