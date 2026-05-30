package market

/*
Level3Order is one resting order from POST /private/Level3.
See https://docs.kraken.com/api/docs/rest-api/get-level-3-order-book

Fetching requires an authenticated private REST client; this package only models
the response shape. WebSocket level3 parsing lives in level3.go.
*/
type Level3Order struct {
	OrderID    string `json:"order_id"`
	LimitPrice string `json:"limit_price"`
	OrderQty   string `json:"order_qty"`
	Timestamp  string `json:"timestamp"`
}

/*
Level3Book is the Level3 order book payload for one symbol.

A point-in-time order-by-order snapshot: every resting order's ID, price, size,
and time on each side. It is the authoritative per-order state of the book -- the
exact picture needed to establish or reconcile order-level truth at a single
instant.
*/
type Level3Book struct {
	Symbol string        `json:"symbol"`
	Bids   []Level3Order `json:"bids"`
	Asks   []Level3Order `json:"asks"`
}
