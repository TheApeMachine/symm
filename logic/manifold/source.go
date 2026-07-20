package manifold

import "github.com/krakenfx/api-go/v2/pkg/book"

/*
ordersForSymbol copies one SDK-managed L3 population under the BookSource read
lease so websocket updates cannot mutate a level during extraction.
*/
func ordersForSymbol(
	source BookSource,
	symbol string,
) ([]physicalOrder, float64, bool) {
	if source == nil || symbol == "" {
		return nil, 0, false
	}

	var (
		orders   []physicalOrder
		midPrice float64
		ready    bool
	)

	source.PeekBook(symbol, func(symbolBook *book.Book) {
		bestBid := symbolBook.BestBid()
		bestAsk := symbolBook.BestAsk()

		if bestBid == nil || bestAsk == nil ||
			bestBid.Price == nil || bestAsk.Price == nil {
			return
		}

		bidPrice := bestBid.Price.Float64()
		askPrice := bestAsk.Price.Float64()

		if bidPrice <= 0 || askPrice <= 0 {
			return
		}

		copied := ordersFromBook(symbolBook)

		if len(copied) == 0 {
			return
		}

		orders = copied
		midPrice = (bidPrice + askPrice) / 2
		ready = true
	})

	return orders, midPrice, ready
}
