package toxicity

import (
	"container/ring"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/kraken"
	"sync"
)

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
