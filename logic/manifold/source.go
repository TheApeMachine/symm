package manifold

import (
	"github.com/krakenfx/api-go/v2/pkg/book"
)

/*
bookForSymbol resolves one SDK-managed L3 book and its midpoint for ingestion.
*/
func bookForSymbol(
	source BookSource,
	symbol string,
) (*book.Book, float64, bool) {
	if source == nil || symbol == "" {
		return nil, 0, false
	}

	for manager := range source.Books() {
		if manager == nil {
			continue
		}

		symbolBook := manager.GetBook(symbol)

		if symbolBook == nil {
			continue
		}

		bestBid := symbolBook.BestBid()
		bestAsk := symbolBook.BestAsk()

		if bestBid == nil || bestAsk == nil ||
			bestBid.Price == nil || bestAsk.Price == nil {
			continue
		}

		bidPrice := bestBid.Price.Float64()
		askPrice := bestAsk.Price.Float64()

		if bidPrice <= 0 || askPrice <= 0 {
			continue
		}

		return symbolBook, (bidPrice + askPrice) / 2, true
	}

	return nil, 0, false
}
