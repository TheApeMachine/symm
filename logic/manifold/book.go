package manifold

import (
	"iter"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/spot"
)

/*
BookSource exposes SDK-managed L3 order books for manifold ingestion.
*/
type BookSource interface {
	Books() iter.Seq[*spot.BookManager]
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
*/
func ordersFromBook(symbolBook *book.Book) []physicalOrder {
	if symbolBook == nil || symbolBook.Bids == nil || symbolBook.Asks == nil {
		return nil
	}

	orders := make([]physicalOrder, 0)

	orders = append(orders, ordersFromSide(symbolBook.Bids)...)
	orders = append(orders, ordersFromSide(symbolBook.Asks)...)

	return orders
}

func ordersFromSide(side *book.Side) []physicalOrder {
	if side == nil {
		return nil
	}

	orders := make([]physicalOrder, 0)

	for _, level := range side.Levels {
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
