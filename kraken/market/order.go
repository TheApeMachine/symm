package market

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// OrderSnapshot is the envelope type tag for a full L3 book frame after subscribe.
const OrderSnapshot = "snapshot"

/*
OrderTokenSource supplies short-lived authenticated WebSocket tokens for level3.
*/
type OrderTokenSource interface {
	Token(context.Context) (string, error)
}

/*
OrderParams is the Kraken WebSocket v2 subscribe payload for the level3 channel.
*/
type OrderParams struct {
	Channel  string   `json:"channel"`
	Symbol   []string `json:"symbol"`
	Depth    int      `json:"depth"`
	Snapshot bool     `json:"snapshot"`
	Token    string   `json:"token"`
}

/*
OrderEvent is one resting order or order event on the L3 book feed.

Snapshots list orders with order_id, limit_price, order_qty, and timestamp.
Updates add an event field: add, modify, or delete.
*/
type OrderEvent struct {
	Event      string  `json:"event,omitempty"`
	OrderID    string  `json:"order_id"`
	LimitPrice float64 `json:"limit_price"`
	OrderQty   float64 `json:"order_qty"`
	Timestamp  string  `json:"timestamp"`
}

/*
Order is one L3 order book snapshot or update from the authenticated level3 feed.

Each frame carries per-order bids and asks, a CRC32 checksum over the top ten
price levels per side, and an RFC3339 timestamp. Type records the envelope tag
(snapshot vs update) from the channel message, not the data payload.
*/
type Order struct {
	Symbol    string       `json:"symbol"`
	Bids      []OrderEvent `json:"bids"`
	Asks      []OrderEvent `json:"asks"`
	Checksum  int64        `json:"checksum"`
	Timestamp string       `json:"timestamp"`
	Type      string       `json:"-"`
}

/*
SetEnvelopeType records the channel envelope tag (snapshot or update).
*/
func (order *Order) SetEnvelopeType(kind string) {
	order.Type = kind
}

/*
IsSnapshot reports whether this frame is a full L3 book snapshot.
*/
func (order *Order) IsSnapshot() bool {
	return order.Type == OrderSnapshot
}

var (
	orderTokenSourceMu sync.RWMutex
	orderTokenSource   OrderTokenSource
)

/*
SetOrderTokenSource enables authenticated L3 market data. Pass nil to disable.
*/
func SetOrderTokenSource(source OrderTokenSource) {
	orderTokenSourceMu.Lock()
	defer orderTokenSourceMu.Unlock()

	orderTokenSource = source
}

/*
OrderAvailable reports whether authenticated L3 market data is configured.
*/
func OrderAvailable() bool {
	orderTokenSourceMu.RLock()
	defer orderTokenSourceMu.RUnlock()

	return orderTokenSource != nil
}

/*
OrderEventTime parses an L3 event timestamp, falling back to now when absent.
*/
func OrderEventTime(raw string, fallback time.Time) time.Time {
	if raw == "" {
		return fallback
	}

	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		parsed, err := time.Parse(layout, raw)

		if err == nil {
			return parsed
		}
	}

	return fallback
}

// Level3 names alias the level3 WebSocket types for existing callers.

type (
	Level3TokenSource = OrderTokenSource
	Level3Params      = OrderParams
	Level3OrderEvent  = OrderEvent
	Level3Update      = Order
)

func SetLevel3TokenSource(source Level3TokenSource) {
	SetOrderTokenSource(source)
}

func Level3Available() bool {
	return OrderAvailable()
}

func Level3EventTime(raw string, fallback time.Time) time.Time {
	return OrderEventTime(raw, fallback)
}

func orderToken(ctx context.Context) (string, error) {
	orderTokenSourceMu.RLock()
	source := orderTokenSource
	orderTokenSourceMu.RUnlock()

	if source == nil {
		return "", fmt.Errorf("market: level3 token source not configured")
	}

	return source.Token(ctx)
}
