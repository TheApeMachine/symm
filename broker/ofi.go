package broker

/*
NoteOFI caches Cont event order-flow imbalance for profit-only exit pacing.
Negative readings mean bid liquidity is withdrawing — never authorizes a loss exit.
*/
func (price *Price) NoteOFI(symbol string, eventOFI float64) {
	if price == nil {
		return
	}

	if price.normalizer != nil {
		symbol = price.normalizer.Name(symbol)
	}

	if symbol == "" {
		return
	}

	price.ofi.Store(symbol, eventOFI)
}

/*
OFIReading is the last NoteOFI for symbol, or 0 when never seen.
*/
func (price *Price) OFIReading(symbol string) float64 {
	if price == nil {
		return 0
	}

	if price.normalizer != nil {
		symbol = price.normalizer.Name(symbol)
	}

	if symbol == "" {
		return 0
	}

	raw, ok := price.ofi.Load(symbol)
	if !ok {
		return 0
	}

	return raw.(float64)
}
