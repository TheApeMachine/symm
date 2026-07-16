package pumpdump

import (
	"time"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

func tickerCache(rows ...kraken.TickerData) *types.MarketFeed[kraken.TickerData] {
	cache := types.NewMarketFeed[kraken.TickerData](128, 128)

	for _, row := range rows {
		if err := cache.Observe(row.Symbol, row.Timestamp, row); err != nil {
			panic(err)
		}
	}

	return cache
}

func tickerRows(
	cache *types.MarketFeed[kraken.TickerData],
	cutoff time.Time,
) []kraken.TickerData {
	rows, err := cache.Pending(cutoff)

	if err != nil {
		panic(err)
	}

	return rows
}
