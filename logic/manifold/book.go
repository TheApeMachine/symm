package manifold

import (
	"time"

	"github.com/krakenfx/api-go/v2/pkg/book"
)

/*
BookSource exposes SDK-managed L3 order books for manifold ingestion through a
read lease. PeekBook must hold exclusion against websocket book writers for the
duration of fn — ranging Side.Levels without that lease fatals under concurrent
map write.
*/
type BookSource interface {
	PeekBook(symbol string, fn func(*book.Book)) bool
}

/*
physicalOrder is one resting limit order extracted from the authoritative SDK book.
*/
type physicalOrder struct {
	orderID   string
	side      book.BookDirection
	price     float64
	quantity  float64
	timestamp time.Time
}

/*
ordersFromBook walks every visible L3 order on both sides of one symbol book.
Callers must already hold the BookSource read lease for this book.
*/
func ordersFromBook(symbolBook *book.Book) []physicalOrder {
	if symbolBook == nil {
		return nil
	}

	orders := make([]physicalOrder, 0)

	orders = append(orders, ordersFromSide(symbolBook.Bids)...)
	orders = append(orders, ordersFromSide(symbolBook.Asks)...)

	return orders
}

/*
ordersFromSide copies one book's side into physical orders. Callers must hold
the BookSource read lease; Level.Queue also ranges an orders map that writers
mutate under the matching write lease.
*/
func ordersFromSide(side *book.Side) []physicalOrder {
	if side == nil || side.Levels == nil {
		return nil
	}

	levels := make([]*book.Level, 0, len(side.Levels))

	for _, level := range side.Levels {
		levels = append(levels, level)
	}

	orders := make([]physicalOrder, 0)

	for _, level := range levels {
		if level == nil {
			continue
		}

		for _, order := range level.Queue() {
			if order == nil || order.Quantity == nil || order.LimitPrice == nil {
				continue
			}

			quantity := order.Quantity.Float64()

			if quantity <= 0 {
				continue
			}

			orders = append(orders, physicalOrder{
				orderID:   order.ID,
				side:      side.Direction,
				price:     order.LimitPrice.Float64(),
				quantity:  quantity,
				timestamp: order.Timestamp,
			})
		}
	}

	return orders
}
