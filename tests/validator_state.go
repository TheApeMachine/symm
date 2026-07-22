package tests

/*
clone deep-copies reconstructed books so a rejected checksum or invariant
cannot poison the next coherent frame.
*/
func (validator *Validator) clone() *Validator {
	draft := NewValidator()
	draft.nextPriority = validator.nextPriority

	for symbol, sides := range validator.books {
		draft.books[symbol] = map[string]map[float64]float64{}

		for side, levels := range sides {
			draft.books[symbol][side] = make(map[float64]float64, len(levels))

			for price, quantity := range levels {
				draft.books[symbol][side][price] = quantity
			}
		}
	}

	for symbol, sides := range validator.orders {
		draft.orders[symbol] = map[string]map[string]orderState{}

		for side, orders := range sides {
			draft.orders[symbol][side] = make(map[string]orderState, len(orders))

			for orderID, order := range orders {
				draft.orders[symbol][side][orderID] = order
			}
		}
	}

	for symbol, ticker := range validator.ticker {
		draft.ticker[symbol] = ticker
	}

	for symbol, observed := range validator.observed {
		draft.observed[symbol] = observed
	}

	return draft
}
