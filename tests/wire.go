package tests

import (
	"encoding/json"
	"time"
)

/*
wireFrame is the shared Kraken channel envelope decoded by the invariant gate.
*/
type wireFrame[T any] struct {
	Type string `json:"type"`
	Data []T    `json:"data"`
}

/*
wireLevel is one exact-decimal Level2 price aggregate.
*/
type wireLevel struct {
	Price json.Number `json:"price"`
	Qty   json.Number `json:"qty"`
}

/*
wireBook is one simulated Kraken Level2 snapshot or delta.
*/
type wireBook struct {
	Symbol    string      `json:"symbol"`
	Bids      []wireLevel `json:"bids"`
	Asks      []wireLevel `json:"asks"`
	Checksum  uint32      `json:"checksum"`
	Timestamp time.Time   `json:"timestamp"`
}

/*
wireOrder is one exact-decimal Level3 resting-order event.
*/
type wireOrder struct {
	Event     string      `json:"event"`
	OrderID   string      `json:"order_id"`
	Price     json.Number `json:"limit_price"`
	Qty       json.Number `json:"order_qty"`
	Timestamp time.Time   `json:"timestamp"`
}

/*
wireLevel3 is one simulated Kraken Level3 snapshot or delta.
*/
type wireLevel3 struct {
	Symbol    string      `json:"symbol"`
	Bids      []wireOrder `json:"bids"`
	Asks      []wireOrder `json:"asks"`
	Checksum  uint32      `json:"checksum"`
	Timestamp time.Time   `json:"timestamp"`
}

/*
wireTrade is one simulated execution at the authoritative touch.
*/
type wireTrade struct {
	Symbol    string      `json:"symbol"`
	Side      string      `json:"side"`
	Price     json.Number `json:"price"`
	Qty       float64     `json:"qty"`
	TradeID   int64       `json:"trade_id"`
	Timestamp time.Time   `json:"timestamp"`
}

/*
wireTicker is one simulated ticker projection of book and trade state.
*/
type wireTicker struct {
	Symbol    string      `json:"symbol"`
	Bid       json.Number `json:"bid"`
	BidQty    float64     `json:"bid_qty"`
	Ask       json.Number `json:"ask"`
	AskQty    float64     `json:"ask_qty"`
	Last      json.Number `json:"last"`
	Volume    float64     `json:"volume"`
	VWAP      float64     `json:"vwap"`
	Low       json.Number `json:"low"`
	High      json.Number `json:"high"`
	Timestamp time.Time   `json:"timestamp"`
}

/*
orderState retains exact L3 text for lifecycle and checksum reconstruction.
*/
type orderState struct {
	price string
	qty   string
}

/*
tickerState retains the independent accumulated trade facts used to verify
the generated ticker projection.
*/
type tickerState struct {
	volume   float64
	notional float64
	high     float64
	low      float64
	tradeID  int64
	at       time.Time
}
