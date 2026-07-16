package depthflow

import (
	"container/ring"
	"context"
	"sync"

	"github.com/spf13/viper"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
)

/*
Trade ingests public trade rows into the shared depth-flow sample.
*/
type Trade struct {
	ctx    context.Context
	cancel context.CancelFunc
	api    *websocket.API
	cache  *sync.Map
}

func NewTrade(ctx context.Context, api *websocket.API) *Trade {
	ctx, cancel := context.WithCancel(ctx)

	trade := &Trade{
		ctx:    ctx,
		cancel: cancel,
		api:    api,
		cache:  &sync.Map{},
	}

	trade.api.On("trade", trade.On)
	return trade
}

/*
On decodes an inbound market-data message and retains its relevant rows so
measurement uses the authoritative event stream.
*/
func (trade *Trade) On(data []byte) {
	if len(data) == 0 {
		return
	}

	frame := kraken.NewTrade(data)

	if len(frame.Data) == 0 {
		return
	}

	for _, data := range frame.Data {
		found, _ := trade.cache.LoadOrStore(data.Symbol, ring.New(
			viper.GetInt("signals.feed_ring_capacity"),
		))

		track := found.(*ring.Ring)
		track.Value = data
		trade.cache.Store(data.Symbol, track.Next())
	}
}
