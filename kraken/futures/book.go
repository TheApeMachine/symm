package futures

import (
	"fmt"
	"strconv"
	"time"

	"github.com/theapemachine/symm/kraken/market"
)

const (
	bookSnapshotFeed = "book_snapshot"
	bookDeltaFeed    = "book"
)

type bookLevel struct {
	Price float64 `json:"price"`
	Qty   float64 `json:"qty"`
}

type bookSnapshotMessage struct {
	Feed      string      `json:"feed"`
	ProductID string      `json:"product_id"`
	Timestamp int64       `json:"timestamp"`
	Seq       int64       `json:"seq"`
	TickSize  float64     `json:"tick_size"`
	Bids      []bookLevel `json:"bids"`
	Asks      []bookLevel `json:"asks"`
}

type bookDeltaMessage struct {
	Feed      string  `json:"feed"`
	ProductID string  `json:"product_id"`
	Side      string  `json:"side"`
	Seq       int64   `json:"seq"`
	Price     float64 `json:"price"`
	Qty       float64 `json:"qty"`
	Timestamp int64   `json:"timestamp"`
}

/*
BookFromSnapshot converts a futures book snapshot frame into the shared L2 book type.
*/
func BookFromSnapshot(message bookSnapshotMessage) (market.Book, error) {
	identity, err := market.FuturesIdentityFromProduct(message.ProductID)

	if err != nil {
		return market.Book{}, err
	}

	book := market.Book{
		Symbol:    identity.Symbol,
		Bids:      levelsFromSnapshot(message.Bids),
		Asks:      levelsFromSnapshot(message.Asks),
		Timestamp: timestampString(message.Timestamp),
	}
	book.SetEnvelopeType("snapshot")
	book.SetInstrumentIdentity(identity)

	return book, nil
}

/*
BookFromDelta converts one futures book delta into an incremental L2 update.
*/
func BookFromDelta(message bookDeltaMessage) (market.Book, error) {
	identity, err := market.FuturesIdentityFromProduct(message.ProductID)

	if err != nil {
		return market.Book{}, err
	}

	level := market.BookLevel{
		Price: message.Price,
		Qty:   message.Qty,
	}

	book := market.Book{
		Symbol:    identity.Symbol,
		Timestamp: timestampString(message.Timestamp),
	}

	if message.Side == "sell" {
		book.Asks = []market.BookLevel{level}
	}

	if message.Side == "buy" {
		book.Bids = []market.BookLevel{level}
	}

	book.SetEnvelopeType("update")
	book.SetInstrumentIdentity(identity)

	return book, nil
}

func levelsFromSnapshot(levels []bookLevel) []market.BookLevel {
	if len(levels) == 0 {
		return nil
	}

	out := make([]market.BookLevel, len(levels))

	for index, level := range levels {
		out[index] = market.BookLevel{
			Price: level.Price,
			Qty:   level.Qty,
		}
	}

	return out
}

func timestampString(unixMillis int64) string {
	if unixMillis <= 0 {
		return time.Now().UTC().Format(time.RFC3339Nano)
	}

	return time.UnixMilli(unixMillis).UTC().Format(time.RFC3339Nano)
}

func parseSubscribeProducts(raw any) ([]string, error) {
	message, ok := raw.(SubscribeMessage)

	if !ok {
		return nil, fmt.Errorf("futures: invalid subscribe payload")
	}

	if len(message.ProductIDs) == 0 {
		return nil, fmt.Errorf("futures: subscribe product list is empty")
	}

	return message.ProductIDs, nil
}

func parseReqID(raw any) int64 {
	switch value := raw.(type) {
	case int64:
		return value
	case float64:
		return int64(value)
	case string:
		parsed, err := strconv.ParseInt(value, 10, 64)

		if err != nil {
			return 0
		}

		return parsed
	default:
		return 0
	}
}
