package exhaust

import (
	"container/ring"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/kraken"
	"sync"
)

func bookCache(rows ...kraken.BookData) *sync.Map {
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
func tradeCache(rows ...kraken.TradeData) *sync.Map {
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
func bookRows(cache *sync.Map) []kraken.BookData {
	rows := make([]kraken.BookData, 0)
	cache.Range(func(_, value any) bool {
		value.(*ring.Ring).Do(func(value any) {
			if value != nil {
				rows = append(rows, value.(kraken.BookData))
			}
		})
		return true
	})
	return rows
}
func tradeRows(cache *sync.Map) []kraken.TradeData {
	rows := make([]kraken.TradeData, 0)
	cache.Range(func(_, value any) bool {
		value.(*ring.Ring).Do(func(value any) {
			if value != nil {
				rows = append(rows, value.(kraken.TradeData))
			}
		})
		return true
	})
	return rows
}
