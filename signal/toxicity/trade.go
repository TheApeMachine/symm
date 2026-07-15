package toxicity

import (
	"context"
	"sync"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
)

/*
Trade ingests public tape rows into the shared toxicity sample.
*/
type Trade struct {
	ctx    context.Context
	cancel context.CancelFunc
	api    *websocket.API
	access sync.Mutex
	cache  []kraken.TradeData
}

/*
NewTrade binds one shared sample and measurement stage for trade rows.
*/
func NewTrade(ctx context.Context, api *websocket.API) *Trade {
	ctx, cancel := context.WithCancel(ctx)

	trade := &Trade{
		ctx:    ctx,
		cancel: cancel,
		api:    api,
		cache:  []kraken.TradeData{},
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

	trades := kraken.NewTrade(data)

	if len(trades.Data) == 0 {
		return
	}

	trade.access.Lock()
	trade.cache = append(trade.cache, trades.Data...)
	trade.access.Unlock()
}
