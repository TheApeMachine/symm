package correlation

import (
	"time"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

func tickerCache(rows ...kraken.TickerData) *types.MarketFeed[kraken.TickerData] {
	capacity := 128
	for capacity < len(rows) {
		capacity <<= 1
	}
	cache := types.NewMarketFeed[kraken.TickerData](capacity, capacity)

	for _, row := range rows {
		if err := cache.Observe(row.Symbol, row.Timestamp, row); err != nil {
			panic(err)
		}
	}

	return cache
}

func tickerRows(cache *types.MarketFeed[kraken.TickerData]) []kraken.TickerData {
	rows, err := cache.Pending(time.Now().UTC())

	if err != nil {
		panic(err)
	}

	return rows
}
