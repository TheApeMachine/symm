package exhaust

import (
	"time"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

func bookCache(rows ...kraken.BookData) *types.MarketFeed[kraken.BookData] {
	cache := types.NewMarketFeed[kraken.BookData](128, 128)

	for _, row := range rows {
		if err := cache.Observe(row.Symbol, row.Timestamp, row); err != nil {
			panic(err)
		}
	}

	return cache
}

func tradeCache(rows ...kraken.TradeData) *types.MarketFeed[kraken.TradeData] {
	cache := types.NewMarketFeed[kraken.TradeData](128, 128)

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
