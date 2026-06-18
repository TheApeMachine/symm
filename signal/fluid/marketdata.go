package fluid

import "time"

/*
BookLevel is one price/qty level in a book update payload.
*/
type BookLevel struct {
	Price float64 `json:"price"`
	Qty   float64 `json:"qty"`
}

/*
BookUpdate is the decoded book frame used by fluid symbol state.
*/
type BookUpdate struct {
	Symbol    string      `json:"symbol"`
	Type      string      `json:"type"`
	Timestamp time.Time   `json:"timestamp"`
	Bids      []BookLevel `json:"bids"`
	Asks      []BookLevel `json:"asks"`
}

/*
TickerUpdate is the decoded ticker frame used by fluid symbol state.
*/
type TickerUpdate struct {
	Symbol    string    `json:"symbol"`
	Last      float64   `json:"last"`
	Bid       float64   `json:"bid"`
	Ask       float64   `json:"ask"`
	BidQty    float64   `json:"bid_qty"`
	AskQty    float64   `json:"ask_qty"`
	Change    float64   `json:"change"`
	ChangePct float64   `json:"change_pct"`
	High      float64   `json:"high"`
	Low       float64   `json:"low"`
	Volume    float64   `json:"volume"`
	VWAP      float64   `json:"vwap"`
	Timestamp time.Time `json:"timestamp"`
}
