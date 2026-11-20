package broker

/*
NoteExcitation caches Hawkes spectral radius for profit-only exit pacing.
Never authorizes a loss exit — only tightens tranche spacing when already green.
*/
func (price *Price) NoteExcitation(symbol string, spectralRadius float64) {
	if price == nil {
		return
	}

	if price.normalizer != nil {
		symbol = price.normalizer.Name(symbol)
	}

	if symbol == "" || spectralRadius < 0 {
		return
	}

	price.excitation.Store(symbol, spectralRadius)
}

/*
ExcitationReading is the last NoteExcitation for symbol, or 0 when never seen.
*/
func (price *Price) ExcitationReading(symbol string) float64 {
	if price == nil {
		return 0
	}

	if price.normalizer != nil {
		symbol = price.normalizer.Name(symbol)
	}

	if symbol == "" {
		return 0
	}

	raw, ok := price.excitation.Load(symbol)
	if !ok {
		return 0
	}

	return raw.(float64)
}
