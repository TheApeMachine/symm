package manifold

/*
ordersForSymbol copies one SDK-managed L3 population while BookSource retains
its read lease, so websocket updates cannot mutate a level during extraction.
*/
func ordersForSymbol(
	source BookSource,
	symbol string,
) ([]physicalOrder, float64, bool) {
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

		orders := ordersFromBook(symbolBook)

		if len(orders) == 0 {
			continue
		}

		return orders, (bidPrice + askPrice) / 2, true
	}

	return nil, 0, false
}
