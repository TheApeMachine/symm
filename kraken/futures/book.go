package futures

import (
	"encoding/json"
	"time"

	"github.com/bytedance/sonic"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
)

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
	Bids      []krakenmarket.BookLevel `json:"bids"`
	Asks      []krakenmarket.BookLevel `json:"asks"`
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

type wireFrame struct {
	Event   string          `json:"event"`
	Feed    string          `json:"feed"`
	Message string          `json:"message"`
	Raw     json.RawMessage `json:"-"`
}

func decodeWireFrame(payload []byte) (wireFrame, error) {
	frame := wireFrame{}

	if err := sonic.Unmarshal(payload, &frame); err != nil {
		return wireFrame{}, err
	}

	frame.Raw = payload

	return frame, nil
}

func parseBookSnapshot(payload []byte) (BookSnapshot, error) {
	snapshot := BookSnapshot{}

	if err := sonic.Unmarshal(payload, &snapshot); err != nil {
		return BookSnapshot{}, err
	}

	return snapshot, nil
}

func parseBookDelta(payload []byte) (BookDelta, error) {
	delta := BookDelta{}

	if err := sonic.Unmarshal(payload, &delta); err != nil {
		return BookDelta{}, err
	}

	return delta, nil
}

func snapshotTimestamp(snapshot BookSnapshot) time.Time {
	if snapshot.Timestamp <= 0 {
		return time.Now()
	}

	return time.UnixMilli(snapshot.Timestamp)
}

func deltaTimestamp(delta BookDelta) time.Time {
	if delta.Timestamp <= 0 {
		return time.Now()
	}

	return time.UnixMilli(delta.Timestamp)
}
