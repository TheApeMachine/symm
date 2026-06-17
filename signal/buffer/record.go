package buffer

import "time"

/*
BookLevelRecord is one price level stored in a book feed element.
*/
type BookLevelRecord struct {
	Price float64
	Qty   float64
}

/*
BookRecord is one L2 book frame stored in the book ring.
*/
type BookRecord struct {
	Symbol    string
	Timestamp time.Time
	Bids      []BookLevelRecord
	Asks      []BookLevelRecord
}

/*
TradeRecord is one executed trade stored in the trade ring.
*/
type TradeRecord struct {
	Symbol    string
	Side      string
	Price     float64
	Qty       float64
	Timestamp time.Time
}

/*
TickerRecord is one ticker row stored in the ticker ring.
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

/*
SymbolWindow summarizes ring history for one symbol.
*/
type SymbolWindow struct {
	Prices        []float64
	Spreads       []float64
	LatestElement []byte
	Elapsed       float64
}

/*
TradeSnapshot is the latest trade-derived quote for one symbol.
*/
type TradeSnapshot struct {
	Price    float64
	Volume   float64
	Elapsed  float64
	Observed time.Time
}

/*
TickerSnapshot is the latest ticker row for one symbol.
*/
type TickerSnapshot struct {
	Last      float64
	Bid       float64
	Ask       float64
	Volume    float64
	ChangePct float64
	Elapsed   float64
	Observed  time.Time
}
