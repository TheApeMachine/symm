package kraken

import (
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
)

/*
Side identifies which half of an order book a level belongs to.
*/
type Side string

const (
	SideBid Side = "bid"
	SideAsk Side = "ask"
)

type Book struct {
	Channel string     `json:"channel"`
	Type    string     `json:"type"`
	Data    []BookData `json:"data"`
}

type BookData struct {
	Symbol         string           `json:"symbol"`
	Type           string           `json:"type"`
	PriceIncrement *decimal.Decimal `json:"-"`
	Bids           []BookLevel      `json:"bids"`
	Asks           []BookLevel      `json:"asks"`
	Checksum       uint32           `json:"checksum"`
	Timestamp      time.Time        `json:"timestamp"`
}

type BookLevel struct {
	Price decimal.Decimal `json:"price"`
	Qty   float64         `json:"qty"`
}
