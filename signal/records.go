package signal

import "time"

/*
TradeRecord is one executed trade observation from a feed artifact payload.
*/
type TradeRecord struct {
	Symbol    string
	Side      string
	Price     float64
	Qty       float64
	Timestamp time.Time
}

/*
BookLevelRecord is one price level in an L2 book observation.
*/
type BookLevelRecord struct {
	Price float64
	Qty   float64
}

/*
BookRecord is one L2 book observation from a feed artifact payload.
*/
type BookRecord struct {
	Symbol    string
	Bids      []BookLevelRecord
	Asks      []BookLevelRecord
	Timestamp time.Time
}

/*
TickerRecord is one ticker observation from a feed artifact payload.
*/
type TickerRecord struct {
	Symbol    string
	Ask       float64
	AskQty    float64
	Bid       float64
	BidQty    float64
	Change    float64
	ChangePct float64
	High      float64
	Last      float64
	Low       float64
	Volume    float64
	VWAP      float64
	Timestamp time.Time
}
