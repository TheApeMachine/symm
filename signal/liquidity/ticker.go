package liquidity

import (
	"container/ring"
	"context"
	"sync"

	"github.com/spf13/viper"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/utils"
)

/*
Ticker ingests public ticker rows into the shared liquidity sample.
*/
type Ticker struct {
	ctx    context.Context
	cancel context.CancelFunc
	api    *websocket.API
	cache  *sync.Map
}

func NewTicker(ctx context.Context, api *websocket.API) *Ticker {
	ctx, cancel := context.WithCancel(ctx)

	ticker := &Ticker{
		ctx:    ctx,
		cancel: cancel,
		api:    api,
		cache:  &sync.Map{},
	}

	ticker.api.On("ticker", ticker.On)
	return ticker
}

/*
On decodes an inbound market-data message and retains its relevant rows so
measurement uses the authoritative event stream.
*/
func (ticker *Ticker) On(data []byte) {
	if len(data) == 0 {
		return
	}

	frame := utils.Unmarshal[kraken.Ticker](data)

	if len(frame.Data) == 0 {
		return
	}

	for _, data := range frame.Data {
		found, _ := ticker.cache.LoadOrStore(data.Symbol, ring.New(
			viper.GetInt("signals.feed_ring_capacity"),
		))

		track := found.(*ring.Ring)
		track.Value = data
		ticker.cache.Store(data.Symbol, track.Next())
	}
}
