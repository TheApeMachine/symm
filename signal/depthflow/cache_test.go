package depthflow

import (
	"time"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

func bookCache(rows ...kraken.BookData) *types.MarketFeed[kraken.BookData] {
	capacity := max(128, len(rows))
	cache := types.NewMarketFeed[kraken.BookData](capacity, capacity)

	for _, row := range rows {
		if err := cache.Observe(row.Symbol, row.Timestamp, row); err != nil {
			panic(err)
		}
	}

	return cache
}

func tradeCache(rows ...kraken.TradeData) *types.MarketFeed[kraken.TradeData] {
	capacity := max(128, len(rows))
	cache := types.NewMarketFeed[kraken.TradeData](capacity, capacity)

	for _, row := range rows {
		if err := cache.Observe(row.Symbol, row.Timestamp, row); err != nil {
			panic(err)
		}
	}

	return cache
}

func bookRows(cache *types.MarketFeed[kraken.BookData]) []kraken.BookData {
	rows, err := cache.Pending(time.Now().UTC())

	if err != nil {
		panic(err)
	}

	return rows
}

func tradeRows(cache *types.MarketFeed[kraken.TradeData]) []kraken.TradeData {
	rows, err := cache.Pending(time.Now().UTC())

	if err != nil {
		panic(err)
	}

	return rows
}
