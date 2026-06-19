package correlation

import (
	"time"
)

type tradeUpdate struct {
	Symbol    string    `json:"symbol"`
	Price     float64   `json:"price"`
	Qty       float64   `json:"qty"`
	Timestamp time.Time `json:"timestamp"`
}

type tickerUpdate struct {
	Symbol    string    `json:"symbol"`
	Last      float64   `json:"last"`
	Bid       float64   `json:"bid"`
	Ask       float64   `json:"ask"`
	BidQty    float64   `json:"bid_qty"`
	AskQty    float64   `json:"ask_qty"`
	Volume    float64   `json:"volume"`
	Timestamp time.Time `json:"timestamp"`
}
