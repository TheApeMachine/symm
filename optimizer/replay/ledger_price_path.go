package replay

import (
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/market/perspectives/types"
)

func (ledger *replayLedger) observePrice(row types.Measurement) {
	ledger.observeSymbolPrice(row.Symbol, row.Last)
}

func (ledger *replayLedger) observeSymbolPrice(symbol string, price float64) {
	if symbol == "" || price <= 0 {
		return
	}

	prices := ledger.pricePaths[symbol]

	if len(prices) == 0 || prices[len(prices)-1] != price {
		prices = append(prices, price)
	}

	window := perspectives.RegimeWindow()

	if len(prices) > window {
		prices = append(prices[:0], prices[len(prices)-window:]...)
	}

	ledger.pricePaths[symbol] = prices
}

func (ledger *replayLedger) priceVolatility(symbol string) float64 {
	return perspectives.DistinctPriceVolatility(ledger.pricePaths[symbol])
}
