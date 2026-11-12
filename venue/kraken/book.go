package kraken

import (
	"time"

	"github.com/shopspring/decimal"
)

/*
Side identifies which half of an order book a level belongs to.
*/
type Side string

const (
	SideBid Side = "bid"
	SideAsk Side = "ask"
)

/*
Book is the legacy aggregated ws book shape kept for decode compatibility;
symm does not subscribe here — broker.Book consumes Level3 instead.
*/
type Book struct {
	Channel string     `json:"channel"`
	Type    string     `json:"type"`
	Data    []BookData `json:"data"`
}

/*
BookData is one symbol snapshot on the aggregated book channel with venue checksum.
*/
type BookData struct {
	Symbol         string           `json:"symbol"`
	Type           string           `json:"type"`
	PriceIncrement *decimal.Decimal `json:"-"`
	Bids           []BookLevel      `json:"bids"`
	Asks           []BookLevel      `json:"asks"`
	Checksum       uint32           `json:"checksum"`
	Timestamp      time.Time        `json:"timestamp"`
}

/*
BookLevel is price/qty on the aggregated channel — no order id, unlike Level3.
*/
type BookLevel struct {
	Price decimal.Decimal `json:"price"`
	Qty   float64         `json:"qty"`
}
