package toxicity

import (
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

func tradeCache(rows ...kraken.TradeData) *types.MarketFeed[kraken.TradeData] {
	cache := types.NewMarketFeed[kraken.TradeData](128, 128)

	for _, row := range rows {
		if err := cache.Observe(row.Symbol, row.Timestamp, row); err != nil {
			panic(err)
		}
	}

	return cache
}
