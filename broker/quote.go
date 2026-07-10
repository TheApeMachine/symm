package broker

import (
	"strings"

	"github.com/theapemachine/symm/kraken"
)

type Quote struct {
	price *Price
}

func NewQuote(price *Price) *Quote {
	return &Quote{price: price}
}

func (quote *Quote) On(data []byte) {
	rows := kraken.NewTickerDataSlice(data)
	symbols, _ := quote.price.symbols.Load().(map[string]struct{})

	if len(symbols) == 0 {
		return
	}

	current, _ := quote.price.tickers.Load().(map[string]kraken.TickerData)
	next := make(map[string]kraken.TickerData, len(symbols))

	for symbol, ticker := range current {
		if _, ok := symbols[symbol]; ok {
			next[symbol] = ticker
		}
	}

	if len(rows) == 0 {
		if len(next) != len(current) {
			quote.price.tickers.Store(next)
		}

		return
	}

	changed := false

	for _, ticker := range rows {
		symbol := strings.TrimSpace(ticker.Symbol)

		if symbol == "" {
			continue
		}

		if _, ok := symbols[symbol]; !ok {
			continue
		}

		ticker.Symbol = symbol
		next[symbol] = ticker
		changed = true
	}

	if changed || len(next) != len(current) {
		quote.price.tickers.Store(next)
	}
}
