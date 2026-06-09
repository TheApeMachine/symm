package futures

import (
	"github.com/theapemachine/symm/kraken/market"
)

type BookSnapshot struct {
	Feed      string             `json:"feed"`
	ProductID string             `json:"product_id"`
	Timestamp int64              `json:"timestamp"`
	Seq       int                `json:"seq"`
	TickSize  any                `json:"tickSize"` // Always null
	Bids      []market.BookLevel `json:"bids"`
	Asks      []market.BookLevel `json:"asks"`
}

type BookDelta struct {
	Feed      string `json:"feed"`
	ProductID string `json:"product_id"`
	Side      string `json:"side"`
	Seq       int    `json:"seq"`
	Price     int    `json:"price"`
	Qty       int    `json:"qty"`
	Timestamp int64  `json:"timestamp"`
}
