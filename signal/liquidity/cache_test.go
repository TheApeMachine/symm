package liquidity

import (
	"container/ring"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/kraken"
	"sync"
)

func tickerCache(rows ...kraken.TickerData) *sync.Map {
	viper.Set("signals.feed_ring_capacity", 128)
	cache := &sync.Map{}
	for _, row := range rows {
		found, _ := cache.LoadOrStore(row.Symbol, ring.New(128))
		track := found.(*ring.Ring)
		track.Value = row
		cache.Store(row.Symbol, track.Next())
	}
	return cache
}

func tickerRows(cache *sync.Map) []kraken.TickerData {
	rows := make([]kraken.TickerData, 0)
	cache.Range(func(_, value any) bool {
		value.(*ring.Ring).Do(func(value any) {
			if value != nil {
				rows = append(rows, value.(kraken.TickerData))
			}
		})
		return true
	})
	return rows
}
