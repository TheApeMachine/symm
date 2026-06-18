package futures

import "encoding/json"

/*
BookLevel is one price/qty level in a futures book frame.
*/
type BookLevel struct {
	Price float64 `json:"price"`
	Qty   float64 `json:"qty"`
}

const (
	FeedBookSnapshot = "book_snapshot"
	FeedBookDelta    = "book"
)

type BookSnapshot struct {
	Feed      string                   `json:"feed"`
	ProductID string                   `json:"product_id"`
	Timestamp int64                    `json:"timestamp"`
	Seq       int                      `json:"seq"`
	TickSize  json.RawMessage          `json:"tickSize"`
	Bids      []BookLevel `json:"bids"`
	Asks      []BookLevel `json:"asks"`
}

type BookDelta struct {
	Feed      string  `json:"feed"`
	ProductID string  `json:"product_id"`
	Side      string  `json:"side"`
	Seq       int     `json:"seq"`
	Price     float64 `json:"price"`
	Qty       float64 `json:"qty"`
	Timestamp int64   `json:"timestamp"`
}
