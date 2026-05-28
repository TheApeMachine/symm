// Package level3 parses Kraken WebSocket v2 "level3" (order-by-order) frames
// into a neutral Order event the toxicity tracker consumes. The level3 channel
// streams individual resting orders with an exact matching-engine timestamp, so
// order age is exact rather than inferred. A delete carries no reason, so
// fill-vs-cancel is resolved downstream by joining the public trade tape.
package level3

import (
	"encoding/json"
	"time"
)

// SideBid and SideAsk are the byte side codes the toxicity tracker expects.
const (
	SideBid byte = 'b'
	SideAsk byte = 'a'
)

// Order is one parsed level3 event. Event is "add", "delete", or "amend"
// (snapshot entries are reported as "add"). Side is 'b' for a bid, 'a' for an
// ask. Ts is the order's matching-engine timestamp; it falls back to the parse
// time when the wire timestamp is missing or malformed.
type Order struct {
	Symbol  string
	Event   string
	OrderID string
	Side    byte
	Price   float64
	Qty     float64
	Ts      time.Time
}

type wireOrder struct {
	Event      string  `json:"event"`
	OrderID    string  `json:"order_id"`
	LimitPrice float64 `json:"limit_price"`
	OrderQty   float64 `json:"order_qty"`
	Timestamp  string  `json:"timestamp"`
}

type wireData struct {
	Symbol string      `json:"symbol"`
	Bids   []wireOrder `json:"bids"`
	Asks   []wireOrder `json:"asks"`
}

type wireFrame struct {
	Channel string     `json:"channel"`
	Type    string     `json:"type"`
	Data    []wireData `json:"data"`
}

// ParseOrders decodes a level3 channel frame into a flat list of Order events.
// fallbackTime stamps any entry whose wire timestamp is absent or unparseable.
// ok is false for non-level3 frames (heartbeats, status, acks) and empty data.
func ParseOrders(payload []byte, fallbackTime time.Time) (orders []Order, ok bool) {
	var frame wireFrame

	if err := json.Unmarshal(payload, &frame); err != nil {
		return nil, false
	}

	if frame.Channel != "level3" || len(frame.Data) == 0 {
		return nil, false
	}

	// A snapshot lists every resting order with no per-entry event; treat each
	// as an add so the tracker's per-order book is seeded.
	snapshot := frame.Type == "snapshot"

	for _, data := range frame.Data {
		orders = appendSide(orders, data.Symbol, SideBid, data.Bids, snapshot, fallbackTime)
		orders = appendSide(orders, data.Symbol, SideAsk, data.Asks, snapshot, fallbackTime)
	}

	return orders, len(orders) > 0
}

func appendSide(
	orders []Order, symbol string, side byte, entries []wireOrder, snapshot bool, fallbackTime time.Time,
) []Order {
	for _, entry := range entries {
		if entry.OrderID == "" {
			continue
		}

		event := entry.Event

		if snapshot || event == "" {
			event = "add"
		}

		orders = append(orders, Order{
			Symbol:  symbol,
			Event:   event,
			OrderID: entry.OrderID,
			Side:    side,
			Price:   entry.LimitPrice,
			Qty:     entry.OrderQty,
			Ts:      parseTimestamp(entry.Timestamp, fallbackTime),
		})
	}

	return orders
}

func parseTimestamp(value string, fallbackTime time.Time) time.Time {
	if value == "" {
		return fallbackTime
	}

	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05.000000Z"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed
		}
	}

	return fallbackTime
}
